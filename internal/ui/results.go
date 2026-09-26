package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joe-salamy/joe-md/internal/search"

	"github.com/charmbracelet/x/ansi"
)

const resultsMaxRows = 10

// Results is the search results panel, a list like vim's quickfix window.
type Results struct {
	req    search.Request
	res    search.Result
	cursor int
	offset int
}

// Height is the panel height on a screen of the given height, including the
// title row.
func (r *Results) Height(screen int) int {
	return min(len(r.res.Matches), resultsMaxRows, max(screen/3, 1)) + 1
}

// Select moves the cursor to row i and scrolls it into a panel of rows rows.
func (r *Results) Select(i, rows int) {
	r.cursor = max(0, min(i, len(r.res.Matches)-1))
	if r.cursor < r.offset {
		r.offset = r.cursor
	} else if r.cursor >= r.offset+rows {
		r.offset = r.cursor - rows + 1
	}
}

func (r *Results) Scroll(n, rows int) {
	r.offset = max(0, min(r.offset+n, len(r.res.Matches)-rows))
}

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
		if samePath(m.Path, path) {
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

func highlightSpans(text string, spans [][2]int, th theme) string {
	var sb strings.Builder
	prev := 0
	for _, s := range spans {
		if s[0] < prev {
			continue
		}
		sb.WriteString(th.bar.Render(text[prev:s[0]]))
		sb.WriteString(th.match.Render(text[s[0]:s[1]]))
		prev = s[1]
	}
	sb.WriteString(th.bar.Render(text[prev:]))
	return sb.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if strings.HasPrefix(p, home+string(filepath.Separator)) {
			return "~" + p[len(home):]
		}
	}
	return p
}

// samePath reports whether a and b name the same file.
func samePath(a, b string) bool {
	if a == b {
		return true
	}
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && aa == bb
}
