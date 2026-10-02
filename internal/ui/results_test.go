package ui

import (
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/config"
	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/charmbracelet/x/ansi"
)

// A line with two matches is two rows, each highlighting its own, and every
// row shows its line number, however long the path.
func TestResultsRowPerOccurrence(t *testing.T) {
	long := "/notes/" + strings.Repeat("a very long file name ", 5) + ".md"
	res := search.Result{Files: 1, Matches: []search.Match{
		{Path: long, Line: 14, Text: "an assaultive aspect, nonassaultive conduct", Spans: [][2]int{{3, 13}, {25, 35}}},
		{Path: long, Line: 123, Text: "  assaultive", Spans: [][2]int{{2, 12}}},
	}}
	th := newTheme(true, "dark", config.Theme{})
	for _, scope := range []search.Scope{search.File, search.Dir} {
		r := newResults(search.Request{Query: "assaultive", Scope: scope, Root: "/notes"}, res)
		if r.Len() != 3 {
			t.Fatalf("%v: rows %v", scope, r.rows)
		}
		r.cursor = -1 // no row selected, so every row is highlighted
		out := r.Render(60, 4, false, th)
		for i, want := range []string{"14", "14", "123"} {
			row := ansi.Strip(out[i+1])
			if !strings.Contains(row, want+"  ") {
				t.Errorf("%v row %d should show line %s: %q", scope, i, want, row)
			}
		}
		if hl := th.match.Render("assaultive"); strings.Count(out[1], hl)+strings.Count(out[2], hl) != 2 {
			t.Errorf("%v: each row should highlight only its own match:\n%q\n%q", scope, out[1], out[2])
		}
		r.cursor = 1
		if m, nth := r.Current(); m.Line != 14 || nth != 1 {
			t.Errorf("%v: row 1 is line %d match %d", scope, m.Line, nth)
		}
		if i := r.Find(long, 14, 1); i != 1 {
			t.Errorf("%v: find 14/1 = %d", scope, i)
		}
	}
}

// A match past the edge of the panel scrolls into view.
func TestScrollTo(t *testing.T) {
	text := strings.Repeat("x", 50) + " needle"
	got, spans := scrollTo(text, [][2]int{{51, 57}}, 20)
	if !strings.HasPrefix(got, "…") || got[spans[0][0]:spans[0][1]] != "needle" || ansi.StringWidth(got[:spans[0][1]]) > 20 {
		t.Fatalf("got %q %v", got, spans)
	}
	if got, _ := scrollTo("a needle", [][2]int{{2, 8}}, 20); got != "a needle" {
		t.Fatalf("in view: %q", got)
	}
}

// The selected row is highlighted all the way across, not just under its
// text.
func TestResultsCursorRowFullWidth(t *testing.T) {
	res := search.Result{Files: 1, Matches: []search.Match{{Path: "/n/a.md", Line: 1, Text: "foo", Spans: [][2]int{{0, 3}}}}}
	th := newTheme(true, "dark", config.Theme{})
	r := newResults(search.Request{Query: "foo", Scope: search.File, Root: "/n/a.md"}, res)
	row := r.Render(40, 2, true, th)[1]
	if ansi.StringWidth(row) != 40 || strings.Contains(row, "\x1b[0m ") {
		t.Errorf("highlight stops early: %q", row)
	}
}

// A match inside the indentation trimMatch strips keeps its place, so each
// row still highlights its own match.
func TestTrimMatchKeepsSpanOrder(t *testing.T) {
	text, spans := trimMatch(search.Match{Text: "    foo", Spans: [][2]int{{0, 2}, {4, 7}}})
	if text != "foo" || len(spans) != 2 || spans[0] != [2]int{0, 0} || spans[1] != [2]int{0, 3} {
		t.Errorf("got %q %v", text, spans)
	}
}
