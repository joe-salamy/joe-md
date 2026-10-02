package ui

import (
	"cmp"
	"maps"
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
//
// Each occurrence found is a match of its own: n / N step through them and
// the count is theirs, so a line with a term twice is two matches, as it is
// two rows in the results list.

// cellRange is a highlighted run of cells [start, end) on a rendered line,
// part of occurrence occ. An occurrence that wraps has a range on each line.
type cellRange struct{ start, end, occ int }

// occurrence is one match in the rendering. line and col are where it
// starts; src and nth tie it to the nth match ripgrep found on source line
// src, so it can be found from a result and the other way round.
type occurrence struct {
	line, col int
	src, nth  int
	mark      bool // not visible as text: a gutter mark on line instead
}

// piece is a run of cells [start, end) of a rendered line, part of an
// occurrence.
type piece struct{ line, start, end int }

type matches struct {
	query string
	mode  search.Mode
	lines []int            // 0-based source lines with a match, distinct, sorted
	count map[int]int      // matches ripgrep found on each source line, if more than one
	pats  []*regexp.Regexp // the query's terms, to find matches in rendered lines
	terms []string         // matched text, the fallback when pats find nothing
	cur   int              // current occurrence, or -1

	// Derived from a rendering by index. doc and view are what it was
	// indexed against, so an index built off the UI thread can be checked.
	doc   *doc.Doc
	view  *doc.Rendered
	occs  []occurrence        // in rendering order
	hits  map[int][]cellRange // rendered line -> highlighted cells, by start
	marks map[int]bool        // rendered lines with a match not visible as text
}

const maxTerms = 64

// newMatches makes the matches of a search, all in one file. They are not
// yet indexed.
func newMatches(req search.Request, ms []search.Match) *matches {
	lines := make([]int, 0, len(ms))
	count := map[int]int{}
	var terms []string
	seen := map[string]bool{}
	for _, m := range ms {
		lines = append(lines, m.Line-1)
		if n := len(m.Spans); n > 1 {
			count[m.Line-1] = n
		}
		for _, t := range m.Terms() {
			if k := strings.ToLower(t); !seen[k] && len(terms) < maxTerms {
				seen[k] = true
				terms = append(terms, t)
			}
		}
	}
	slices.Sort(lines)
	pats, _ := req.Patterns() // on an error, the matched strings will do
	return &matches{query: req.Query, mode: req.Mode, lines: slices.Compact(lines), count: count, pats: pats, terms: terms, cur: -1}
}

// setMatches puts m in the pane, indexing it unless it already was, for the
// pane's current rendering. If the pane already shows the same matches it
// keeps them, and its place among them.
func (p *Pane) setMatches(m *matches) {
	if old := p.match; old != nil && old.query == m.query && old.mode == m.mode &&
		old.doc == p.doc && old.view == p.view && slices.Equal(old.lines, m.lines) && maps.Equal(old.count, m.count) {
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
	return p.match.cur + 1, len(p.match.occs)
}

// CurrentMatch is the source line (0-based) and the index among that line's
// matches of the current match.
func (p *Pane) CurrentMatch() (src, nth int, ok bool) {
	m := p.match
	if m == nil || m.cur < 0 || m.cur >= len(m.occs) {
		return 0, 0, false
	}
	return m.occs[m.cur].src, m.occs[m.cur].nth, true
}

// JumpMatch moves to the n-th match after (n > 0) or before (n < 0) the
// current one, wrapping around the ends, and scrolls only if that match is
// out of view. If the current match is out of view (or there is none), it
// counts from the top of the pane instead. With inclusive, it is a new search:
// the first match counts from the top, so a match in view is not scrolled to.
// It reports false if there are no matches.
func (p *Pane) JumpMatch(n int, inclusive bool) (wrapped, ok bool) {
	m := p.match
	if m == nil || len(m.occs) == 0 || n == 0 {
		return false, false
	}
	count := len(m.occs)
	var i int
	switch {
	case !inclusive && m.cur >= 0 && m.cur < count && p.visible(m.occs[m.cur].line):
		i = m.cur + sign(n)
	case n > 0:
		i = sort.Search(count, func(i int) bool {
			return m.occs[i].line > p.offset || inclusive && m.occs[i].line == p.offset
		})
	default:
		i = sort.Search(count, func(i int) bool { return m.occs[i].line >= p.offset }) - 1
	}
	i += n - sign(n)
	wrapped = i < 0 || i >= count
	m.cur = (i%count + count) % count
	p.reveal(m.occs[m.cur].line)
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

// GotoMatch jumps to the nth match on 0-based source line src (scrolling
// only if it is out of view), or to the line itself if it has no match.
func (p *Pane) GotoMatch(src, nth int) {
	if m := p.match; m != nil && len(m.occs) > 0 {
		m.cur = m.find(src, nth)
		p.reveal(m.occs[m.cur].line)
		return
	}
	p.GotoSource(src)
}

// find returns the occurrence for the nth match on source line src: that
// one if it was placed, else the line's last before it, else the one nearest
// where the line renders.
func (m *matches) find(src, nth int) int {
	best := nthOnLine(len(m.occs), nth, func(i int) (bool, int) { return m.occs[i].src == src, m.occs[i].nth })
	if best >= 0 {
		return best
	}
	est := m.view.SourceToRendered(m.doc, max(0, min(src, m.doc.Lines-1)))
	best = 0
	for i, o := range m.occs {
		if abs(o.line-est) <= abs(m.occs[best].line-est) {
			best = i
		}
	}
	return best
}

func abs(n int) int { return max(n, -n) }

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
	m.occs, m.hits, m.marks = nil, nil, nil
	if v == nil || len(d.Blocks) == 0 {
		return
	}
	m.hits = map[int][]cellRange{}
	m.marks = map[int]bool{}
	block := func(i int) int { return d.BlockAt(max(0, min(m.lines[i], d.Lines-1))) }
	var occs []occurrence
	var found [][]piece // each occurrence's pieces
	for i := 0; i < len(m.lines); {
		b := block(i)
		j := i + 1
		for j < len(m.lines) && block(j) == b {
			j++
		}
		srcs := m.lines[i:j]
		i = j
		pieces := m.findInBlock(v, b)
		if len(pieces) == 0 {
			for _, s := range srcs {
				est := v.SourceToRendered(d, max(0, min(s, d.Lines-1)))
				occs = append(occs, occurrence{line: est, src: s, mark: true})
				found = append(found, nil)
			}
			continue
		}
		// The block's occurrences go to its source lines in order, as many
		// to each line as ripgrep found on it; any extra go to the last.
		k, nth := 0, 0
		for _, ps := range pieces {
			if nth >= max(m.count[srcs[k]], 1) && k < len(srcs)-1 {
				k, nth = k+1, 0
			}
			occs = append(occs, occurrence{line: ps[0].line, col: ps[0].start, src: srcs[k], nth: nth})
			found = append(found, ps)
			nth++
		}
	}
	// Blocks render in order, so this only settles ties; JumpMatch's binary
	// search needs the occurrences sorted.
	order := make([]int, len(occs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(occs[a].line, occs[b].line), cmp.Compare(occs[a].col, occs[b].col))
	})
	m.occs = make([]occurrence, len(occs))
	for n, i := range order {
		m.occs[n] = occs[i]
		if occs[i].mark {
			m.marks[occs[i].line] = true
		}
		for _, p := range found[i] {
			m.hits[p.line] = append(m.hits[p.line], cellRange{p.start, p.end, n})
		}
	}
	for _, hs := range m.hits {
		slices.SortStableFunc(hs, func(a, b cellRange) int { return cmp.Compare(a.start, b.start) })
	}
}

// findInBlock returns the occurrences in block b, in order, each as its
// pieces on rendered lines. The query's patterns come first; the matched
// strings only if the patterns find nothing in the block.
func (m *matches) findInBlock(v *doc.Rendered, b int) [][]piece {
	start, end := v.BlockSpan(b)
	plain := make([]string, end-start)
	for r := start; r < end; r++ {
		plain[r-start] = ansi.Strip(v.Lines[r])
	}
	var out [][]piece
	if len(m.pats) > 0 {
		out = findPatterns(plain, m.pats)
	}
	if len(out) == 0 {
		for i, l := range plain {
			for _, c := range findTerms(l, m.terms) {
				out = append(out, []piece{{i, c[0], c[1]}})
			}
		}
	}
	for _, ps := range out {
		for j := range ps {
			ps[j].line += start
		}
	}
	return out
}

// findTerms returns the cells [start, end) of plain-text line that match any
// term, ignoring case, one per occurrence, sorted.
func findTerms(line string, terms []string) [][2]int {
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
	var out [][2]int
	for _, t := range terms {
		term := []rune(strings.ToLower(t))
		if len(term) == 0 {
			continue
		}
		for i := 0; i+len(term) <= len(folded); {
			if slices.Equal(folded[i:i+len(term)], term) {
				out = append(out, [2]int{cells[i], cells[i+len(term)]})
				i += len(term)
			} else {
				i++
			}
		}
	}
	slices.SortFunc(out, func(a, b [2]int) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) })
	return slices.Compact(out)
}

