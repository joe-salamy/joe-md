package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// mkey sends one raw key press, spelling modifiers the way terminals do.
func mkey(a *App, code rune, text string, mod tea.KeyMod) {
	_, _ = a.Update(tea.KeyPressMsg{Code: code, Text: text, Mod: mod})
}

func openMenuAt(a *App, name string) {
	for i, j := range a.menu.shown {
		if a.menu.entries[j].name == name {
			a.menu.move(i, a.menuRows())
			return
		}
	}
	panic("menu has no " + name)
}

func markedNames(a *App) string {
	var names []string
	for _, j := range a.menu.marked() {
		names = append(names, a.menu.entries[j].name)
	}
	return strings.Join(names, " ")
}

func menuCursor(a *App) string {
	e, _ := a.menu.current()
	return e.name
}

// Shift extends the marks from the anchor, like a file manager. Only files
// join the range: directories and .. are skipped but the cursor still lands
// on them. Shrinking the range unmarks.
func TestMenuShiftExtends(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "b.md"))
	press(a, "o") // shown: .. docs a.md b.md
	openMenuAt(a, "a.md")

	mkey(a, 'j', "", tea.ModShift)
	if got, cur := markedNames(a), menuCursor(a); got != "a.md b.md" || cur != "b.md" {
		t.Fatalf("shift+j: marks %q cursor %q", got, cur)
	}
	mkey(a, 'j', "", tea.ModShift) // past the end: clamped, marks kept
	if got, cur := markedNames(a), menuCursor(a); got != "a.md b.md" || cur != "b.md" {
		t.Fatalf("shift+j at the end: marks %q cursor %q", got, cur)
	}
	mkey(a, 'k', "", tea.ModShift) // back to the anchor: just a.md
	if got, cur := markedNames(a), menuCursor(a); got != "a.md" || cur != "a.md" {
		t.Fatalf("shrinking unmarks: marks %q cursor %q", got, cur)
	}
	mkey(a, 'k', "", tea.ModShift) // over docs: skipped, not marked
	if got, cur := markedNames(a), menuCursor(a); got != "a.md" || cur != "docs" {
		t.Fatalf("extend over a directory: marks %q cursor %q", got, cur)
	}
	mkey(a, 'j', "", tea.ModShift) // back onto the anchor
	if got, cur := markedNames(a), menuCursor(a); got != "a.md" || cur != "a.md" {
		t.Fatalf("extending back: marks %q cursor %q", got, cur)
	}
	press(a, "enter")
	if a.menu != nil || tabNames(a) != "b.md a.md" {
		t.Fatalf("open marked: menu %v tabs %q", a.menu != nil, tabNames(a))
	}
}

// Ctrl moves without touching the marks; a plain move clears them, and capitals
// work where shift arrives as a capital letter.
func TestMenuCtrlKeepsAndPlainClears(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "b.md"))
	press(a, "o") // shown: .. docs a.md b.md
	openMenuAt(a, "docs")

	press(a, "t") // no-op on a directory
	if len(a.menu.sel) != 0 {
		t.Fatal("t on a directory marked something")
	}
	openMenuAt(a, "docs")          // re-anchor: t stepped past it
	mkey(a, 'j', "", tea.ModShift) // docs..a.md: only a.md marked
	if got, cur := markedNames(a), menuCursor(a); got != "a.md" || cur != "a.md" {
		t.Fatalf("extend over a directory: marks %q cursor %q", got, cur)
	}
	mkey(a, 'n', "", tea.ModCtrl) // onto b.md, marks kept
	if got, cur := markedNames(a), menuCursor(a); got != "a.md" || cur != "b.md" {
		t.Fatalf("ctrl+n: marks %q cursor %q", got, cur)
	}
	press(a, "k") // plain move clears
	if len(a.menu.sel) != 0 {
		t.Fatalf("plain move keeps %q", markedNames(a))
	}

	openMenuAt(a, "a.md")
	press(a, "J") // capital = shift+j without the Kitty protocol
	if got := markedNames(a); got != "a.md b.md" {
		t.Fatalf("J marks %q", got)
	}
	press(a, "esc") // clears the marks first
	if a.menu == nil || len(a.menu.sel) != 0 {
		t.Fatal("esc should clear the marks before closing")
	}
	press(a, "esc")
	if a.menu != nil {
		t.Fatal("second esc should close the menu")
	}
}

