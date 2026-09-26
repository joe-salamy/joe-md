package ui

import (
	"sort"
	"strings"
	"unicode"

	"github.com/joe-salamy/joe-md/internal/doc"
	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/charmbracelet/x/ansi"
)

// Search matches come from ripgrep as source lines, but the pane shows
// glamour's output, where markup is gone and lines are re-wrapped. So each
// match is placed in two steps: the source line picks its block, then the
// matched text is looked up (case-insensitively) in that block's rendered
// lines. If the text is not visible there (say it was a link URL), the line
// estimated from the source position gets a gutter mark instead.

// cellRange is a highlighted run of cells [start, end) on a rendered line.
type cellRange struct{ start, end int }

type matches struct {
	query string
	mode  search.Mode
	lines []int    // 0-based source lines with a match, distinct, in target order
	terms []string // matched text, used to find matches in rendered lines
	cur   int      // current match, or -1

	// Derived from a rendering by index. doc and view are what it was
	// indexed against, so an index built off the UI thread can be checked.
	doc    *doc.Doc
	view   *doc.Rendered
	target []int               // rendered line to jump to for each match
	hits   map[int][]cellRange // rendered line -> highlighted cells
	marks  map[int]bool        // rendered lines with a match not visible as text
}

const maxTerms = 64

// newMatches makes the matches of a search, all in one file. They are not
// yet indexed.
func newMatches(req search.Request, ms []search.Match) *matches {
	lines := make([]int, 0, len(ms))
	var terms []string
	seen := map[string]bool{}
	for _, m := range ms {
		lines = append(lines, m.Line-1)
		for _, t := range m.Terms() {
			if k := strings.ToLower(t); !seen[k] && len(terms) < maxTerms {
				seen[k] = true
				terms = append(terms, t)
			}
		}
	}
	sort.Ints(lines)
	return &matches{query: req.Query, mode: req.Mode, lines: compactInts(lines), terms: terms, cur: -1}
}

// SetMatches highlights search matches in the pane. lines are 0-based source
// lines; terms are the matched strings.
func (p *Pane) SetMatches(query string, lines []int, terms []string) {
	lines = append([]int(nil), lines...)
	sort.Ints(lines)
	if len(terms) > maxTerms {
		terms = terms[:maxTerms]
	}
	p.setMatches(&matches{query: query, lines: compactInts(lines), terms: terms, cur: -1})
}

// setMatches puts m in the pane, indexing it unless it already was, for the
// pane's current rendering. If the pane already shows the same matches it
// keeps them, and its place among them.
func (p *Pane) setMatches(m *matches) {
	if old := p.match; old != nil && old.query == m.query && old.mode == m.mode &&
		old.doc == p.doc && old.view == p.view && sameSet(old.lines, m.lines) {
		return
	}
	p.match = m
	if m.doc != p.doc || m.view != p.view {
		p.indexMatches()
	}
}

// sameSet reports whether a and b hold the same ints; b is sorted.
func sameSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	a = append([]int(nil), a...)
	sort.Ints(a)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p *Pane) ClearMatches() { p.match = nil }

// Query is the search the pane's matches came from, or "".
func (p *Pane) Query() string {
	if p.match == nil {
		return ""
	}
	return p.match.query
}

// MatchPos returns the current match (1-based, 0 if none) and the count.
func (p *Pane) MatchPos() (cur, total int) {
	if p.match == nil {
		return 0, 0
	}
	return p.match.cur + 1, len(p.match.lines)
}

// JumpMatch moves to the n-th match below (n > 0) or above (n < 0) the top of
// the pane, wrapping around the ends. With inclusive, a match exactly at the
// top counts as the first one below. It reports false if there are no matches.
func (p *Pane) JumpMatch(n int, inclusive bool) (wrapped, ok bool) {
	m := p.match
	if m == nil || len(m.target) == 0 || n == 0 {
		return false, false
	}
	for step := 0; step < max(n, -n); step++ {
		var i int
		if n > 0 {
			i = sort.Search(len(m.target), func(i int) bool {
				return m.target[i] > p.offset || inclusive && m.target[i] == p.offset
			})
			if i == len(m.target) {
				i, wrapped = 0, true
			}
		} else {
			i = sort.Search(len(m.target), func(i int) bool { return m.target[i] >= p.offset }) - 1
			if i < 0 {
				i, wrapped = len(m.target)-1, true
			}
		}
		m.cur = i
		p.ScrollTo(m.target[i])
		inclusive = false
	}
	return wrapped, true
}

// GotoMatchLine jumps to the match on 0-based source line src, or to the line
// itself if it has no match.
func (p *Pane) GotoMatchLine(src int) {
	if m := p.match; m != nil && m.target != nil {
		for i, l := range m.lines {
			if l == src {
				m.cur = i
				p.ScrollTo(m.target[i])
				return
			}
		}
	}
	p.GotoSource(src)
}

