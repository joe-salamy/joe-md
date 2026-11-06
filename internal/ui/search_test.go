package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/config"
	"github.com/joe-salamy/joe-md/internal/doc"
	"github.com/joe-salamy/joe-md/internal/search"

	tea "charm.land/bubbletea/v2"
)

// A cross-file search that completes after the last tab closed must not
// dereference the nil focused pane.
func TestSearchDoneWithNoPaneDoesNotPanic(t *testing.T) {
	dir := fixture(t)
	path := filepath.Join(dir, "a.md")
	a := newTestApp(t, path)
	p := a.pane
	if a.closeTab() {
		t.Fatal("closing the last tab should report nothing left")
	}
	if a.pane != nil {
		t.Fatal("no pane should be left")
	}
	a.seq = 1
	req := search.Request{Query: "text", Scope: search.Dir, Root: dir}
	res := search.Result{Files: 1, Matches: []search.Match{{Path: path, Line: 3, Text: "text", Spans: [][2]int{{0, 4}}}}}
	a.update(searchDoneMsg{seq: 1, pane: p, req: req, res: res})
}

// An empty cross-file search leaves nothing to kill, so q still closes the
// pane (and quits on the last one) in a single press.
func TestEmptyCrossSearchIsNotActive(t *testing.T) {
	dir := fixture(t)
	a := newTestApp(t, filepath.Join(dir, "a.md"))
	p := a.pane
	req := search.Request{Query: "nowhere", Scope: search.Dir, Root: dir}
	a.seq++
	a.update(searchDoneMsg{seq: a.seq, pane: p, req: req, res: search.Result{}, pre: indexed(req, nil, p.doc, p.view)})
	if a.searchActive() {
		t.Fatal("an empty search should not count as active")
	}
	if cmd := press(a, "q"); cmd == nil {
		t.Fatal("q should close the last pane")
	}
}

// Opening a result indexes its matches off the UI thread and then jumps.
func TestOpenResultIndexesInBackground(t *testing.T) {
	dir := fixture(t)
	path := filepath.Join(dir, "a.md")
	a := newTestApp(t, path)
	req := search.Request{Query: "text", Scope: search.File, Root: path}
	res := search.Result{Files: 1, Matches: []search.Match{{Path: path, Line: 3, Text: "text", Spans: [][2]int{{0, 4}}}}}
	a.results = newResults(req, res)
	cmd := a.openResult(openNewTab)
	if cmd == nil {
		t.Fatal("openResult should return an indexing command")
	}
	a.update(cmd())
	if a.pane.match == nil || len(a.pane.match.occs) == 0 {
		t.Fatal("matches were not placed")
	}
	if src, _, ok := a.pane.CurrentMatch(); !ok || src != 2 {
		t.Fatalf("current match = %d, %v; want source line 2", src, ok)
	}
}

// The zero-value Options still gets a usable wrap width.
func TestMaxWrapZero(t *testing.T) {
	d, err := doc.Load(filepath.Join(fixture(t), "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	a := New([]*doc.Doc{d}, Options{Style: "notty"})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if a.pane.view == nil || a.pane.wrap < minWrap {
		t.Fatalf("view %v, wrap %d", a.pane.view != nil, a.pane.wrap)
	}
}

// Terminal control sequences in a filename or a document must not reach the
// screen: they are replaced with spaces at display time.
func TestInjectedSequencesAreNotDrawn(t *testing.T) {
	const evil = "\x1b]52;c;ZXZpbA==\x07"
	dir := t.TempDir()
	path := filepath.Join(dir, "bad"+evil+".md")
	if err := os.WriteFile(path, []byte("# title\n\n"+evil+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := doc.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := New([]*doc.Doc{d}, Options{Style: "notty", MaxWrap: 80})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for name, s := range map[string]string{
		"status": a.statusLine(),
		"tabs":   strings.Join(a.tabLabels(), ""),
		"view":   a.View().Content,
	} {
		if strings.Contains(s, "\x1b]52") {
			t.Errorf("%s contains the injected sequence", name)
		}
	}
	press(a, "o")
	if out := strings.Join(a.menuView(), "\n"); strings.Contains(out, "\x1b]52") {
		t.Error("menu contains the injected sequence")
	}

	th := newTheme(true, "notty", config.Theme{})
	res := search.Result{Files: 1, Matches: []search.Match{{
		Path: path, Line: 1, Text: "x" + evil + "y", Spans: [][2]int{{0, 1}},
	}}}
	r := newResults(search.Request{Query: "x", Scope: search.File, Root: path}, res)
	if out := strings.Join(r.Render(80, 3, false, th), "\n"); strings.Contains(out, "\x1b]52") {
		t.Error("results contain the injected sequence")
	}
}

func TestSanitizeLine(t *testing.T) {
	if got := sanitizeLine("a\x1b[31mb\nc\td"); strings.ContainsAny(got, "\x1b\n\t") {
		t.Fatalf("sanitizeLine = %q", got)
	}
	if got := stripControlBytes("a\x1b[31mb"); len(got) != 7 || strings.Contains(got, "\x1b") {
		t.Fatalf("stripControlBytes = %q (len %d)", got, len(got))
	}
}
