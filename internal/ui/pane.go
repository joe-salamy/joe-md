package ui

import (
	"bytes"
	"sort"
	"strconv"
	"strings"

	"github.com/joe-salamy/joe-md/internal/doc"

	"github.com/charmbracelet/x/ansi"
)

// Pane is a scrollable view of one rendered document.
type Pane struct {
	doc    *doc.Doc
	view   *doc.Rendered
	offset int // top rendered line
	width  int
	height int
	wrap   int // word-wrap width p.view was rendered at
	match  *matches
	toc    TOC  // the sidebar's state while this pane has focus
	bind   bool // scrollbind: scrolls with the tab's other bound panes

	// pin is the heading GotoHeading last jumped to. A heading near the end
	// can't be scrolled to the top, so it counts as the current heading for as
	// long as the pane stays where the jump left it.
	pin struct {
		heading, offset int
		ok              bool
	}
}

func NewPane(d *doc.Doc) *Pane { return &Pane{doc: d} }

// clone is a new pane on the same document at the same place, for a split.
func (p *Pane) clone() *Pane {
	c := *p
	c.bind = false
	if p.match != nil {
		m := *p.match
		c.match = &m // indexMatches replaces the derived maps, never edits them
	}
	return &c
}

// Layout re-renders the document for a new size, keeping the same source line
// at the top of the pane. If the wrap width is unchanged the rendering is too,
// so the view stays exactly where it was.
func (p *Pane) Layout(r *doc.Renderer, width, height, maxWrap int) error {
	p.width, p.height = width, height
	wrap := min(width, maxWrap)
	if p.view != nil && wrap == p.wrap {
		return nil
	}
	return p.render(r, wrap, p.TopSource())
}

// render renders the document at wrap and puts source line anchor on top.
func (p *Pane) render(r *doc.Renderer, wrap, anchor int) error {
	v, err := r.Render(p.doc, wrap)
	if err != nil {
		return err
	}
	p.view, p.wrap = v, wrap
	p.pin.ok = false
	p.indexMatches()
	p.GotoSource(anchor)
	return nil
}

// Reload re-reads the file from disk, keeping the same source line on top.
// If the file is unchanged the view is left exactly as it was.
func (p *Pane) Reload(r *doc.Renderer, maxWrap int) (changed bool, err error) {
	d, err := doc.Load(p.doc.Path)
	if err != nil {
		return false, err
	}
	if bytes.Equal(d.Source, p.doc.Source) {
		return false, nil
	}
	anchor := p.TopSource()
	p.doc, p.view = d, nil // never a view of the old text over the new
	return true, p.render(r, min(p.width, maxWrap), anchor)
}

// TopSource is the source line shown at the top of the pane.
func (p *Pane) TopSource() int {
	if p.view == nil {
		return 0
	}
	return p.view.RenderedToSource(p.doc, p.offset)
}

func (p *Pane) GotoSource(src int) {
	if p.view == nil {
		return
	}
	if src <= 0 {
		p.ScrollTo(0) // include the leading blank line
		return
	}
	p.ScrollTo(p.view.SourceToRendered(p.doc, src))
}

func (p *Pane) GotoHeading(h int) {
	if p.view == nil || h < 0 || h >= len(p.doc.Headings) {
		return
	}
	p.jumpTo(p.view.HeadingLine(p.doc, h))
	p.pin.heading, p.pin.offset, p.pin.ok = h, p.offset, true
}

// ScrollTo puts line at the top of the pane. It may scroll past the end, so
// that a source line (say, micro's cursor line) is always at the top.
func (p *Pane) ScrollTo(line int) {
	last := 0
	if p.view != nil {
		last = len(p.view.Lines) - 1
	}
	p.offset = max(0, min(line, last))
}

// ScrollBy scrolls relative lines, stopping once the last line is visible (but
// not snapping back if a jump already scrolled past that).
func (p *Pane) ScrollBy(n int) {
	target := p.offset + n
	if n > 0 {
		target = min(target, max(p.maxOffset(), p.offset))
	}
	p.ScrollTo(target)
}

