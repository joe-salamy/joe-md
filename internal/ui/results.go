package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/charmbracelet/x/ansi"
)

const resultsMaxRows = 10

// Results is the search results panel, a list like vim's quickfix window.
type Results struct {
	list
	req search.Request
	res search.Result
}

// Height is the panel height on a screen of the given height, including the
// title row.
func (r *Results) Height(screen int) int {
	return min(len(r.res.Matches), resultsMaxRows, max(screen/3, 1)) + 1
}

// Select moves the cursor to row i and scrolls it into a panel of rows rows.
func (r *Results) Select(i, rows int) { r.selectIdx(i, len(r.res.Matches), rows) }

func (r *Results) Scroll(n, rows int) { r.scroll(n, len(r.res.Matches), rows) }

func (r *Results) Current() search.Match { return r.res.Matches[r.cursor] }

// At returns the result shown at row y of the panel, or -1.
func (r *Results) At(y int) int {
	i := r.offset + y - 1
	if y < 1 || i >= len(r.res.Matches) {
		return -1
	}
	return i
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
	n, files := len(r.res.Matches), r.res.Files
	title := " " + plural(n, "match", "matches") + " in " + plural(files, "file", "files")
	if r.res.Truncated {
		title += " (limit reached)"
	}
	title += " · " + r.req.Scope.String() + " " + tildePath(r.req.Root) + " "
	title = th.separator.Render("─") + th.resultsTitle.Render(ansi.Truncate(title, width-2, "…"))
	out = append(out, fit(title+th.separator.Render(strings.Repeat("─", max(0, width-ansi.StringWidth(title)))), width))

	locW := 0
	for i := r.offset; i < n && i < r.offset+height-1; i++ {
		locW = max(locW, ansi.StringWidth(r.location(i)))
	}
	locW = min(locW, max(width/3, 8))
	for i := r.offset; i < n && len(out) < height; i++ {
		loc := ansi.Truncate(r.location(i), locW, "…")
		pad := strings.Repeat(" ", locW-ansi.StringWidth(loc))
		text, spans := trimMatch(r.res.Matches[i])
		if i == r.cursor {
			row := " " + loc + pad + "  " + text
			style := th.tocCurrent
			if focused {
				style = th.tocCursor
			}
			out = append(out, style.Render(fit(ansi.Truncate(row, width, ""), width)))
			continue
		}
		path, line, _ := strings.Cut(loc, ":")
		row := " " + th.resultsPath.Render(path) + th.resultsLine.Render(":"+line) + pad + "  " +
			highlightSpans(text, spans, th)
		out = append(out, fit(row, width))
	}
	for len(out) < height {
		out = append(out, strings.Repeat(" ", width))
	}
	return out
}

// location is "path:line", the path relative to the search root.
func (r *Results) location(i int) string {
	m := r.res.Matches[i]
	return relPath(r.req, m.Path) + ":" + strconv.Itoa(m.Line)
}

func relPath(req search.Request, path string) string {
	if req.Scope == search.File {
		return filepath.Base(path)
	}
	if rel, err := filepath.Rel(req.Root, path); err == nil {
		return rel
	}
	return path
}

// trimMatch strips leading indentation and tabs from a match's text, shifting
// its spans to suit.
func trimMatch(m search.Match) (string, [][2]int) {
	text := strings.ReplaceAll(m.Text, "\t", " ") // same byte length, spans still valid
	cut := len(text) - len(strings.TrimLeft(text, " "))
	spans := make([][2]int, 0, len(m.Spans))
	for _, s := range m.Spans {
		if s[1] > cut {
			spans = append(spans, [2]int{max(s[0]-cut, 0), s[1] - cut})
		}
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
