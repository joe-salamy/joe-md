// Package search runs ripgrep and parses its JSON output. A query is one
// pattern either way: in literal mode a phrase, matched with any whitespace
// or inline markup between its words and with straight and curly quotes and
// the dash forms alike; in regex mode a single regex.
package search

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/joe-salamy/joe-md/internal/natural"
)

// Scope is how much ripgrep searches around the current file.
type Scope int

const (
	File Scope = iota // the current file
	Open              // every open file
	Dir               // the current file's directory, recursively
	Repo              // the enclosing git repository (or Dir outside one)
)

var scopeNames = [...]string{"file", "open", "dir", "repo"}

func (s Scope) String() string { return scopeNames[s] }

// Next cycles file → open → dir → repo → file (or backwards for step < 0).
func (s Scope) Next(step int) Scope {
	n := Scope(len(scopeNames))
	return ((s+Scope(step))%n + n) % n
}

// Root returns the path ripgrep searches for scope s around file, which
// must be absolute (as doc paths are), so ripgrep reports absolute paths.
func Root(file string, s Scope) string {
	switch s {
	case File:
		return file
	case Repo:
		if r := RepoRoot(filepath.Dir(file)); r != "" {
			return r
		}
	}
	return filepath.Dir(file)
}

// RepoRoot is the nearest directory at or above dir containing .git (a
// directory, or a file for worktrees and submodules), or "" if there is none.
func RepoRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Case is how a search treats letter case.
type Case int

const (
	IgnoreCase Case = iota // rg -i
	SmartCase              // rg -S: case-sensitive only if the query has capitals
	MatchCase              // rg -s
)

var caseNames = [...]string{"ignore", "smart", "sensitive"}

func (c Case) String() string { return caseNames[c] }

// Scope and Case are written by name in the settings file.

func (s Scope) MarshalText() ([]byte, error) { return []byte(s.String()), nil }
func (c Case) MarshalText() ([]byte, error)  { return []byte(c.String()), nil }

func (s *Scope) UnmarshalText(b []byte) error { return unmarshalName(s, "scope", scopeNames[:], b) }
func (c *Case) UnmarshalText(b []byte) error  { return unmarshalName(c, "case", caseNames[:], b) }

