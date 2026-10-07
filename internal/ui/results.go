package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/charmbracelet/x/ansi"
)

const resultsMaxRows = 10

// Results is the search results panel, a list like vim's quickfix window. It
// has a row for each occurrence: a line with two matches is two rows.
type Results struct {
	list
	req  search.Request
	res  search.Result
	rows []resultRow
}

// resultRow is the nth match on line res.Matches[match].
type resultRow struct{ match, nth int }

func newResults(req search.Request, res search.Result) *Results {
	r := &Results{req: req, res: res}
	for i, m := range res.Matches {
		for n := range max(len(m.Spans), 1) {
			r.rows = append(r.rows, resultRow{i, n})
		}
	}
	return r
}

// Len is the number of results, one per occurrence.
func (r *Results) Len() int { return len(r.rows) }

// Height is the panel height on a screen of the given height, including the
// title row.
func (r *Results) Height(screen int) int {
	return min(len(r.rows), resultsMaxRows, max(screen/3, 1)) + 1
}

// Select moves the cursor to row i and scrolls it into a panel of rows rows.
func (r *Results) Select(i, rows int) { r.selectIdx(i, len(r.rows), rows) }

func (r *Results) Scroll(n, rows int) { r.scroll(n, len(r.rows), rows) }

// Current is the selected result: its line, and which match on it.
func (r *Results) Current() (search.Match, int) {
	row := r.rows[r.cursor]
	return r.res.Matches[row.match], row.nth
}

// At returns the result shown at row y of the panel, or -1.
func (r *Results) At(y int) int {
	i := r.offset + y - 1
	if y < 1 || i >= len(r.rows) {
		return -1
	}
	return i
}

// Find returns the row of the nth match on line (1-based) of path, or of the
// line's last match if it has fewer, or -1.
func (r *Results) Find(path string, line, nth int) int {
	return nthOnLine(len(r.rows), nth, func(i int) (bool, int) {
		m := r.res.Matches[r.rows[i].match]
		return m.Path == path && m.Line == line, r.rows[i].nth
	})
}

// nthOnLine finds the nth match on a line among n items: the item that is
// it, else the line's last before it, else its first, or -1 if the line has
// none. on reports whether item i is on the line, and which of the line's
// matches it is.
func nthOnLine(n, nth int, on func(i int) (bool, int)) int {
	found := -1
	for i := range n {
		if ok, k := on(i); ok && (found < 0 || k <= nth) {
			found = i
		}
	}
	return found
}

// InFile returns the matches in path.
func (r *Results) InFile(path string) []search.Match {
	var out []search.Match
	for _, m := range r.res.Matches {
		if m.Path == path {
			out = append(out, m)
		}
	}
	return out
}

func (r *Results) Render(width, height int, focused bool, th theme) []string {
	out := make([]string, 0, height)
	n, files := len(r.rows), r.res.Files
	title := " " + plural(n, "match", "matches") + " in " + plural(files, "file", "files")
	if r.res.Truncated {
		title += " (limit reached)"
	}
	title += " · " + r.req.Scope.String() + " " + openRoot(r.req) + " "
	title = th.separator.Render("─") + th.resultsTitle.Render(ansi.Truncate(title, width-2, "…"))
	out = append(out, fit(title+th.separator.Render(strings.Repeat("─", max(0, width-ansi.StringWidth(title)))), width))

	// The line numbers always show; long paths lose their end instead.
	pathW, lineW := 0, 0
	for i := r.offset; i < n && i < r.offset+height-1; i++ {
		path, line := r.location(i)
		pathW, lineW = max(pathW, ansi.StringWidth(path)), max(lineW, len(line))
	}
	pathW = min(pathW, max(width/3-lineW, 8))
	for i := r.offset; i < n && len(out) < height; i++ {
		path, line := r.location(i)
		path = ansi.Truncate(path, pathW, "…")
		var loc string
		if path == "" { // a file search: line numbers right-aligned
			loc = strings.Repeat(" ", lineW-len(line)) + line
		} else {
			loc = path + line + strings.Repeat(" ", pathW-ansi.StringWidth(path)+lineW-len(line))
		}
		row := r.rows[i]
		text, spans := trimMatch(r.res.Matches[row.match])
		var span [][2]int
		if row.nth < len(spans) {
			span = spans[row.nth : row.nth+1]
		}
		text, span = scrollTo(text, span, width-ansi.StringWidth(loc)-3)
		if i == r.cursor {
			style := th.tocCurrent
			if focused {
				style = th.tocCursor
			}
			// Padded before styling, not with fit, whose reset would end the
			// highlight where the text does.
			l := ansi.Truncate(" "+loc+"  "+text, width, "")
			out = append(out, style.Render(l+strings.Repeat(" ", max(width-ansi.StringWidth(l), 0))))
			continue
		}
		row2 := " " + th.resultsPath.Render(path) + th.resultsLine.Render(loc[len(path):]) + "  " +
			highlightSpans(text, span, th)
		out = append(out, fit(row2, width))
	}
	for len(out) < height {
		out = append(out, strings.Repeat(" ", width))
	}
	return out
}

