package ui

import (
	"image/color"
	"os"
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

// A document reload can cut the headings a focused sidebar is showing: the
// cursor and its scroll position come back inside the new list.
func TestTOCClampAfterReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	big := "# one\n\n## two\n\n## three\n\n## four\n\n## five\n"
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, path)
	press(a, "tab") // the sidebar
	press(a, "G")   // the last heading
	if a.focus != focusTOC || a.toc.cursor != len(a.pane.doc.Headings)-1 {
		t.Fatalf("cursor %d of %d headings", a.toc.cursor, len(a.pane.doc.Headings))
	}
	if err := os.WriteFile(path, []byte("# only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	press(a, "r")
	if n := len(a.pane.doc.Headings); a.toc.cursor >= n {
		t.Fatalf("cursor %d is past the %d headings left", a.toc.cursor, n)
	}
	if a.toc.offset != 0 {
		t.Fatalf("scroll offset %d is past the end", a.toc.offset)
	}
	press(a, "j") // and the next key still works
	if a.toc.cursor != 0 {
		t.Fatalf("cursor %d after j", a.toc.cursor)
	}
}

// Dragging a pane border moves the panes on every motion, but re-renders the
// document only once, when the drag ends: a render per mouse motion would
// redo the whole document at each intermediate width.
func TestDragRendersOnceAtTheEnd(t *testing.T) {
	a := newTestApp(t, filepath.Join(fixture(t), "a.md"), filepath.Join(fixture(t), "b.md"))
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	press(a, "ctrl+w", "v") // split, two panes on a.md and b.md
	if !a.tab().multi() {
		t.Fatal("expected a split")
	}
	border := -1
	for x := a.bodyX(); x < a.width && border < 0; x++ {
		if a.tab().root.borderAt(x, 2) != nil {
			border = x
		}
	}
	if border < 0 {
		t.Fatal("no border to drag")
	}
	y := 1 + 2 // the tab bar is above the body
	a.Update(tea.MouseClickMsg(tea.Mouse{X: border, Y: y, Button: tea.MouseLeft}))
	if a.drag == nil {
		t.Fatal("a click on the border should start a drag")
	}
	view, wrap := a.pane.view, a.pane.wrap
	a.Update(tea.MouseMotionMsg(tea.Mouse{X: border + 6, Y: y, Button: tea.MouseLeft}))
	if a.pane.view != view || a.pane.wrap != wrap {
		t.Error("the drag re-rendered the document")
	}
	a.Update(tea.MouseReleaseMsg(tea.Mouse{X: border + 6, Y: y, Button: tea.MouseLeft}))
	if a.drag != nil {
		t.Fatal("the release should end the drag")
	}
	if a.pane.view == view && a.pane.wrap == wrap {
		t.Error("the drag should render the document once it ends")
	}
}

// A size that keeps changing is rendered once it settles: the panes move at
// once, but the document is rendered again by the last timer only.
func TestResizeRendersWhenItSettles(t *testing.T) {
	a := newTestApp(t, filepath.Join(fixture(t), "a.md"))
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if a.pane.view == nil {
		t.Fatal("the first size should render at once")
	}
	_, cmd := a.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if cmd == nil {
		t.Fatal("a later size should wait for the drag to settle")
	}
	a.Update(tea.WindowSizeMsg{Width: 70, Height: 30}) // supersedes cmd
	if a.pane.wrap != wrapWidth(100, a.opts.MaxWrap) {
		t.Fatalf("the resize rendered at once: wrap %d", a.pane.wrap)
	}
	a.pane.view = nil             // so a render shows
	a.update(relayoutMsg{gen: 1}) // a superseded timer
	if a.pane.view != nil {
		t.Error("a superseded resize rendered")
	}
	a.update(relayoutMsg{gen: a.layoutGen})
	if a.pane.view == nil {
		t.Fatal("the latest resize should render")
	}
	if a.pane.wrap != wrapWidth(a.pane.width, a.opts.MaxWrap) || a.pane.width != 70-a.bodyX() {
		t.Errorf("pane %d cells wide, wrapped at %d", a.pane.width, a.pane.wrap)
	}
}