// unmarshalName sets *v to the index of name b in names, the inverse of the
// String methods; what names the setting in the error.
func unmarshalName[T ~int](v *T, what string, names []string, b []byte) error {
	i := slices.Index(names, string(b))
	if i < 0 {
		return fmt.Errorf("%s %q: must be %s or %s", what, b, strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
	}
	*v = T(i)
	return nil
}

// Mode is how a query is matched.
type Mode struct {
	Literal bool // a phrase of plain text (see phrase), not a regex
	Case    Case
}

// Request is one search.
type Request struct {
	Query string // see Term
	Mode
	Scope Scope
	Root  string   // file or directory to search, from Root
	Files []string // open files to search, for Open (absolute paths)
	// Exclude skips these paths in dir/repo/open walks, as gitignore-style
	// globs (rg -g '!pat'). Explicit file paths are unaffected: ripgrep's
	// globs never filter them.
	Exclude []string
	Limit   int // stop after this many matching lines; 0 means the default
}

// Default limits: a list of results across files is only useful up to a
// point, but n / N within one file should reach every match.
const (
	DefaultLimit     = 5000
	DefaultFileLimit = 1_000_000
)

// Match is one matching line.
type Match struct {
	Path  string   // as ripgrep printed it (absolute, since Root is)
	Line  int      // 1-based
	Text  string   // the line, without its line ending
	Spans [][2]int // byte ranges of the matches within Text
}

// Terms returns the distinct matched strings on the line.
func (m Match) Terms() []string {
	var out []string
	for _, s := range m.Spans {
		if t := m.Text[s[0]:s[1]]; t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

type Result struct {
	Matches   []Match // sorted by path (natural.PathCompare), then line
	Files     int     // number of distinct files in Matches
	Truncated bool    // Limit was hit
}

// Term is the query as ripgrep runs it: trimmed, and otherwise whole, so
// spaces in a regex are part of it.
func (r Request) Term() string { return strings.TrimSpace(r.Query) }

// Pattern compiles the query as a Go regexp that matches what ripgrep would,
// so matches can be found again in text ripgrep never saw, or nil for an
// empty query. Go's syntax is close to ripgrep's but not identical; a query
// Go can't compile is an error. The pattern is multi-line, so ^ and $ still
// match at each line of a text holding several.
func (r Request) Pattern() (*regexp.Regexp, error) {
	t := r.Term()
	if t == "" {
		return nil, nil
	}
	expr := "(?m)" + r.expr()
	if r.Case == IgnoreCase || r.Case == SmartCase && !hasUpper(t, r.Literal) {
		expr = "(?i)" + expr
	}
	return regexp.Compile(expr)
}

// expr is the regexp ripgrep runs: the term itself, or for a literal query
// its phrase (see phrase).
func (r Request) expr() string {
	if r.Literal {
		return phrase(r.Term())
	}
	return r.Term()
}

// Inline markup that may sit next to the spaces of a phrase: emphasis and
// code marks, link brackets and targets, and HTML tags.
const markup = "(?:[*_~`]|\\[|\\]\\([^)]*\\)|\\]|<[^>]*>)*"

// folds are the characters a phrase treats as the same: typographic quotes
// and dashes stand in for the ' " and - people type.
var folds = []string{"'‘’‚‛ʼ′", "\"“”„‟″", "-‐‑‒–—―−"}

// phrase is the regexp for literal phrase q: its words, quoted, with any
// whitespace between them (a no-break space included), and markup allowed on
// either side of it, so `one **fixed** string` matches "one fixed string".
// A character in folds matches any of its look-alikes. The syntax is common
// to Go and ripgrep.
func phrase(q string) string {
	words := strings.Fields(q)
	for i, w := range words {
		var b strings.Builder
		for _, c := range w {
			f := ""
			for _, s := range folds {
				if strings.ContainsRune(s, c) {
					f = s
				}
			}
			if f == "" {
				b.WriteString(regexp.QuoteMeta(string(c)))
				continue
			}
			b.WriteByte('[')
			for _, l := range f {
				if l == '-' {
					b.WriteByte('\\')
				}
				b.WriteRune(l)
			}
			b.WriteByte(']')
		}
		words[i] = b.String()
	}
	return strings.Join(words, markup+"[\\s\\x{a0}]+"+markup)
}

// hasUpper reports whether term has a capital letter, as ripgrep's smart case
// sees it: in a regex, an escape such as \W or \S is not a capital.
func hasUpper(term string, literal bool) bool {
	escaped := false
	for _, c := range term {
		switch {
		case escaped:
			escaped = false
		case c == '\\' && !literal:
			escaped = true
		case unicode.IsUpper(c):
			return true
		}
	}
	return false
}

// Run searches with ripgrep. Directory scopes only search markdown files; a
// single file, and the open scope's explicit files, are searched whatever
// their extension. It stops as soon as more than the limit of lines match.
func Run(parent context.Context, req Request) (Result, error) {
	rg, err := exec.LookPath("rg")
	if err != nil {
		return Result{}, errors.New("ripgrep (rg) not found in PATH")
	}
	limit := req.Limit
	switch {
	case limit > 0:
	case req.Scope == File:
		limit = DefaultFileLimit
	default:
		limit = DefaultLimit
	}
	if req.Term() == "" || req.Scope == Open && len(req.Files) == 0 {
		// With no files, ripgrep would search the working directory instead.
		return Result{}, nil
	}
	var out []Match
	err = stream(parent, rg, req, func(m Match) bool {
		out = append(out, m)
		return len(out) <= limit
	})
	if err := parent.Err(); err != nil {
		return Result{}, err // superseded by a newer search
	}
	if err != nil {
		return Result{}, err
	}
	return finish(out, limit), nil
}

// args are ripgrep's options for searching the scope. Ignored files are
// always searched: the file menu lists them, so a cross-file search must
// cover them too (an explicit file path bypasses ignores anyway). Hidden
// files stay skipped.
func args(req Request) []string {
	out := []string{[...]string{"-i", "-S", "-s"}[req.Case], "--no-ignore"}
	if req.Scope != File && req.Scope != Open {
		out = append(out, "-t", "markdown")
	}
	for _, e := range req.Exclude {
		out = append(out, "-g", "!"+e)
	}
	return out
}

// paths are what ripgrep searches for req: the open files, or the root.
func paths(req Request) []string {
	if req.Scope == Open {
		return req.Files
	}
	return []string{req.Root}
}

// stream runs the query through ripgrep over the scope's paths, handing each
// matching line to f until f returns false.
func stream(ctx context.Context, rg string, req Request, f func(Match) bool) error {
	cmdArgs := append(append(args(req), "--json"), "--", req.expr())
	cmdArgs = append(cmdArgs, paths(req)...)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg, cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	found, stopped := false, false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		m, ok := parseMatch(sc.Bytes())
		if !ok {
			continue
		}
		found = true
		if !f(m) {
			stopped = true
			cancel() // kill rg; we have enough
			break
		}
	}
	// A line too long for the scanner ends the loop early. Kill ripgrep
	// before waiting: it would block writing to a pipe no one reads.
	scanErr := sc.Err()
	if scanErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if scanErr != nil {
		return fmt.Errorf("reading ripgrep output: %w", scanErr)
	}

	// Exit 1 means no matches. Exit 2 means an error, but ripgrep keeps going
	// past unreadable files, so only report it when nothing was found.
	var exit *exec.ExitError
	if waitErr != nil && !stopped && !found && ctx.Err() == nil {
		if !errors.As(waitErr, &exit) || exit.ExitCode() != 1 {
			if msg := firstLines(stderr.String()); msg != "" {
				return errors.New(msg)
			}
			return waitErr
		}
	}
	return nil
}

// finish sorts matches by path, in menu order, then line, keeps the first
// limit, and counts the files.
func finish(ms []Match, limit int) Result {
	slices.SortStableFunc(ms, func(a, b Match) int {
		return cmp.Or(natural.PathCompare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})
	truncated := len(ms) > limit
	if truncated {
		ms = ms[:limit]
	}
	res := Result{Matches: ms, Truncated: truncated}
	for i, m := range ms {
		if i == 0 || m.Path != ms[i-1].Path {
			res.Files++
		}
	}
	return res
}

// rgText is ripgrep's encoding of possibly non-UTF-8 data.
type rgText struct {
	Text  *string `json:"text"`
	Bytes string  `json:"bytes"`
}

func (t rgText) String() string {
	if t.Text != nil {
		return *t.Text
	}
	b, _ := base64.StdEncoding.DecodeString(t.Bytes)
	return string(b)
}

type rgMessage struct {
	Type string `json:"type"`
	Data struct {
		Path       rgText `json:"path"`
		Lines      rgText `json:"lines"`
		LineNumber int    `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}

// parseMatch decodes one line of `rg --json` output if it is a match.
func parseMatch(line []byte) (Match, bool) {
	if !bytes.Contains(line, []byte(`"type":"match"`)) {
		return Match{}, false
	}
	var msg rgMessage
	if err := json.Unmarshal(line, &msg); err != nil || msg.Type != "match" {
		return Match{}, false
	}
	d := msg.Data
	m := Match{
		Path: d.Path.String(),
		Line: d.LineNumber,
		Text: strings.TrimRight(d.Lines.String(), "\r\n"),
	}
	for _, s := range d.Submatches {
		start, end := min(s.Start, len(m.Text)), min(s.End, len(m.Text))
		if start < end {
			m.Spans = append(m.Spans, [2]int{start, end})
		}
	}
	return m, true
}

// firstLines flattens ripgrep's (often multi-line) error message.
func firstLines(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "^") {
			parts = append(parts, l)
		}
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " ")
}