func (p *Pane) ScrollToBottom() { p.ScrollTo(p.maxOffset()) }

// jumpTo puts line at the top of the pane, but never scrolls past the point
// where the last line is at the bottom.
func (p *Pane) jumpTo(line int) { p.ScrollTo(min(line, p.maxOffset())) }

// reveal scrolls line to the top of the pane (as jumpTo) unless it is
// already in view.
func (p *Pane) reveal(line int) {
	if !p.visible(line) {
		p.jumpTo(line)
	}
}

// ScrollToSource puts source line src at the top. Moving down, it stops once
// the last line is visible, like ScrollBy.
func (p *Pane) ScrollToSource(src int) {
	if p.view == nil {
		return
	}
	target := 0
	if src > 0 {
		target = p.view.SourceToRendered(p.doc, min(src, p.doc.Lines-1))
	}
	if target > p.offset {
		target = min(target, max(p.maxOffset(), p.offset))
	}
	p.ScrollTo(target)
}

// maxOffset is the offset that shows the last line at the bottom.
func (p *Pane) maxOffset() int {
	if p.view == nil {
		return 0
	}
	return max(0, len(p.view.Lines)-p.height)
}

// top is the first line of content at the top of the pane: a blank separator
// line sitting directly above a block counts as that block.
func (p *Pane) top() int {
	if p.offset < len(p.view.Lines) && p.view.Lines[p.offset] == "" {
		return p.offset + 1
	}
	return p.offset
}

// CurrentHeading is the last heading at or above the top of the pane, or -1.
// After a jump to a heading that couldn't reach the top, it is that heading.
func (p *Pane) CurrentHeading() int {
	if p.view == nil {
		return -1
	}
	if p.pin.ok && p.pin.offset == p.offset && p.pin.heading < len(p.doc.Headings) {
		return p.pin.heading
	}
	top := p.top()
	return sort.Search(len(p.doc.Headings), func(i int) bool {
		return p.view.HeadingLine(p.doc, i) > top
	}) - 1
}

// NextHeading returns the n-th heading below (n > 0) or above (n < 0) the top
// of the pane, clamped to the first/last heading, or -1 if there are none.
func (p *Pane) NextHeading(n int) int {
	hs := len(p.doc.Headings)
	if p.view == nil || hs == 0 || n == 0 {
		return -1
	}
	cur := p.CurrentHeading()
	if n < 0 && cur < 0 {
		return -1 // already above the first heading
	}
	if n < 0 && p.view.HeadingLine(p.doc, cur) < p.top() {
		n++ // mid-section: the first step back goes to the section's own heading
	}
	return max(0, min(cur+n, hs-1))
}

// Percent is how far through the document the pane is, in vim's style.
// After a jump past the end (to a source line) it is the share of the
// document above the top line.
func (p *Pane) Percent() string {
	switch m := p.maxOffset(); {
	case p.offset > m:
		return strconv.Itoa(p.offset*100/len(p.view.Lines)) + "%"
	case m == 0:
		return "All"
	case p.offset == 0:
		return "Top"
	case p.offset >= m:
		return "Bot"
	default:
		return strconv.Itoa(p.offset*100/m) + "%"
	}
}

// Render returns exactly p.height lines, each exactly p.width cells wide.
// When the pane is wider than the wrap width the document is centred; a line
// wider than the wrap (a long code line, a wide table) gives up some of its
// margin rather than being cut off.
func (p *Pane) Render(th theme) []string {
	out := make([]string, p.height)
	margin := max(p.width-p.wrap, 0) / 2
	for y := range out {
		l := ""
		if r := p.offset + y; p.view != nil && r < len(p.view.Lines) {
			l = p.decorate(r, p.view.Lines[r], th)
			if pad := min(margin, p.width-ansi.StringWidth(l)); pad > 0 {
				l = strings.Repeat(" ", pad) + l
			}
		}
		out[y] = fit(l, p.width)
	}
	return out
}
