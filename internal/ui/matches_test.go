package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/joe-salamy/joe-md/internal/config"
	"github.com/joe-salamy/joe-md/internal/doc"
	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/charmbracelet/x/ansi"
)

func TestFindTerms(t *testing.T) {
	got := findTerms("Setup the SETUP and set", []string{"setup"})
	want := [][2]int{{0, 5}, {10, 15}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// Wide characters take two cells.
	if got = findTerms("日本 go", []string{"go"}); !slices.Equal(got, [][2]int{{5, 7}}) {
		t.Fatalf("wide: %v", got)
	}
	// Overlapping terms are two occurrences.
	if got = findTerms("abcdef", []string{"cde", "abc"}); !slices.Equal(got, [][2]int{{0, 3}, {2, 5}}) {
		t.Fatalf("overlap: %v", got)
	}
}

func TestFindPatterns(t *testing.T) {
	pats, err := search.Request{Query: `\bup\b 日本`}.Patterns()
	if err != nil {
		t.Fatal(err)
	}
	// Cells, not bytes: the wide characters take two cells each. Each
	// occurrence is its own, in order.
	got := findPatterns([]string{"setup 日本 Up 日本"}, pats)
	want := [][]piece{{{0, 6, 10}}, {{0, 11, 13}}, {{0, 14, 18}}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("got %v want %v", got, want)
	}
	// Empty matches are not highlights.
	pats, _ = search.Request{Query: "x*"}.Patterns()
	if got := findPatterns([]string{"abc"}, pats); len(got) != 0 {
		t.Fatalf("empty matches: %v", got)
	}
	// Two terms matching the same text are one occurrence.
	pats, _ = search.Request{Query: "ab a."}.Patterns()
	if got := findPatterns([]string{"ab ab"}, pats); len(got) != 2 {
		t.Fatalf("same text: %v", got)
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
	want := [][]piece{
		{{0, 6, 15}, {1, 2, 8}},
		{{1, 14, 23}, {2, 2, 8}},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// Highlights follow the query, not just the strings ripgrep matched.
func TestMatchesUsePatterns(t *testing.T) {
	p := testPane(t, "setup is up and UP\n")
	line := func(req search.Request, spans ...[2]int) []cellRange {
		m := newMatches(req, []search.Match{{Line: 1, Text: "setup is up and UP", Spans: spans}})
		p.setMatches(m)
		return m.hits[m.occs[0].line]
	}
	hitText := func(hs []cellRange) []string {
		plain := ansi.Strip(p.view.Lines[p.match.occs[0].line])
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

// setTestMatches highlights matches in p as a search would: lines are 0-based
// source lines, terms the matched strings.
func setTestMatches(p *Pane, query string, lines []int, terms []string) {
	lines = slices.Compact(slices.Sorted(slices.Values(lines)))
	p.setMatches(&matches{query: query, lines: lines, terms: terms, cur: -1})
}

func TestMatchesHighlightAndJump(t *testing.T) {
	src := "# Title\n\nalpha **needle** beta\n\ngamma\n\n<span title=\"needle\">x</span> y\n\nlast needle\n"
	p := testPane(t, src)
	// rg would report lines 3, 7 and 9 (1-based).
	setTestMatches(p, "needle", []int{2, 6, 8}, []string{"needle"})

	m := p.match
	if len(m.occs) != 3 {
		t.Fatalf("occurrences %v", m.occs)
	}
	// Bold markup is gone in the rendering but the word is still found.
	if hs := m.hits[m.occs[0].line]; len(hs) != 1 {
		t.Fatalf("line %q hits %v", ansi.Strip(p.view.Lines[m.occs[0].line]), hs)
	}
	// The HTML attribute is not visible: the match gets a gutter mark instead.
	if !m.marks[m.occs[1].line] {
		t.Fatalf("expected a gutter mark on %d, marks %v", m.occs[1].line, m.marks)
	}

	// n from the top goes to each match in turn, then wraps.
	for i := range 3 {
		if wrapped, ok := p.JumpMatch(1, i == 0); !ok || wrapped {
			t.Fatalf("jump %d: ok=%v wrapped=%v", i, ok, wrapped)
		}
		if cur, _ := p.MatchPos(); cur != i+1 || !p.visible(m.occs[i].line) || p.offset > p.maxOffset() {
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
	line := p.decorate(m.occs[0].line, p.view.Lines[m.occs[0].line], newTheme(true, "dark", config.Theme{}))
	if ansi.Strip(line) != ansi.Strip(p.view.Lines[m.occs[0].line]) {
		t.Fatalf("decorate changed the text:\n%q\n%q", ansi.Strip(line), ansi.Strip(p.view.Lines[m.occs[0].line]))
	}
	if !strings.Contains(ansi.Strip(p.decorate(m.occs[1].line, p.view.Lines[m.occs[1].line], newTheme(true, "dark", config.Theme{}))), "▌") {
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
	setTestMatches(p, "needle", []int{0, 2, 40, 58}, []string{"needle"})
	m := p.match
	if len(m.occs) != 4 {
		t.Fatalf("occurrences %v", m.occs)
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
	if p.match.cur != 2 || p.offset != m.occs[2].line {
		t.Fatalf("n out of view: cur %d offset %d", p.match.cur, p.offset)
	}
	// The last match can't reach the top: the view stops at the end.
	p.JumpMatch(1, false)
	if p.match.cur != 3 || p.offset != p.maxOffset() || !p.visible(m.occs[3].line) {
		t.Fatalf("n at the end: cur %d offset %d max %d", p.match.cur, p.offset, p.maxOffset())
	}
	// Wrapping counts from the current match, not the top of the view.
	if wrapped, _ := p.JumpMatch(1, false); !wrapped || p.match.cur != 0 || p.offset != m.occs[0].line {
		t.Fatalf("wrap: cur %d offset %d", p.match.cur, p.offset)
	}
	if wrapped, _ := p.JumpMatch(-1, false); !wrapped || p.match.cur != 3 || p.offset != p.maxOffset() {
		t.Fatalf("N wrap: cur %d offset %d", p.match.cur, p.offset)
	}
	// Once the current match is scrolled out of view, n counts from the top.
	p.ScrollTo(m.occs[1].line + 1)
	p.JumpMatch(1, false)
	if p.match.cur != 2 {
		t.Fatalf("n after scrolling away: cur %d", p.match.cur)
	}
	// A new search below the view scrolls to the first match below the top.
	p.ScrollTo(m.occs[1].line + 1)
	p.match.cur = -1
	p.JumpMatch(1, true)
	if p.match.cur != 2 || p.offset != m.occs[2].line {
		t.Fatalf("new search out of view: cur %d offset %d", p.match.cur, p.offset)
	}
	// A counted jump steps from the current match.
	p.JumpMatch(-2, false)
	if p.match.cur != 0 || p.offset != m.occs[0].line {
		t.Fatalf("2N: cur %d offset %d", p.match.cur, p.offset)
	}
	// Opening a result in view doesn't scroll; one out of view is capped.
	p.GotoMatch(2, 0)
	if p.match.cur != 1 || p.offset != m.occs[0].line {
		t.Fatalf("result in view: cur %d offset %d", p.match.cur, p.offset)
	}
	p.GotoMatch(58, 0)
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
	if len(m.occs) != 1 || len(m.marks) != 0 {
		t.Fatalf("occurrences %v marks %v", m.occs, m.marks)
	}
	start := m.occs[0].line
	var text []string
	for _, r := range []int{start, start + 1} {
		hs := m.hits[r]
		if len(hs) != 1 || hs[0].occ != 0 {
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

// Overlapping spans are highlighted as one run, not dropped.
func TestHighlightSpansOverlap(t *testing.T) {
	th := newTheme(true, "dark", config.Theme{})
	got := highlightSpans("abcdefgh", [][2]int{{1, 4}, {2, 6}}, th)
	want := th.bar.Render("a") + th.match.Render("bcd") + th.bar.Render("") + th.match.Render("ef") + th.bar.Render("gh")
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// Each occurrence is a match of its own: n steps through the two on one line,
// only the current one takes the strong colour, and a result opens its own.
func TestMatchesStepThroughOccurrences(t *testing.T) {
	src := "one needle and nonneedle\n\ntwo needle\n"
	p := testPane(t, src)
	req := search.Request{Query: "needle"}
	p.setMatches(newMatches(req, []search.Match{
		{Line: 1, Text: "one needle and nonneedle", Spans: [][2]int{{4, 10}, {18, 24}}},
		{Line: 3, Text: "two needle", Spans: [][2]int{{4, 10}}},
	}))
	m := p.match
	if _, total := p.MatchPos(); total != 3 {
		t.Fatalf("occurrences %v", m.occs)
	}
	want := []occurrence{{line: m.occs[0].line, col: m.occs[0].col, src: 0, nth: 0}, {src: 0, nth: 1}, {src: 2, nth: 0}}
	for i, o := range m.occs {
		if o.src != want[i].src || o.nth != want[i].nth {
			t.Fatalf("occurrence %d: %+v", i, o)
		}
	}
	if m.occs[0].line != m.occs[1].line || m.occs[0].col >= m.occs[1].col {
		t.Fatalf("both on one line, in order: %v", m.occs)
	}

	th := newTheme(true, "dark", config.Theme{})
	r := m.occs[0].line
	p.JumpMatch(1, true)
	p.JumpMatch(1, false)
	if cur, _ := p.MatchPos(); cur != 2 {
		t.Fatalf("n should reach the second on the line, cur %d", cur)
	}
	line := p.decorate(r, p.view.Lines[r], th)
	hs := m.hits[r]
	if len(hs) != 2 {
		t.Fatalf("hits %v", hs)
	}
	first, second := ansi.Cut(line, hs[0].start, hs[0].end), ansi.Cut(line, hs[1].start, hs[1].end)
	if strings.Contains(first, th.matchCur.Render(ansi.Strip(first))) || !strings.Contains(second, th.matchCur.Render(ansi.Strip(second))) {
		t.Fatalf("only the second should be current:\n%q\n%q", first, second)
	}
	if src, nth, _ := p.CurrentMatch(); src != 0 || nth != 1 {
		t.Fatalf("current match %d/%d", src, nth)
	}

	p.GotoMatch(0, 1)
	if p.match.cur != 1 {
		t.Fatalf("result 0/1: cur %d", p.match.cur)
	}
	p.GotoMatch(2, 0)
	if p.match.cur != 2 {
		t.Fatalf("result 2/0: cur %d", p.match.cur)
	}
}