// indexMatches places the matches in the current rendering.
func (p *Pane) indexMatches() {
	if p.match != nil {
		p.match.index(p.doc, p.view)
	}
}

// index places the matches in rendering v of d. It only reads d and v, so it
// can run in the background on matches nothing else has yet.
func (m *matches) index(d *doc.Doc, v *doc.Rendered) {
	m.doc, m.view = d, v
	m.target, m.hits, m.marks = nil, nil, nil
	if v == nil || len(d.Blocks) == 0 {
		return
	}
	m.target = make([]int, len(m.lines))
	m.hits = map[int][]cellRange{}
	m.marks = map[int]bool{}
	blockHits := map[int][]int{} // block -> rendered lines with visible hits
	for i, src := range m.lines {
		src = max(0, min(src, d.Lines-1))
		b := d.BlockAt(src)
		lines, done := blockHits[b]
		if !done {
			lines = m.findInBlock(v, b)
			blockHits[b] = lines
		}
		est := v.SourceToRendered(d, src)
		if len(lines) == 0 {
			m.target[i] = est
			m.marks[est] = true
			continue
		}
		m.target[i] = nearest(lines, est)
	}
	// Picking the nearest visible hit can reorder matches within a block;
	// JumpMatch's binary search needs the targets sorted.
	idx := make([]int, len(m.lines))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return m.target[idx[a]] < m.target[idx[b]] })
	lines, target := make([]int, len(idx)), make([]int, len(idx))
	for i, j := range idx {
		lines[i], target[i] = m.lines[j], m.target[j]
	}
	m.lines, m.target = lines, target
}

// findInBlock records the highlights in block b and returns the rendered
// lines that have any.
func (m *matches) findInBlock(v *doc.Rendered, b int) []int {
	start, end := v.BlockSpan(b)
	var lines []int
	for r := start; r < end; r++ {
		if hs := findTerms(ansi.Strip(v.Lines[r]), m.terms); len(hs) > 0 {
			m.hits[r] = hs
			lines = append(lines, r)
		}
	}
	return lines
}

// nearest returns the element of sorted xs closest to x, preferring the later
// one on a tie (matches are searched forwards).
func nearest(xs []int, x int) int {
	i := sort.SearchInts(xs, x)
	switch {
	case i == len(xs):
		return xs[i-1]
	case i == 0 || xs[i] == x:
		return xs[i]
	case x-xs[i-1] < xs[i]-x:
		return xs[i-1]
	default:
		return xs[i]
	}
}

// findTerms returns the cells of plain-text line that match any term,
// ignoring case, merged and sorted.
func findTerms(line string, terms []string) []cellRange {
	if line == "" || len(terms) == 0 {
		return nil
	}
	runes := []rune(line)
	folded := make([]rune, len(runes))
	cells := make([]int, len(runes)+1) // cells[i] is the column where rune i starts
	for i, r := range runes {
		folded[i] = unicode.ToLower(r)
		cells[i+1] = cells[i] + ansi.StringWidth(string(r))
	}
	var out []cellRange
	for _, t := range terms {
		term := []rune(strings.ToLower(t))
		if len(term) == 0 {
			continue
		}
		for i := 0; i+len(term) <= len(folded); {
			if equalRunes(folded[i:i+len(term)], term) {
				out = append(out, cellRange{cells[i], cells[i+len(term)]})
				i += len(term)
			} else {
				i++
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	merged := out[:1]
	for _, c := range out[1:] {
		last := &merged[len(merged)-1]
		if c.start <= last.end {
			last.end = max(last.end, c.end)
		} else {
			merged = append(merged, c)
		}
	}
	return merged
}

func equalRunes(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func compactInts(xs []int) []int {
	out := xs[:0]
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// decorate applies search highlights to rendered line r.
func (p *Pane) decorate(r int, s string, th theme) string {
	m := p.match
	if m == nil || m.hits == nil {
		return s
	}
	current := m.cur >= 0 && m.target[m.cur] == r
	if hs := m.hits[r]; len(hs) > 0 {
		style := th.match
		if current {
			style = th.matchCur
		}
		var sb strings.Builder
		prev := 0
		for _, h := range hs {
			sb.WriteString(ansi.Cut(s, prev, h.start))
			sb.WriteString("\x1b[0m")
			sb.WriteString(style.Render(ansi.Strip(ansi.Cut(s, h.start, h.end))))
			prev = h.end
		}
		// TruncateLeft keeps the escape codes before the cut, restoring the
		// line's own style after the last highlight.
		sb.WriteString(ansi.TruncateLeft(s, prev, ""))
		s = sb.String()
	}
	if m.marks[r] {
		style := th.matchMark
		if current {
			style = th.matchMarkCur
		}
		s = style.Render("▌") + "\x1b[0m" + ansi.TruncateLeft(s, 1, "")
	}
	return s
}
