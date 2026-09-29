package ui

import (
	"strings"

	"github.com/joe-salamy/joe-md/internal/doc"

	"github.com/charmbracelet/x/ansi"
)

const (
	tocMinWidth = 18
	tocMaxWidth = 40
	tocHeader   = 1 // rows above the entries
)

// TOC is the table-of-contents sidebar for the focused document.
type TOC struct {
	list // over the headings
}

// Width is the sidebar width for d on a screen of the given width.
func (t *TOC) Width(d *doc.Doc, screen int) int {
	want := len(" Contents ")
	minLevel := minHeadingLevel(d)
	for _, h := range d.Headings {
		want = max(want, 2*(h.Level-minLevel)+ansi.StringWidth(h.Text)+3)
	}
	return max(tocMinWidth, min(want, tocMaxWidth, screen/3))
}

// Select moves the cursor to heading i and scrolls it into view.
func (t *TOC) Select(d *doc.Doc, i, height int) {
	t.selectIdx(i, len(d.Headings), max(height-tocHeader, 1))
}

func (t *TOC) Scroll(d *doc.Doc, n, height int) {
	t.scroll(n, len(d.Headings), max(height-tocHeader, 1))
}

// HeadingAt returns the heading shown at row y of the sidebar, or -1.
func (t *TOC) HeadingAt(d *doc.Doc, y int) int {
	i := t.offset + y - tocHeader
	if y < tocHeader || i >= len(d.Headings) {
		return -1
	}
	return i
}

// Render draws the sidebar. current is the section the document is in;
// the cursor is only highlighted while the sidebar has focus.
func (t *TOC) Render(d *doc.Doc, current int, focused bool, width, height int, th theme) []string {
	out := make([]string, 0, height)
	out = append(out, fit(th.tocTitle.Render(" Contents"), width))
	if len(d.Headings) == 0 {
		out = append(out, fit(th.dim.Render(" (no headings)"), width))
	}
	minLevel := minHeadingLevel(d)
	for i := t.offset; i < len(d.Headings) && len(out) < height; i++ {
		h := d.Headings[i]
		text := strings.Repeat("  ", h.Level-minLevel) + h.Text
		text = " " + ansi.Truncate(text, width-2, "…")
		text += strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
		style := th.tocLevel(h.Level - minLevel)
		switch {
		case focused && i == t.cursor:
			style = th.tocCursor
		case i == current:
			style = th.tocCurrent
		}
		out = append(out, style.Render(text))
	}
	for len(out) < height {
		out = append(out, strings.Repeat(" ", width))
	}
	return out
}

func minHeadingLevel(d *doc.Doc) int {
	m := 6
	for _, h := range d.Headings {
		m = min(m, h.Level)
	}
	return m
}
