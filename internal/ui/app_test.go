package ui

import (
	"image/color"
	"path/filepath"
	"testing"

	"github.com/joe-salamy/joe-md/internal/doc"

	tea "charm.land/bubbletea/v2"
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
