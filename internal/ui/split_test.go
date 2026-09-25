package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func ctrlW(a *App, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		a.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
		cmd = press(a, k)
	}
	return cmd
}

func paneNames(a *App) string {
	var names []string
	for _, l := range a.tab().leaves() {
		n := l.pane.doc.Name
		if l == a.tab().focus {
			n = "*" + n
		}
		names = append(names, n)
	}
	return strings.Join(names, " ")
}

func TestSplitFocusClose(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "a.md"))

	ctrlW(a, "v")
	if got := paneNames(a); got != "a.md *a.md" {
		t.Fatalf("vsplit: %q", got)
	}
	l, r := a.tab().leaves()[0], a.tab().leaves()[1]
	if l.x+l.w+1 != r.x || l.h != r.h || r.x+r.w != a.width {
		t.Fatalf("vsplit geometry: %+v %+v", *l, *r)
	}
	if l.pane == r.pane || l.pane.doc != r.pane.doc {
		t.Fatal("a split should be a new pane on the same document")
	}

	// A split from the menu opens a file below, even one open elsewhere.
	a.openAt(filepath.Join(dir, "b.md"), openHSplit)
	if got := paneNames(a); got != "a.md a.md *b.md" {
		t.Fatalf("hsplit: %q", got)
	}
	a.openAt(filepath.Join(dir, "b.md"), openHSplit)
	if got := paneNames(a); got != "a.md a.md b.md *b.md" {
		t.Fatalf("hsplit of an open file: %q", got)
	}
	ctrlW(a, "c")

	// Focus moves geometrically; h past the left edge reaches the sidebar.
	ctrlW(a, "k")
	if got := paneNames(a); got != "a.md *a.md b.md" {
		t.Fatalf("ctrl+w k: %q", got)
	}
	ctrlW(a, "h")
	if got := paneNames(a); got != "*a.md a.md b.md" {
		t.Fatalf("ctrl+w h: %q", got)
	}
	ctrlW(a, "h")
	if a.focus != focusTOC {
		t.Fatal("ctrl+w h from the leftmost pane should focus the sidebar")
	}
	ctrlW(a, "l")
	if a.focus != focusDoc || a.pane != a.tab().leaves()[0].pane {
		t.Fatal("ctrl+w l from the sidebar should return to the pane")
	}

	// Opening a tab dedupes into the pane that shows the file.
	press(a, "o")
	a.menu = nil
	a.openAt(filepath.Join(dir, "docs/c.md"), openNewTab)
	a.openAt(filepath.Join(dir, "b.md"), openNewTab)
	if a.cur != 0 || a.pane.doc.Name != "b.md" || len(a.tabs) != 2 {
		t.Fatalf("dedupe into pane: tab %d showing %s", a.cur, a.pane.doc.Name)
	}

	// q closes panes, then the tab; the tab label counts the other panes.
	if !strings.Contains(a.tabLabels()[0], "+2") {
		t.Fatalf("label %q", a.tabLabels()[0])
	}
	press(a, "q")
	if got := paneNames(a); got != "a.md *a.md" {
		t.Fatalf("q closes the pane: %q", got)
	}
	press(a, "q", "q")
	if len(a.tabs) != 1 || a.tab().multi() {
		t.Fatalf("q q: %d tabs", len(a.tabs))
	}
}

func TestSplitNesting(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "a.md"))
	ctrlW(a, "v", "s") // right half split in two
	if got := paneNames(a); got != "a.md a.md *a.md" {
		t.Fatalf("%q", got)
	}
	root := a.tab().root
	if !root.vert || len(root.kids) != 2 || root.kids[1].vert || len(root.kids[1].kids) != 2 {
		t.Fatal("want vert[leaf, stack[leaf, leaf]]")
	}
	// Closing one of the stacked panes collapses the stack.
	ctrlW(a, "c")
	if root := a.tab().root; !root.vert || len(root.kids) != 2 || root.kids[1].pane == nil {
		t.Fatal("stack should collapse back to a leaf")
	}
	ctrlW(a, "v") // same direction: a third column, not a nested node
	if root := a.tab().root; len(root.kids) != 3 {
		t.Fatalf("want 3 columns, got %d", len(root.kids))
	}
	ctrlW(a, "o")
	if a.tab().multi() {
		t.Fatal("ctrl+w o should leave one pane")
	}
}

func TestResizeAndDrag(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "a.md"))
	a.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}) // no sidebar: panes start at column 0
	if a.showTOC {
		t.Fatal("ctrl+t should hide the sidebar")
	}
	ctrlW(a, "v")
	l := a.tab().leaves()[0]
	w := l.w
	press(a, "5")
	ctrlW(a, ">")
	if r := a.tab().leaves()[1]; r.w != 100-w-1+5 {
		t.Fatalf("5 ctrl+w >: right pane %d wide, left was %d", r.w, w)
	}
	ctrlW(a, "=")
	if l.w != w {
		t.Fatalf("ctrl+w =: %d, want %d", l.w, w)
	}
	// Drag the separator to column 30.
	y := a.tabBarHeight() + 2
	a.Update(tea.MouseClickMsg{X: l.x + l.w, Y: y, Button: tea.MouseLeft})
	if a.drag == nil {
		t.Fatal("click on the separator should start a drag")
	}
	a.Update(tea.MouseMotionMsg{X: 30, Y: y, Button: tea.MouseLeft})
	a.Update(tea.MouseReleaseMsg{X: 30, Y: y, Button: tea.MouseLeft})
	if l.w != 30 || a.drag != nil {
		t.Fatalf("drag: left pane %d wide", l.w)
	}
	// Clicking a pane focuses it.
	a.Update(tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft})
	if a.tab().focus != l {
		t.Fatal("click should focus the pane")
	}
}

func TestScrollBindAndReload(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := range 200 {
		sb.WriteString("line " + strings.Repeat("x", i%7) + "\n\n")
	}
	path := filepath.Join(dir, "long.md")
	os.WriteFile(path, []byte(sb.String()), 0o644)
	a := newTestApp(t, path)
	ctrlW(a, "v")
	ctrlW(a, "b")
	ctrlW(a, "h")
	ctrlW(a, "b")
	l, r := a.tab().leaves()[0].pane, a.tab().leaves()[1].pane
	r.ScrollTo(3)
	press(a, "j", "j")
	if l.offset != 2 || r.offset != 5 {
		t.Fatalf("scrollbind: %d %d", l.offset, r.offset)
	}
	ctrlW(a, "b")
	press(a, "j")
	if r.offset != 5 {
		t.Fatal("unbound pane should not follow")
	}

	// Reloading updates every pane showing the file.
	os.WriteFile(path, []byte("# new\n"), 0o644)
	press(a, "r")
	if l.doc.Lines != r.doc.Lines || !strings.Contains(string(r.doc.Source), "new") {
		t.Fatal("reload should reach both panes")
	}
}
