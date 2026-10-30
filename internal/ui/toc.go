package ui

import (
	"slices"
	"strings"

	"github.com/joe-salamy/joe-md/internal/doc"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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

// HeadingAt returns the heading shown at row y of the sidebar, or -1. While
// filtering, the rows are the filter's headings.
func (t *TOC) HeadingAt(d *doc.Doc, f *tocFilter, y int) int {
	if y < tocHeader {
		return -1
	}
	if f != nil {
		if i := f.view.offset + y - tocHeader; i < len(f.shown) {
			return f.shown[i]
		}
		return -1
	}
	if i := t.offset + y - tocHeader; i < len(d.Headings) {
		return i
	}
	return -1
}

// Render draws the sidebar. current is the section the document is in;
// the cursor is only highlighted while the sidebar has focus. With a filter
// f, the filter line replaces the title and only its headings are listed.
func (t *TOC) Render(d *doc.Doc, f *tocFilter, current int, focused bool, width, height int, th theme) []string {
	out := make([]string, 0, height)
	n, first := len(d.Headings), t.offset
	if f != nil {
		out = append(out, fit(th.barPrompt.Render(" / ")+f.input.View(), width))
		n, first = len(f.shown), f.view.offset
		if n == 0 && len(d.Headings) > 0 {
			out = append(out, fit(th.dim.Render(" nothing matches /"+f.input.Value()), width))
		}
	} else {
		out = append(out, fit(th.tocTitle.Render(" Contents"), width))
	}
	if len(d.Headings) == 0 {
		out = append(out, fit(th.dim.Render(" (no headings)"), width))
	}
	minLevel := minHeadingLevel(d)
	for r := first; r < n && len(out) < height; r++ {
		i := r
		if f != nil {
			i = f.shown[r]
		}
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

// The sidebar's filter narrows the headings as you type, like the file
// menu's name filter and with its rule: every word, ignoring case. It only
// exists while it has the keyboard. The TOC cursor stays a heading index and
// the document follows it, so leaving with enter is already at the heading.

// tocFilter is the sidebar filter's state.
type tocFilter struct {
	input textinput.Model
	on    bool  // filtering: the input has the keyboard
	toc   *TOC  // the sidebar being filtered
	hide  bool  // search_toc showed the hidden sidebar: accept hides it again
	shown []int // the matching headings, in order
	view  list  // scroll over shown; view.cursor is the cursor's position in it
}

// tocMatches appends to into the headings of d whose text contains every
// word of query, ignoring case.
func tocMatches(d *doc.Doc, query string, into []int) []int {
	terms := filterTerms(query)
	for i, h := range d.Headings {
		if hasTerms(h.Text, terms) {
			into = append(into, i)
		}
	}
	return into
}

// activeTOCFilter is the filter while it is on, for drawing and the mouse.
func (a *App) activeTOCFilter() *tocFilter {
	if a.tocFilter.on {
		return &a.tocFilter
	}
	return nil
}

// tocRows is how many headings the sidebar shows.
func (a *App) tocRows() int { return max(a.bodyHeight()-tocHeader, 1) }

// startTOCFilter gives the focused sidebar's filter the keyboard. hide says
// the sidebar was shown just for it.
func (a *App) startTOCFilter(hide bool) tea.Cmd {
	f := &a.tocFilter
	f.on, f.toc, f.hide = true, a.toc, hide
	f.input.SetValue("")
	f.view = list{}
	a.tocRefilter(false)
	return f.input.Focus()
}

// searchTOC is normal.search_toc: show the sidebar if it is hidden, focus
// it and start the filter.
func (a *App) searchTOC() tea.Cmd {
	hide := !a.showTOC
	if hide {
		a.showTOC = true
		a.layout()
	}
	a.focus = focusTOC
	return a.startTOCFilter(hide)
}

// stopTOCFilter drops the filter, scrolling the full list to the cursor.
func (a *App) stopTOCFilter() {
	f := &a.tocFilter
	f.on, f.toc, f.hide = false, nil, false
	f.input.Blur()
	f.input.SetValue("")
	f.shown = f.shown[:0]
	if a.pane != nil {
		a.toc.Select(a.pane.doc, a.toc.cursor, a.bodyHeight())
	}
}

// acceptTOCFilter is enter: back to the document, which is already at the
// heading, hiding the sidebar again if search_toc showed it.
func (a *App) acceptTOCFilter() {
	hide := a.tocFilter.hide
	a.stopTOCFilter()
	a.focus = focusDoc
	if hide {
		a.showTOC = false
		a.layout()
	}
}

// setTOCFilter replaces the filter text.
func (a *App) setTOCFilter(s string) {
	a.tocFilter.input.SetValue(s)
	a.tocRefilter(true)
}

// tocRefilter matches the headings again. The cursor stays on its heading
// if it is still shown, else goes to the first shown one, and with follow
// the document goes along; with nothing shown, both stay put.
func (a *App) tocRefilter(follow bool) {
	f := &a.tocFilter
	f.shown = tocMatches(a.pane.doc, f.input.Value(), f.shown[:0])
	f.view = list{}
	if len(f.shown) == 0 {
		return
	}
	i := a.toc.cursor
	if !slices.Contains(f.shown, i) {
		i = f.shown[0]
	}
	if follow {
		a.tocFilterSelect(i)
	} else {
		f.view.selectIdx(slices.Index(f.shown, i), len(f.shown), a.tocRows())
	}
}

// tocFilterSelect moves the cursor to shown heading i, scrolling the
// filtered list to it; the document follows.
func (a *App) tocFilterSelect(i int) {
	f := &a.tocFilter
	f.view.selectIdx(slices.Index(f.shown, i), len(f.shown), a.tocRows())
	a.tocSelect(i)
}

// tocFilterStep moves the cursor d shown headings on.
func (a *App) tocFilterStep(d int) {
	f := &a.tocFilter
	if len(f.shown) == 0 {
		return
	}
	p := max(0, min(slices.Index(f.shown, a.toc.cursor)+d, len(f.shown)-1))
	a.tocFilterSelect(f.shown[p])
}

// tocFilterKey handles a key while filtering: an action, or an edit.
func (a *App) tocFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	var none string
	if cmd, ok := a.dispatch(&none, msg.String(), 0, ctxTOCFilter); ok {
		return cmd
	}
	f := &a.tocFilter
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != before {
		a.tocRefilter(true)
	}
	return cmd
}

// keepTOCFilter ends the filter once the sidebar has lost the keyboard (a
// click on a pane or the search bar, another tab) and otherwise matches
// again, so the shown headings always index the document on screen.
func (a *App) keepTOCFilter() {
	f := &a.tocFilter
	if !f.on {
		return
	}
	if a.pane == nil || a.toc != f.toc || a.focus != focusTOC || !a.showTOC || a.typing {
		a.stopTOCFilter()
		return
	}
	f.shown = tocMatches(a.pane.doc, f.input.Value(), f.shown[:0])
	f.view.scroll(0, len(f.shown), a.tocRows())
}
