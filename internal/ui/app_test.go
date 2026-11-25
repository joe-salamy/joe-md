package ui

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/doc"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// An auto style starts dark and follows the background colour the terminal
// reports, without blocking start-up on the query.
func TestAutoBackgroundSwitchesStyle(t *testing.T) {
	d, err := doc.Load(filepath.Join(fixture(t), "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	a := New([]*doc.Doc{d}, Options{Style: "auto", Dark: true, MaxWrap: 80})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !a.opts.Dark || a.opts.Style != "dark" {
		t.Fatalf("initial style %q dark=%v", a.opts.Style, a.opts.Dark)
	}
	a.Update(tea.BackgroundColorMsg{Color: color.White})
	if a.opts.Dark || a.opts.Style != "light" {
		t.Fatalf("light background: style %q dark=%v", a.opts.Style, a.opts.Dark)
	}
	if a.pane.view == nil {
		t.Fatal("the pane should be rendered again for the new style")
	}
	a.Update(tea.BackgroundColorMsg{Color: color.Black})
	if !a.opts.Dark || a.opts.Style != "dark" {
		t.Fatalf("dark background: style %q dark=%v", a.opts.Style, a.opts.Dark)
	}
}

// The frame is exactly the terminal's size, whatever the state, pop-ups
// included: a line wider than the screen wraps, and an extra line scrolls.
func TestViewFitsTerminal(t *testing.T) {
	dir := fixture(t)
	d, err := doc.Load(filepath.Join(dir, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []int{8, 20, 40, 100} {
		for _, h := range []int{1, 2, 3, 4, 12, 30} {
			for _, state := range []string{"doc", "help", "menu", "empty", "results"} {
				a := New([]*doc.Doc{d}, Options{Style: "notty", Dark: true, MaxWrap: 80})
				a.Update(tea.WindowSizeMsg{Width: w, Height: h})
				switch state {
				case "help":
					press(a, "f1")
				case "menu":
					press(a, "o")
				case "empty":
					a = New(nil, Options{Style: "notty", Dark: true, MaxWrap: 80})
					a.Update(tea.WindowSizeMsg{Width: w, Height: h})
				case "results":
					press(a, "/")
					typeText(a, "text")
					a.update(press(a, "enter")())
				}
				lines := strings.Split(a.View().Content, "\n")
				if len(lines) != h {
					t.Errorf("%s at %dx%d: %d lines", state, w, h, len(lines))
				}
				for i, l := range lines {
					if got := ansi.StringWidth(l); got > w {
						t.Errorf("%s at %dx%d: line %d is %d cells: %q", state, w, h, i, got, ansi.Strip(l))
					}
				}
			}
		}
	}
}