// toggle-all, directories, refiltering, and the multi-open path.
func TestMenuMultiSelectEdges(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "b.md"))

	// toggle-all marks the shown files, and again unmarks.
	press(a, "o")
	mkey(a, 'a', "", tea.ModCtrl)
	if got := markedNames(a); got != "a.md b.md" {
		t.Fatalf("ctrl+a marks %q", got)
	}
	if got := a.menu.count(a.menu.cursor); !strings.Contains(got, "+2") {
		t.Fatalf("count %q should carry +2", got)
	}
	mkey(a, 'a', "", tea.ModCtrl)
	if len(a.menu.sel) != 0 {
		t.Fatalf("second ctrl+a keeps %q", markedNames(a))
	}

	// Directories and .. can never be marked.
	openMenuAt(a, "docs")
	press(a, "t")
	if len(a.menu.sel) != 0 {
		t.Fatal("t on a directory marked something")
	}
	a.menu.move(0, a.menuRows()) // ..
	press(a, "t")
	if len(a.menu.sel) != 0 {
		t.Fatal("t on .. marked something")
	}

	// A refilter keeps only the marks still shown.
	openMenuAt(a, "a.md")
	press(a, "t")
	press(a, "/")
	typeText(a, "b")
	if got := markedNames(a); got != "" {
		t.Fatalf("refilter keeps hidden mark %q", got)
	}
	press(a, "esc") // stops filtering and clears the filter
	if a.menu == nil || a.menu.filtering {
		t.Fatal("esc should leave the menu open and stop filtering")
	}
	openMenuAt(a, "a.md")
	press(a, "t")
	mkey(a, 'j', "", tea.ModShift) // a.md b.md
	press(a, "enter")
	if a.menu != nil || tabNames(a) != "b.md a.md" {
		t.Fatalf("open marked: menu %v tabs %q", a.menu != nil, tabNames(a))
	}

	// Entering a directory drops the marks through a fresh listing.
	// The multi-open left a.md focused, so the menu opens in the root.
	press(a, "o")
	openMenuAt(a, "docs")
	press(a, "l") // into docs
	if a.menu == nil || menuNames(a) != ".. sub c.md" {
		t.Fatalf("in docs: %q", menuNames(a))
	}
	if len(a.menu.sel) != 0 {
		t.Fatal("entering a directory keeps marks")
	}
	openMenuAt(a, "sub")
	press(a, "t") // no-op on the directory, steps onto c.md
	if menuCursor(a) != "c.md" || len(a.menu.sel) != 0 {
		t.Fatalf("t on a directory: cursor %q marks %q", menuCursor(a), markedNames(a))
	}
	openMenuAt(a, "sub")
	press(a, "l") // into sub
	if menuNames(a) != ".. d.md" || len(a.menu.sel) != 0 {
		t.Fatalf("entered with marks %q: %q", markedNames(a), menuNames(a))
	}
	press(a, "h")
	if len(a.menu.sel) != 0 {
		t.Fatal("going up keeps marks")
	}
	press(a, "esc")

	// Marks render as + and the footer carries the count.
	press(a, "o")
	openMenuAt(a, "a.md")
	press(a, "t")
	out := ansi.Strip(strings.Join(a.menuView(), "\n"))
	if !strings.Contains(out, "+") || !strings.Contains(out, "a.md") || !strings.Contains(out, "+1") {
		t.Fatalf("marks not drawn:\n%s", out)
	}
}

// Shift-click extends, ctrl-click flips, a plain click opens one file.
func TestMenuMouseMultiSelect(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "b.md"))
	press(a, "o")
	openMenuAt(a, "a.md")
	x, y, _, _ := a.menuRect()
	click := func(name string, mod tea.KeyMod) {
		row := -1
		for r, j := range a.menu.shown {
			if a.menu.entries[j].name == name {
				row = r - a.menu.offset
			}
		}
		if row < 0 {
			t.Fatalf("no %s in menu", name)
		}
		a.Update(tea.MouseClickMsg{X: x + 2, Y: y + 1 + row, Button: tea.MouseLeft, Mod: mod})
	}
	click("b.md", tea.ModShift)
	if got := markedNames(a); got != "a.md b.md" {
		t.Fatalf("shift-click marks %q", got)
	}
	click("a.md", tea.ModCtrl)
	if got := markedNames(a); got != "b.md" {
		t.Fatalf("ctrl-click flips to %q", got)
	}
	click("a.md", 0)
	if a.menu != nil || tabNames(a) != "b.md a.md" {
		t.Fatalf("plain click: menu %v tabs %q", a.menu != nil, tabNames(a))
	}
}