// findPatterns returns the occurrences of any pattern in the plain-text lines
// of a block, each as its pieces by index into lines, in order of where they
// start. The lines are searched as one text, so a match can run across a
// wrap; each line gets its part, less the wrap's padding and indent. Empty
// matches are skipped, and two patterns matching the same text count once.
func findPatterns(lines []string, pats []*regexp.Regexp) [][]piece {
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
	var out [][]piece
	for _, re := range pats {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if loc[0] == loc[1] {
				continue
			}
			var ps []piece
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
				if s < e {
					ps = append(ps, piece{i, col(i, s), col(i, e)})
				}
			}
			if len(ps) > 0 {
				out = append(out, ps)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b []piece) int {
		return cmp.Or(cmp.Compare(a[0].line, b[0].line), cmp.Compare(a[0].start, b[0].start), cmp.Compare(a[0].end, b[0].end))
	})
	return slices.CompactFunc(out, func(a, b []piece) bool { return slices.Equal(a, b) })
}

// decorate applies search highlights to rendered line r.
func (p *Pane) decorate(r int, s string, th theme) string {
	m := p.match
	if m == nil || m.hits == nil {
		return s
	}
	if hs := m.hits[r]; len(hs) > 0 {
		var sb strings.Builder
		prev := 0
		for _, h := range hs {
			if h.end <= prev {
				continue // inside an overlapping occurrence before it
			}
			// The current match is highlighted on every line it wraps onto.
			style := th.match
			if h.occ == m.cur {
				style = th.matchCur
			}
			start := max(h.start, prev)
			sb.WriteString(ansi.Cut(s, prev, start))
			sb.WriteString("\x1b[0m")
			sb.WriteString(style.Render(ansi.Strip(ansi.Cut(s, start, h.end))))
			prev = h.end
		}
		// TruncateLeft keeps the escape codes before the cut, restoring the
		// line's own style after the last highlight.
		sb.WriteString(ansi.TruncateLeft(s, prev, ""))
		s = sb.String()
	}
	if m.marks[r] {
		style := th.matchMark
		if m.cur >= 0 && m.cur < len(m.occs) && m.occs[m.cur].mark && m.occs[m.cur].line == r {
			style = th.matchMarkCur
		}
		s = style.Render("▌") + "\x1b[0m" + ansi.TruncateLeft(s, 1, "")
	}
	return s
}
