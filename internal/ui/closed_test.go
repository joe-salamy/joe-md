package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// geometry is the current tab's panes with where they are on screen, the
// focused one starred.
func geometry(a *App) string {
	var out []string
	for _, l := range a.tab().leaves() {
		n := l.pane.doc.Name
		if l == a.tab().focus {
			n = "*" + n
		}
		out = append(out, fmt.Sprintf("%s@%d,%d,%dx%d", n, l.x, l.y, l.w, l.h))
	}
	return strings.Join(out, " ")
}

func TestReopenTabs(t *testing.T) {
	dir := fixture(t)
	p := func(f string) string { return filepath.Join(dir, f) }
	a := newTestApp(t, p("a.md"), p("b.md"), p("docs/c.md"), p("docs/sub/d.md"))

	press(a, "X")
	if a.msg != "nothing to reopen" {
		t.Fatalf("X with nothing closed: %q", a.msg)
	}
	press(a, "2", "g", "t", "x") // b
	press(a, "3", "g", "t", "x") // d
	press(a, "g", "t")
	if got := tabNames(a); got != "a.md c.md" {
		t.Fatalf("after closing: %q", got)
	}
	press(a, "X")
	if got := tabNames(a); got != "a.md c.md d.md" || a.cur != 2 {
		t.Fatalf("X reopens where it was: %q, tab %d", got, a.cur+1)
	}
	press(a, "x", "x") // d, then c
	press(a, "2", "X")
	if got := tabNames(a); got != "a.md c.md d.md" || a.cur != 2 {
		t.Fatalf("2X: %q, tab %d", got, a.cur+1)
	}
	press(a, "X")
	if got := tabNames(a); got != "a.md b.md c.md d.md" || a.cur != 1 {
		t.Fatalf("X after 2X: %q, tab %d", got, a.cur+1)
	}
}

func TestReopenPanes(t *testing.T) {
	dir := fixture(t)
	p := func(f string) string { return filepath.Join(dir, f) }
	a := newTestApp(t, p("a.md"))

	// [a | [b / [c | d]]]
	a.openAt(p("b.md"), openVSplit)
	a.openAt(p("docs/c.md"), openHSplit)
	a.openAt(p("docs/sub/d.md"), openVSplit)
	ctrlW(a, ">") // sizes that aren't halves must come back too
	ctrlW(a, "k")
	want := geometry(a)
	if !strings.Contains(want, "*b.md") {
		t.Fatalf("setup: %s", want)
	}

	// Closing b merges [c | d] into the top row; X must rebuild it.
	press(a, "q")
	if got := paneNames(a); got != "a.md *c.md d.md" {
		t.Fatalf("q: %q", got)
	}
	press(a, "X")
	if got := geometry(a); got != want {
		t.Fatalf("X after a merge:\n got %s\nwant %s", got, want)
	}

	// Several panes, most recent first; a count reopens several.
	ctrlW(a, "j", "l") // d
	press(a, "q")
	ctrlW(a, "h") // a
	press(a, "q")
	press(a, "2", "X")
	if got := strings.ReplaceAll(geometry(a), "*", ""); got != strings.ReplaceAll(want, "*", "") {
		t.Fatalf("2X:\n got %s\nwant %s", got, want)
	}
	if a.pane.doc.Name != "d.md" {
		t.Fatalf("2X should end on the last pane reopened, got %s", a.pane.doc.Name)
	}

	// ctrl+w o is undone in one go, keeping the focus.
	ctrlW(a, "o")
	if got := paneNames(a); got != "*d.md" {
		t.Fatalf("ctrl+w o: %q", got)
	}
	press(a, "X")
	if got := geometry(a); got != strings.ReplaceAll(strings.ReplaceAll(want, "*", ""), "d.md", "*d.md") {
		t.Fatalf("X after ctrl+w o: %s", got)
	}

	// A pane closed in a tab that was closed after it comes back after the tab.
	press(a, "q") // d
	a.openAt(p("notes.txt"), openNewTab)
	press(a, "1", "g", "t", "x")
	press(a, "X")
	if got := paneNames(a); got != "a.md b.md *c.md" {
		t.Fatalf("X reopens the tab: %q", got)
	}
	press(a, "X")
	if got := strings.ReplaceAll(geometry(a), "*", ""); got != strings.ReplaceAll(want, "*", "") {
		t.Fatalf("then its pane: %s", got)
	}

	// A pane whose file is gone is skipped, and doesn't use up the count.
	press(a, "q") // d
	ctrlW(a, "k") // b
	press(a, "q")
	if err := os.Remove(p("b.md")); err != nil {
		t.Fatal(err)
	}
	press(a, "X")
	if got := paneNames(a); got != "a.md c.md *d.md" {
		t.Fatalf("X skips a deleted file: %q", got)
	}
}

func TestReopenPaneOfClosedTab(t *testing.T) {
	dir := fixture(t)
	p := func(f string) string { return filepath.Join(dir, f) }
	a := newTestApp(t, p("a.md"), p("docs/c.md"))
	ctrlW(a, "v")
	a.openAt(p("b.md"), openReplace)
	press(a, "q") // the b pane
	press(a, "x")
	a.closed = a.closed[:1] // forget the tab, as if it fell off the stack
	press(a, "X")
	if got := tabNames(a); got != "c.md b.md" || a.cur != 1 {
		t.Fatalf("a pane whose tab is gone gets a new tab: %q, tab %d", got, a.cur+1)
	}
}
