package ui

import (
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/config"
	"github.com/joe-salamy/joe-md/internal/doc"

	"github.com/charmbracelet/x/ansi"
)

func TestFindTerms(t *testing.T) {
	got := findTerms("Setup the SETUP and set", []string{"setup"})
	want := []cellRange{{0, 5}, {10, 15}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
	// Wide characters take two cells.
	got = findTerms("日本 go", []string{"go"})
	if len(got) != 1 || got[0] != (cellRange{5, 7}) {
		t.Fatalf("wide: %v", got)
	}
	// Overlapping terms merge.
	got = findTerms("abcdef", []string{"abc", "cde"})
	if len(got) != 1 || got[0] != (cellRange{0, 5}) {
		t.Fatalf("merge: %v", got)
	}
}

func TestNearest(t *testing.T) {
	xs := []int{2, 6, 10}
	for x, want := range map[int]int{0: 2, 2: 2, 4: 6, 5: 6, 7: 6, 8: 10, 99: 10} {
		if got := nearest(xs, x); got != want {
			t.Errorf("nearest(%d) = %d, want %d", x, got, want)
		}
	}
}

func testPane(t *testing.T, src string) *Pane {
	t.Helper()
	d := doc.Parse([]byte(src))
	d.Path, d.Name = "t.md", "t.md"
	p := NewPane(d)
	if err := p.Layout(doc.NewRenderer("dark"), 60, 5, 60); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMatchesHighlightAndJump(t *testing.T) {
	src := "# Title\n\nalpha **needle** beta\n\ngamma\n\n<span title=\"needle\">x</span> y\n\nlast needle\n"
	p := testPane(t, src)
	// rg would report lines 3, 7 and 9 (1-based).
	p.SetMatches("needle", []int{2, 6, 8}, []string{"needle"})

	m := p.match
	if len(m.target) != 3 {
		t.Fatalf("targets %v", m.target)
	}
	// Bold markup is gone in the rendering but the word is still found.
	if hs := m.hits[m.target[0]]; len(hs) != 1 {
		t.Fatalf("line %q hits %v", ansi.Strip(p.view.Lines[m.target[0]]), hs)
	}
	// The HTML attribute is not visible: the match gets a gutter mark instead.
	if !m.marks[m.target[1]] {
		t.Fatalf("expected a gutter mark on %d, marks %v", m.target[1], m.marks)
	}

	// n from the top goes to each match in turn, then wraps.
	for i := range 3 {
		if wrapped, ok := p.JumpMatch(1, i == 0); !ok || wrapped {
			t.Fatalf("jump %d: ok=%v wrapped=%v", i, ok, wrapped)
		}
		if cur, _ := p.MatchPos(); cur != i+1 || p.offset != m.target[i] {
			t.Fatalf("jump %d: cur %d offset %d", i, cur, p.offset)
		}
	}
	if wrapped, _ := p.JumpMatch(1, false); !wrapped || p.match.cur != 0 {
		t.Fatal("n at the last match should wrap to the first")
	}
	if wrapped, _ := p.JumpMatch(-1, false); !wrapped || p.match.cur != 2 {
		t.Fatal("N at the first match should wrap to the last")
	}

	// The current highlight is drawn, and the rest of the line survives.
	line := p.decorate(m.target[0], p.view.Lines[m.target[0]], newTheme(true, config.Theme{}))
	if ansi.Strip(line) != ansi.Strip(p.view.Lines[m.target[0]]) {
		t.Fatalf("decorate changed the text:\n%q\n%q", ansi.Strip(line), ansi.Strip(p.view.Lines[m.target[0]]))
	}
	if !strings.Contains(ansi.Strip(p.decorate(m.target[1], p.view.Lines[m.target[1]], newTheme(true, config.Theme{}))), "▌") {
		t.Fatal("gutter mark not drawn")
	}
}
