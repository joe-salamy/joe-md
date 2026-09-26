// Package config loads joe-md's settings file,
// $XDG_CONFIG_HOME/joe-md/config.toml. Command-line flags override it, and it
// overrides the built-in defaults.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Style   string  `toml:"style"`
	Width   int     `toml:"width"`
	Startup Startup `toml:"startup"`
	Search  Search  `toml:"search"`
	Theme   Theme   `toml:"theme"`
	// Keys replaces the default keys of the named actions:
	// Keys[context][action] = keys.
	Keys map[string]map[string][]string `toml:"keys"`
}

type Startup struct {
	TOC       bool `toml:"toc"`
	SearchBar bool `toml:"search_bar"`
	Tabs      bool `toml:"tabs"`
}

type Search struct {
	Scope   string `toml:"scope"` // where ? starts: file, dir or repo
	Literal bool   `toml:"literal"`
	Case    string `toml:"case"` // ignore, smart or sensitive
}

// Theme colours are ANSI numbers ("39") or hex ("#7aa2f7"); empty keeps the
// default, which depends on whether the background is dark.
type Theme struct {
	Accent       string `toml:"accent"`
	Text         string `toml:"text"`
	Subtle       string `toml:"subtle"`
	Dim          string `toml:"dim"`
	Bar          string `toml:"bar"`
	OnAccent     string `toml:"on_accent"`
	TOCMode      string `toml:"toc_mode"`
	SearchMode   string `toml:"search_mode"`
	Match        string `toml:"match"`
	MatchCurrent string `toml:"match_current"`
	TabInactive  string `toml:"tab_inactive"`
	Error        string `toml:"error"`
}

func Default() Config {
	return Config{
		Style:   "auto",
		Width:   120,
		Startup: Startup{TOC: true, SearchBar: true, Tabs: true},
		Search:  Search{Scope: "repo", Case: "ignore"},
	}
}

// Path is where the settings file lives: $XDG_CONFIG_HOME/joe-md/config.toml,
// else ~/.config/joe-md/config.toml.
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "joe-md", "config.toml")
}

// Load reads the settings file at path over the defaults. A missing file is
// not an error unless required is set.
func Load(path string, required bool) (Config, error) {
	c := Default()
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !required {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := Parse(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse decodes TOML over c and checks the values. Unknown settings are
// errors, so typos don't go unnoticed.
func Parse(b []byte, c *Config) error {
	dec := toml.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) && len(strict.Errors) > 0 {
			var names []string
			for _, e := range strict.Errors {
				row, _ := e.Position()
				names = append(names, fmt.Sprintf("%s (line %d)", strings.Join(e.Key(), "."), row))
			}
			return errors.New("unknown setting " + strings.Join(names, ", "))
		}
		var de *toml.DecodeError
		if errors.As(err, &de) {
			row, col := de.Position()
			return fmt.Errorf("line %d column %d: %s", row, col, de.Error())
		}
		return err
	}
	return c.check()
}

var colorRE = regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|[0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])$`)

func (c *Config) check() error {
	if c.Width < 20 {
		return fmt.Errorf("width %d: must be at least 20", c.Width)
	}
	if _, ok := search.ParseScope(c.Search.Scope); !ok {
		return fmt.Errorf("search.scope %q: must be file, dir or repo", c.Search.Scope)
	}
	if _, ok := search.ParseCase(c.Search.Case); !ok {
		return fmt.Errorf("search.case %q: must be ignore, smart or sensitive", c.Search.Case)
	}
	for name, v := range c.Theme.Colors() {
		if v != "" && !colorRE.MatchString(v) {
			return fmt.Errorf("theme.%s %q: must be an ANSI colour 0-255 or #rrggbb", name, v)
		}
	}
	return nil
}

// Colors returns the theme by setting name.
func (t Theme) Colors() map[string]string {
	return map[string]string{
		"accent": t.Accent, "text": t.Text, "subtle": t.Subtle, "dim": t.Dim, "bar": t.Bar,
		"on_accent": t.OnAccent, "toc_mode": t.TOCMode, "search_mode": t.SearchMode,
		"match": t.Match, "match_current": t.MatchCurrent, "tab_inactive": t.TabInactive, "error": t.Error,
	}
}

// Binding documents one action for the template.
type Binding struct {
	Context, Action string
	Keys            []string
	Desc            string
}

// Template is a complete settings file with every setting commented out at
// its default, for `joe-md -dump-config`.
func Template(bindings []Binding) string {
	d := Default()
	var b strings.Builder
	b.WriteString(`# joe-md settings. Every line is commented out at its default value;
# uncomment and change what you want. Command-line flags override this file.

# glamour style: auto (dark or light to suit the terminal, honouring
# $GLAMOUR_STYLE), dark, light, dracula, tokyo-night, pink, notty, ascii, or
# the path to a JSON style.
`)
	fmt.Fprintf(&b, "# style = %q\n\n# Maximum word-wrap width.\n# width = %d\n\n", d.Style, d.Width)
	fmt.Fprintf(&b, "[startup]\n# What is shown at start-up (each can be toggled while running).\n"+
		"# toc = %t\n# search_bar = %t\n# tabs = %t\n\n", d.Startup.TOC, d.Startup.SearchBar, d.Startup.Tabs)
	fmt.Fprintf(&b, "[search]\n# Where ? starts: file, dir or repo. It then remembers the last scope used.\n# scope = %q\n"+
		"# Match queries as fixed strings (rg -F) rather than regexes; ctrl+r toggles it.\n# literal = %t\n"+
		"# Letter case: ignore (rg -i), smart (rg -S: sensitive if the query has capitals) or sensitive.\n# case = %q\n\n",
		d.Search.Scope, d.Search.Literal, d.Search.Case)
	b.WriteString(`[theme]
# ANSI colours (0-255) or "#rrggbb". Unset colours follow the style: dracula,
# tokyo-night and pink have matching UI colours; the others suit the
# terminal's background. The values shown are the dark ones.
# accent = "39"         # focus, cursor, prompts, headings in the menu
# text = "252"          # ordinary UI text
# subtle = "248"        # third-level headings in the sidebar
# dim = "241"           # hints, separators
# bar = "236"           # status line background
# on_accent = "235"     # text drawn on the accent colour
# toc_mode = "214"      # TOC and MENU mode labels
# search_mode = "78"    # SEARCH and RESULTS mode labels, open-file marks
# match = "186"         # search match background
# match_current = "214" # current match background
# tab_inactive = "238"  # inactive tab background
# error = "203"

# Keys. Each line sets all the keys of one action, replacing its defaults;
# [] unbinds it. A key is written as joe-md sees it: "j", "G", "ctrl+d",
# "alt+t", "shift+tab", "enter", "space", "f1". Sequences are keys separated
# by spaces: "g g", "] q". Digits are counts and can't be bound.
# The window keys follow normal.window (ctrl+w).
`)
	ctx := ""
	for _, k := range bindings {
		if k.Context != ctx {
			ctx = k.Context
			fmt.Fprintf(&b, "\n[keys.%s]\n", ctx)
		}
		keys := make([]string, len(k.Keys))
		for i, key := range k.Keys {
			keys[i] = fmt.Sprintf("%q", key)
		}
		line := fmt.Sprintf("# %s = [%s]", k.Action, strings.Join(keys, ", "))
		fmt.Fprintf(&b, "%-44s # %s\n", line, k.Desc)
	}
	return b.String()
}

// SortedKeys returns m's keys in order, for stable error messages.
func SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
