// Command joe-md is a terminal markdown viewer.
package main

import (
	"flag"
	"fmt"
	"os"

	"joe-md/internal/config"
	"joe-md/internal/doc"
	"joe-md/internal/search"
	"joe-md/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func main() {
	style := flag.String("style", "auto", "glamour style (dark, light, dracula, tokyo-night, pink, notty, ascii) or path to a JSON style; auto honours $GLAMOUR_STYLE")
	width := flag.Int("width", 120, "maximum word-wrap width")
	noTOC := flag.Bool("no-toc", false, "start with the table of contents hidden")
	noBar := flag.Bool("no-search-bar", false, "start with the search bar hidden (it still appears while typing a search)")
	noTabs := flag.Bool("no-tabs", false, "start with the tab bar hidden")
	cfgPath := flag.String("config", "", "settings file (default "+config.Path()+")")
	dump := flag.Bool("dump-config", false, "print a settings file with every setting at its default, and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: joe-md [flags] [FILE.md | DIR]...\n\n"+
			"Each file opens in a tab. A directory, or no arguments, opens the file menu there.\n"+
			"Flags override the settings file.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *dump {
		fmt.Print(config.Template(ui.Bindings()))
		return
	}

	path, required := config.Path(), false
	if *cfgPath != "" {
		path, required = *cfgPath, true
	}
	cfg, err := config.Load(path, required)
	if err != nil {
		fatal(err)
	}
	// Flags given on the command line override the file.
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "style":
			cfg.Style = *style
		case "width":
			cfg.Width = *width
		case "no-toc":
			cfg.Startup.TOC = !*noTOC
		case "no-search-bar":
			cfg.Startup.SearchBar = !*noBar
		case "no-tabs":
			cfg.Startup.Tabs = !*noTabs
		}
	})
	keymap, err := ui.NewKeymap(cfg.Keys)
	if err != nil {
		fatal(fmt.Errorf("%s: %w", path, err))
	}
	caseMode, _ := search.ParseCase(cfg.Search.Case) // checked by Load

	var docs []*doc.Doc
	menuDir := ""
	for _, arg := range flag.Args() {
		fi, err := os.Stat(arg)
		if err != nil {
			fatal(err)
		}
		if fi.IsDir() {
			if menuDir == "" {
				menuDir = arg
			}
			continue
		}
		d, err := doc.Load(arg)
		if err != nil {
			fatal(err)
		}
		docs = append(docs, d)
	}

	// Query the background before the TUI takes over the terminal.
	dark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	if cfg.Style == "auto" {
		cfg.Style = os.Getenv("GLAMOUR_STYLE")
		if cfg.Style == "" || cfg.Style == "auto" {
			cfg.Style = map[bool]string{true: "dark", false: "light"}[dark]
		}
	}

	app := ui.New(docs, ui.Options{
		Style: cfg.Style, Dark: dark, MaxWrap: cfg.Width,
		NoTOC: !cfg.Startup.TOC, NoBar: !cfg.Startup.SearchBar, NoTabs: !cfg.Startup.Tabs,
		MenuDir: menuDir,
		Keymap:  keymap,
		Colors:  cfg.Theme,
		Scope:   cfg.Search.Scope,
		Mode:    search.Mode{Literal: cfg.Search.Literal, Case: caseMode},
	})
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "joe-md:", err)
	os.Exit(1)
}
