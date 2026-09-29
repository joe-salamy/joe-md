package ui

import (
	"cmp"
	"regexp"
	"slices"
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
// query's terms are matched again, as Go regexps, in that block's rendered
// lines, searched as one text so a match can run across a wrap. If they match
// nothing there (say the term is anchored with ^), the
// strings ripgrep matched are looked up instead, ignoring case. If the text is
// not visible either way (say it was a link URL), the line estimated from the
// source position gets a gutter mark.

// cellRange is a highlighted run of cells [start, end) on a rendered line.
// from is the rendered line its match starts on, earlier for the rest of a
// match that wrapped.
type cellRange struct{ start, end, from int }

type matches struct {
	query string
	mode  search.Mode
	lines []int            // 0-based source lines with a match, distinct, in target order
	pats  []*regexp.Regexp // the query's terms, to find matches in rendered lines
	terms []string         // matched text, the fallback when pats find nothing
	cur   int              // current match, or -1

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
	slices.Sort(lines)
	pats, _ := req.Patterns() // on an error, the matched strings will do
	return &matches{query: req.Query, mode: req.Mode, lines: slices.Compact(lines), pats: pats, terms: terms, cur: -1}
}

// setMatches puts m in the pane, indexing it unless it already was, for the
// pane's current rendering. If the pane already shows the same matches it
// keeps them, and its place among them.
func (p *Pane) setMatches(m *matches) {
	if old := p.match; old != nil && old.query == m.query && old.mode == m.mode &&
		old.doc == p.doc && old.view == p.view && slices.Equal(slices.Sorted(slices.Values(old.lines)), m.lines) {
		return
	}
	p.match = m
	if m.doc != p.doc || m.view != p.view {
		p.indexMatches()
	}
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

// JumpMatch moves to the n-th match after (n > 0) or before (n < 0) the
// current one, wrapping around the ends, and scrolls only if that match is
// out of view. If the current match is out of view (or there is none), it
// counts from the top of the pane instead. With inclusive, it is a new search:
// the first match counts from the top, so a match in view is not scrolled to.
// It reports false if there are no matches.
func (p *Pane) JumpMatch(n int, inclusive bool) (wrapped, ok bool) {
	m := p.match
	if m == nil || len(m.target) == 0 || n == 0 {
		return false, false
	}
	count := len(m.target)
	var i int
	switch {
	case !inclusive && m.cur >= 0 && m.cur < count && p.visible(m.target[m.cur]):
		i = m.cur + sign(n)
	case n > 0:
		i = sort.Search(count, func(i int) bool {
			return m.target[i] > p.offset || inclusive && m.target[i] == p.offset
		})
	default:
		i = sort.Search(count, func(i int) bool { return m.target[i] >= p.offset }) - 1
	}
	i += n - sign(n)
	wrapped = i < 0 || i >= count
	m.cur = (i%count + count) % count
	p.reveal(m.target[m.cur])
	return wrapped, true
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

// visible reports whether rendered line r is in view.
func (p *Pane) visible(r int) bool { return r >= p.offset && r < p.offset+p.height }

// GotoMatchLine jumps to the match on 0-based source line src (scrolling
// only if it is out of view), or to the line itself if it has no match.
func (p *Pane) GotoMatchLine(src int) {
	if m := p.match; m != nil && m.target != nil {
		for i, l := range m.lines {
			if l == src {
				m.cur = i
				p.reveal(m.target[i])
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
	type pair struct{ line, target int }
	ps := make([]pair, len(m.lines))
	for i := range ps {
		ps[i] = pair{m.lines[i], m.target[i]}
	}
	slices.SortStableFunc(ps, func(a, b pair) int { return cmp.Compare(a.target, b.target) })
	for i, p := range ps {
		m.lines[i], m.target[i] = p.line, p.target
	}
}

// findInBlock records the highlights in block b and returns the rendered
// lines that have any. The query's patterns come first; the matched strings
// only if the patterns find nothing in the block.
func (m *matches) findInBlock(v *doc.Rendered, b int) []int {
	start, end := v.BlockSpan(b)
	plain := make([]string, end-start)
	for r := start; r < end; r++ {
		plain[r-start] = ansi.Strip(v.Lines[r])
	}
	// The lines where matches start, and each line's highlights.
	record := func(hits map[int][]cellRange) []int {
		var lines []int
		for i, hs := range hits {
			for j := range hs {
				hs[j].from += start
				lines = append(lines, hs[j].from)
			}
			m.hits[start+i] = hs
		}
		slices.Sort(lines)
		return slices.Compact(lines)
	}
	if len(m.pats) > 0 {
		if hits := findPatterns(plain, m.pats); len(hits) > 0 {
			return record(hits)
		}
	}
	hits := map[int][]cellRange{}
	for i, l := range plain {
		if hs := findTerms(l, m.terms); len(hs) > 0 {
			for j := range hs {
				hs[j].from = i
			}
			hits[i] = hs
		}
	}
	return record(hits)
}

// nearest returns the element of sorted xs closest to x, preferring the later
// one on a tie (matches are searched forwards).
func nearest(xs []int, x int) int {
	i, found := slices.BinarySearch(xs, x)
	switch {
	case i == len(xs):
		return xs[i-1]
	case i == 0 || found:
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
			if slices.Equal(folded[i:i+len(term)], term) {
				out = append(out, cellRange{start: cells[i], end: cells[i+len(term)]})
				i += len(term)
			} else {
				i++
			}
		}
	}
	return mergeRanges(out)
}

// findPatterns returns the cells that match any pattern in the plain-text
// lines of a block, by index into lines, each line's merged and sorted. The
// lines are searched as one text, so a match can run across a wrap; each line
// gets its part, less the wrap's padding and indent. Empty matches are
// skipped.
func findPatterns(lines []string, pats []*regexp.Regexp) map[int][]cellRange {
	if len(pats) == 0 {
		return nil
	}
	// Glamour pads lines to the wrap width; the padding is not text.
	starts := make([]int, len(lines)) // byte offset of each line in text
	var sb strings.Builder
	for i, l := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		lines[i] = strings.TrimRight(l, " ")
		starts[i] = sb.Len()
		sb.WriteString(lines[i])
	}
	text := sb.String()
	cells := make([][]int, len(lines)) // cells[i][b] is the column of byte b of line i, once needed
	col := func(i, b int) int {
		if cells[i] == nil {
			cells[i] = make([]int, len(lines[i])+1)
			c := 0
			for j, r := range lines[i] {
				cells[i][j] = c
				c += ansi.StringWidth(string(r))
			}
			cells[i][len(lines[i])] = c
		}
		return cells[i][b]
	}
	out := map[int][]cellRange{}
	for _, re := range pats {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if loc[0] == loc[1] {
				continue
			}
			from := -1
			first, _ := slices.BinarySearch(starts, loc[0]+1)
			for i := first - 1; i < len(lines) && starts[i] < loc[1]; i++ {
				l := lines[i]
				s, e := max(loc[0]-starts[i], 0), min(loc[1]-starts[i], len(l))
				if loc[0] < starts[i] { // continued from the line above
					s += len(l[s:]) - len(strings.TrimLeft(l[s:], " \u00a0"))
				}
				if loc[1] > starts[i]+len(l) { // goes on to the line below
					e = len(strings.TrimRight(l[:e], " \u00a0"))
				}
				if s >= e {
					continue
				}
				if from < 0 {
					from = i
				}
				out[i] = append(out[i], cellRange{col(i, s), col(i, e), from})
			}
		}
	}
	for i := range out {
		out[i] = mergeRanges(out[i])
	}
	return out
}

// mergeRanges sorts out and merges the ranges that overlap.
func mergeRanges(out []cellRange) []cellRange {
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, func(a, b cellRange) int { return cmp.Compare(a.start, b.start) })
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

// decorate applies search highlights to rendered line r.
func (p *Pane) decorate(r int, s string, th theme) string {
	m := p.match
	if m == nil || m.hits == nil {
		return s
	}
	cur := -1
	if m.cur >= 0 {
		cur = m.target[m.cur]
	}
	current := cur == r
	if hs := m.hits[r]; len(hs) > 0 {
		var sb strings.Builder
		prev := 0
		for _, h := range hs {
			// The current match is highlighted on every line it wraps onto.
			style := th.match
			if h.from == cur {
				style = th.matchCur
			}
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
