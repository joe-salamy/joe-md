package ui

import (
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/config"
	"github.com/joe-salamy/joe-md/internal/doc"
	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/charmbracelet/x/ansi"
)

func TestFindTerms(t *testing.T) {
	got := findTerms("Setup the SETUP and set", []string{"setup"})
	want := []cellRange{{start: 0, end: 5}, {start: 10, end: 15}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
	// Wide characters take two cells.
	got = findTerms("日本 go", []string{"go"})
	if len(got) != 1 || got[0] != (cellRange{start: 5, end: 7}) {
		t.Fatalf("wide: %v", got)
	}
	// Overlapping terms merge.
	got = findTerms("abcdef", []string{"abc", "cde"})
	if len(got) != 1 || got[0] != (cellRange{start: 0, end: 5}) {
		t.Fatalf("merge: %v", got)
	}
}

func TestFindPatterns(t *testing.T) {
	pats, err := search.Request{Query: `\bup\b 日本`}.Patterns()
	if err != nil {
		t.Fatal(err)
	}
	// Cells, not bytes: the wide characters take two cells each.
	got := findPatterns([]string{"setup 日本 Up"}, pats)
	want := []cellRange{{6, 10, 0}, {11, 13, 0}}
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != want[0] || got[0][1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
	// Empty matches are not highlights.
	pats, _ = search.Request{Query: "x*"}.Patterns()
	if got := findPatterns([]string{"abc"}, pats); len(got) != 0 {
		t.Fatalf("empty matches: %v", got)
	}
}

// A phrase that wraps is highlighted on both lines, less the padding at the
// end of the first and the indent of the second.
func TestFindPatternsAcrossWrap(t *testing.T) {
	pats, err := search.Request{Query: "one fixed string", Mode: search.Mode{Literal: true}}.Patterns()
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		"  say one fixed     ",
		"  string, and one\u00a0fixed\u00a0",
		"  string here",
	}
	got := findPatterns(lines, pats)
	want := map[int][]cellRange{
		0: {{6, 15, 0}},
		1: {{2, 8, 0}, {14, 23, 1}},
		2: {{2, 8, 1}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i, w := range want {
		g := got[i]
		if len(g) != len(w) {
			t.Fatalf("line %d: got %v want %v", i, g, w)
		}
		for j := range w {
			if g[j] != w[j] {
				t.Fatalf("line %d: got %v want %v", i, g, w)
			}
		}
	}
}

// Highlights follow the query, not just the strings ripgrep matched.
func TestMatchesUsePatterns(t *testing.T) {
	p := testPane(t, "setup is up and UP\n")
	line := func(req search.Request, spans ...[2]int) []cellRange {
		m := newMatches(req, []search.Match{{Line: 1, Text: "setup is up and UP", Spans: spans}})
		p.setMatches(m)
		return m.hits[m.target[0]]
	}
	hitText := func(hs []cellRange) []string {
		plain := ansi.Strip(p.view.Lines[p.match.target[0]])
		var out []string
		for _, h := range hs {
			out = append(out, ansi.Cut(plain, h.start, h.end))
		}
		return out
	}

	// \b keeps "setup" plain; sensitive case keeps "UP" plain.
	hs := line(search.Request{Query: `\bup\b`, Mode: search.Mode{Case: search.MatchCase}}, [2]int{9, 11})
	if got := hitText(hs); len(got) != 1 || got[0] != "up" {
		t.Fatalf("sensitive \\bup\\b: %q", got)
	}
	// Ignoring case, "UP" is a match too.
	hs = line(search.Request{Query: `\bup\b`}, [2]int{9, 11}, [2]int{16, 18})
	if got := hitText(hs); len(got) != 2 || got[0] != "up" || got[1] != "UP" {
		t.Fatalf("ignore case: %q", got)
	}
	// ^ anchors the source line, not the indented rendering: fall back to the
	// matched strings.
	hs = line(search.Request{Query: `^setup`}, [2]int{0, 5})
	if got := hitText(hs); len(got) != 1 || got[0] != "setup" {
		t.Fatalf("fallback: %q", got)
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
		if cur, _ := p.MatchPos(); cur != i+1 || !p.visible(m.target[i]) || p.offset > p.maxOffset() {
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
	line := p.decorate(m.target[0], p.view.Lines[m.target[0]], newTheme(true, "dark", config.Theme{}))
	if ansi.Strip(line) != ansi.Strip(p.view.Lines[m.target[0]]) {
		t.Fatalf("decorate changed the text:\n%q\n%q", ansi.Strip(line), ansi.Strip(p.view.Lines[m.target[0]]))
	}
	if !strings.Contains(ansi.Strip(p.decorate(m.target[1], p.view.Lines[m.target[1]], newTheme(true, "dark", config.Theme{}))), "▌") {
		t.Fatal("gutter mark not drawn")
	}
}

// Jumps to a match scroll only when it is out of view, and never past the end.
func TestMatchJumpsScrollOnlyWhenNeeded(t *testing.T) {
	var sb strings.Builder
	for i := range 30 {
		if i == 0 || i == 1 || i == 20 || i == 29 {
			sb.WriteString("needle\n\n")
		} else {
			sb.WriteString("hay\n\n")
		}
	}
	p := testPane(t, sb.String())
	p.SetMatches("needle", []int{0, 2, 40, 58}, []string{"needle"})
	m := p.match
	if len(m.target) != 4 {
		t.Fatalf("targets %v", m.target)
	}

	// A new search with matches in view highlights the uppermost, in place.
	p.JumpMatch(1, true)
	if p.match.cur != 0 || p.offset != 0 {
		t.Fatalf("new search: cur %d offset %d", p.match.cur, p.offset)
	}
	// n to a match in view doesn't scroll either.
	p.JumpMatch(1, false)
	if p.match.cur != 1 || p.offset != 0 {
		t.Fatalf("n in view: cur %d offset %d", p.match.cur, p.offset)
	}
	// n to a match out of view puts it at the top.
	p.JumpMatch(1, false)
	if p.match.cur != 2 || p.offset != m.target[2] {
		t.Fatalf("n out of view: cur %d offset %d", p.match.cur, p.offset)
	}
	// The last match can't reach the top: the view stops at the end.
	p.JumpMatch(1, false)
	if p.match.cur != 3 || p.offset != p.maxOffset() || !p.visible(m.target[3]) {
		t.Fatalf("n at the end: cur %d offset %d max %d", p.match.cur, p.offset, p.maxOffset())
	}
	// Wrapping counts from the current match, not the top of the view.
	if wrapped, _ := p.JumpMatch(1, false); !wrapped || p.match.cur != 0 || p.offset != m.target[0] {
		t.Fatalf("wrap: cur %d offset %d", p.match.cur, p.offset)
	}
	if wrapped, _ := p.JumpMatch(-1, false); !wrapped || p.match.cur != 3 || p.offset != p.maxOffset() {
		t.Fatalf("N wrap: cur %d offset %d", p.match.cur, p.offset)
	}
	// Once the current match is scrolled out of view, n counts from the top.
	p.ScrollTo(m.target[1] + 1)
	p.JumpMatch(1, false)
	if p.match.cur != 2 {
		t.Fatalf("n after scrolling away: cur %d", p.match.cur)
	}
	// A new search below the view scrolls to the first match below the top.
	p.ScrollTo(m.target[1] + 1)
	p.match.cur = -1
	p.JumpMatch(1, true)
	if p.match.cur != 2 || p.offset != m.target[2] {
		t.Fatalf("new search out of view: cur %d offset %d", p.match.cur, p.offset)
	}
	// A counted jump steps from the current match.
	p.JumpMatch(-2, false)
	if p.match.cur != 0 || p.offset != m.target[0] {
		t.Fatalf("2N: cur %d offset %d", p.match.cur, p.offset)
	}
	// Opening a result in view doesn't scroll; one out of view is capped.
	p.GotoMatchLine(2)
	if p.match.cur != 1 || p.offset != m.target[0] {
		t.Fatalf("result in view: cur %d offset %d", p.match.cur, p.offset)
	}
	p.GotoMatchLine(58)
	if p.match.cur != 3 || p.offset != p.maxOffset() {
		t.Fatalf("result at the end: cur %d offset %d", p.match.cur, p.offset)
	}
}

// A literal phrase is highlighted across markup and a wrap, and the current
// match takes the strong colour on both of its lines.
func TestMatchesPhraseAcrossWrap(t *testing.T) {
	src := "aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj one **fixed** string here\n"
	p := testPane(t, src)
	req := search.Request{Query: "one fixed string", Mode: search.Mode{Literal: true}}
	p.setMatches(newMatches(req, []search.Match{{Line: 1, Text: strings.TrimSpace(src), Spans: [][2]int{{50, 70}}}}))
	m := p.match
	if len(m.target) != 1 || len(m.marks) != 0 {
		t.Fatalf("targets %v marks %v", m.target, m.marks)
	}
	start := m.target[0]
	var text []string
	for _, r := range []int{start, start + 1} {
		hs := m.hits[r]
		if len(hs) != 1 || hs[0].from != start {
			t.Fatalf("line %d hits %v", r, hs)
		}
		text = append(text, ansi.Cut(ansi.Strip(p.view.Lines[r]), hs[0].start, hs[0].end))
	}
	if got := strings.Join(text, " "); got != "one fixed string" {
		t.Fatalf("highlighted %q", got)
	}
	p.JumpMatch(1, true)
	th := newTheme(true, "dark", config.Theme{})
	for _, r := range []int{start, start + 1} {
		h := m.hits[r][0]
		cut := ansi.Cut(p.decorate(r, p.view.Lines[r], th), h.start, h.end)
		if !strings.Contains(cut, th.matchCur.Render(ansi.Strip(cut))) {
			t.Fatalf("line %d not in the current colour: %q", r, cut)
		}
	}
}