// location is row i's path, relative to the search root, and ":line". A file
// search has one file, named in the title, so only the line shows.
func (r *Results) location(i int) (path, line string) {
	m := r.res.Matches[r.rows[i].match]
	if r.req.Scope == search.File {
		return "", strconv.Itoa(m.Line)
	}
	return relPath(r.req, m.Path), ":" + strconv.Itoa(m.Line)
}

// scrollTo drops the start of text, behind "…", if the first of spans would
// end past w cells, keeping a little of the text before it.
func scrollTo(text string, spans [][2]int, w int) (string, [][2]int) {
	if len(spans) == 0 || w <= 0 || ansi.StringWidth(text[:spans[0][1]]) <= w {
		return text, spans
	}
	cut := spans[0][0]
	for range w / 4 {
		if cut == 0 {
			break
		}
		_, size := utf8.DecodeLastRuneInString(text[:cut])
		cut -= size
	}
	const ellipsis = "…"
	shift := len(ellipsis) - cut
	out := make([][2]int, 0, len(spans))
	for _, s := range spans {
		if s[1] > cut {
			out = append(out, [2]int{max(s[0], cut) + shift, s[1] + shift})
		}
	}
	return ellipsis + text[cut:], out
}

func relPath(req search.Request, path string) string {
	if req.Scope == search.File {
		return filepath.Base(path)
	}
	if root := relBase(req); root != "" {
		if rel, err := filepath.Rel(root, path); err == nil {
			return rel
		}
	}
	return path
}

// relBase is the directory match paths show relative to: the root, or for an
// open search with files in no shared directory, none.
func relBase(req search.Request) string {
	if req.Scope != search.Open || len(req.Files) != 1 {
		return req.Root
	}
	return filepath.Dir(req.Files[0])
}

// openRoot is the results title's location: the root, or for an open search
// of one file the file itself.
func openRoot(req search.Request) string {
	if req.Scope == search.Open && len(req.Files) == 1 {
		return tildePath(req.Files[0])
	}
	return tildePath(req.Root)
}

// trimMatch strips leading indentation and tabs from a match's text, shifting
// its spans to suit. A span wholly inside the indentation becomes empty
// rather than going, so the spans still line up with the rows' nth.
func trimMatch(m search.Match) (string, [][2]int) {
	text := strings.ReplaceAll(m.Text, "\t", " ") // same byte length, spans still valid
	cut := len(text) - len(strings.TrimLeft(text, " "))
	spans := make([][2]int, len(m.Spans))
	for i, s := range m.Spans {
		spans[i] = [2]int{max(s[0]-cut, 0), max(s[1]-cut, 0)}
	}
	return text[cut:], spans
}

// highlightSpans styles the spans of text, which are sorted by start but
// may overlap.
func highlightSpans(text string, spans [][2]int, th theme) string {
	var sb strings.Builder
	prev := 0
	for _, s := range spans {
		if s[1] <= prev {
			continue // inside the span before
		}
		start := max(s[0], prev)
		sb.WriteString(th.bar.Render(text[prev:start]))
		sb.WriteString(th.match.Render(text[start:s[1]]))
		prev = s[1]
	}
	sb.WriteString(th.bar.Render(text[prev:]))
	return sb.String()
}
