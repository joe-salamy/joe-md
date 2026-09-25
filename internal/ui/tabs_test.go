package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"joe-md/internal/doc"

	tea "charm.land/bubbletea/v2"
)

// fixture makes a small tree:
//
//	a.md  b.md  notes.txt  .hidden.md  docs/c.md  docs/sub/d.md
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"a.md", "b.md", "notes.txt", ".hidden.md", "docs/c.md", "docs/sub/d.md"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# "+f+"\n\ntext\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newTestApp(t *testing.T, paths ...string) *App {
	t.Helper()
	var docs []*doc.Doc
	for _, p := range paths {
		d, err := doc.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, d)
	}
	a := New(docs, Options{Style: "notty", MaxWrap: 80})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return a
}

// press sends keys; each string is one key as msg.String() spells it.
func press(a *App, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch {
		case k == "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case k == "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case strings.HasPrefix(k, "alt+"):
			msg = tea.KeyPressMsg{Code: rune(k[4]), Mod: tea.ModAlt}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd = a.Update(msg)
	}
	return cmd
}

func typeText(a *App, s string) {
	for _, r := range s {
		press(a, string(r))
	}
}

func tabNames(a *App) string {
	var names []string
	for _, t := range a.tabs {
		names = append(names, t.focus.pane.doc.Name)
	}
	return strings.Join(names, " ")
}

func menuNames(a *App) string {
	var names []string
	for _, i := range a.menu.shown {
		names = append(names, a.menu.entries[i].name)
	}
	return strings.Join(names, " ")
}

func TestTabsOpenDedupeClose(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md"))
	if got := tabNames(a); got != "a.md b.md" || a.cur != 0 {
		t.Fatalf("tabs %q cur %d", got, a.cur)
	}

	// A new tab goes after the current one; an open file is switched to.
	a.openAt(filepath.Join(dir, "docs/c.md"), openNewTab)
	if got := tabNames(a); got != "a.md c.md b.md" || a.cur != 1 {
		t.Fatalf("after open: tabs %q cur %d", got, a.cur)
	}
	a.openAt(filepath.Join(dir, "b.md"), openNewTab)
	if len(a.tabs) != 3 || a.cur != 2 {
		t.Fatalf("dedupe: %d tabs, cur %d", len(a.tabs), a.cur)
	}
	a.openAt(filepath.Join(dir, "docs/sub/d.md"), openReplace)
	if got := tabNames(a); got != "a.md c.md d.md" {
		t.Fatalf("replace: tabs %q", got)
	}

	press(a, "g", "t")
	if a.cur != 0 {
		t.Fatalf("gt wraps to 0, got %d", a.cur)
	}
	press(a, "3", "g", "t")
	if a.cur != 2 {
		t.Fatalf("3gt: cur %d", a.cur)
	}
	press(a, "g", "T")
	if a.cur != 1 {
		t.Fatalf("gT: cur %d", a.cur)
	}
	press(a, "alt+1")
	if a.cur != 0 || a.pane != a.tabs[0].focus.pane {
		t.Fatalf("alt+1: cur %d", a.cur)
	}

	// x closes, q closes, and q on the last tab quits.
	press(a, "x")
	if got := tabNames(a); got != "c.md d.md" || a.pane.doc.Name != "c.md" {
		t.Fatalf("x: tabs %q, showing %s", got, a.pane.doc.Name)
	}
	if cmd := press(a, "q"); cmd != nil || tabNames(a) != "d.md" {
		t.Fatalf("q: tabs %q", tabNames(a))
	}
	press(a, "x")
	if len(a.tabs) != 1 {
		t.Fatal("x must not close the last tab")
	}
	if cmd := press(a, "q"); cmd == nil {
		t.Fatal("q on the last tab should quit")
	}
}

func TestMenu(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "b.md"))
	press(a, "o")
	if a.menu == nil || a.menu.dir != dir {
		t.Fatalf("menu not open at %s", dir)
	}
	if got := menuNames(a); got != ".. docs a.md b.md" {
		t.Fatalf("listing %q", got)
	}
	if e, _ := a.menu.current(); e.name != "b.md" {
		t.Fatalf("cursor on %q, want the current file", e.name)
	}

	press(a, ".")
	if got := menuNames(a); got != ".. docs .hidden.md a.md b.md notes.txt" {
		t.Fatalf("show all: %q", got)
	}
	press(a, ".")

	// Filtering then entering directories (this used to index the old listing).
	press(a, "/")
	typeText(a, "doc")
	if got := menuNames(a); got != "docs" {
		t.Fatalf("filter: %q", got)
	}
	press(a, "enter")
	if a.menu.dir != filepath.Join(dir, "docs") || menuNames(a) != ".. sub c.md" {
		t.Fatalf("entered %s: %q", a.menu.dir, menuNames(a))
	}
	press(a, "l")
	if got := menuNames(a); got != ".. d.md" {
		t.Fatalf("sub: %q", got)
	}
	press(a, "h")
	if e, _ := a.menu.current(); e.name != "sub" {
		t.Fatalf("h should land on the directory we came from, got %q", e.name)
	}
	press(a, "j", "l")
	if a.menu != nil || tabNames(a) != "b.md c.md" || a.pane.doc.Name != "c.md" {
		t.Fatalf("open: menu %v, tabs %q", a.menu != nil, tabNames(a))
	}

	// Opening an open file switches to it.
	press(a, "o", "h", "g", "g")
	press(a, "/")
	typeText(a, "b.m")
	press(a, "enter")
	if len(a.tabs) != 2 || a.pane.doc.Name != "b.md" {
		t.Fatalf("dedupe: tabs %q, showing %s", tabNames(a), a.pane.doc.Name)
	}

	// The screen renders with the menu over it.
	press(a, "o")
	if out := a.View().Content; !strings.Contains(out, "╭─") || !strings.Contains(out, "a.md") {
		t.Fatal("menu not drawn")
	}
	press(a, "esc")
	if a.menu != nil {
		t.Fatal("esc should close the menu")
	}
}

func TestStartupMenu(t *testing.T) {
	dir := fixture(t)
	a := New(nil, Options{Style: "notty", MaxWrap: 80, MenuDir: dir})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if a.menu == nil || a.pane != nil {
		t.Fatal("start-up menu should be open with no tabs")
	}
	_ = a.View()
	press(a, "j") // the cursor starts past ../, on docs
	if cmd := press(a, "l"); cmd != nil || a.pane == nil || a.pane.doc.Name != "a.md" {
		t.Fatalf("open from start-up menu: tabs %q", tabNames(a))
	}

	b := New(nil, Options{Style: "notty", MaxWrap: 80, MenuDir: dir})
	if cmd := press(b, "esc"); cmd == nil {
		t.Fatal("closing the start-up menu with no tabs should quit")
	}
}
