package ui

import (
	"image/color"
	"strings"

	"joe-md/internal/config"
	"joe-md/internal/doc"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	tocMinWidth = 18
	tocMaxWidth = 40
	tocHeader   = 1 // rows above the entries
)

// TOC is the table-of-contents sidebar for the focused document.
type TOC struct {
	cursor int // selected heading
	offset int // first visible heading
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

func (t *TOC) Move(d *doc.Doc, n, height int) { t.Select(d, t.cursor+n, height) }

// Select moves the cursor to heading i and scrolls it into view.
func (t *TOC) Select(d *doc.Doc, i, height int) {
	t.cursor = max(0, min(i, len(d.Headings)-1))
	rows := max(height-tocHeader, 1)
	if t.cursor < t.offset {
		t.offset = t.cursor
	} else if t.cursor >= t.offset+rows {
		t.offset = t.cursor - rows + 1
	}
}

func (t *TOC) Scroll(d *doc.Doc, n, height int) {
	rows := max(height-tocHeader, 1)
	t.offset = max(0, min(t.offset+n, len(d.Headings)-rows))
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

type theme struct {
	dim, tocTitle, tocCursor, tocCurrent lipgloss.Style
	tocLevels                            []lipgloss.Style
	separator                            lipgloss.Style
	status, statusMode, statusModeTOC    lipgloss.Style
	statusDim                            lipgloss.Style
	statusModeSearch                     lipgloss.Style

	match, matchCur, matchMark, matchMarkCur lipgloss.Style
	bar, barDim, barPrompt, barChip          lipgloss.Style
	barChipOn                                lipgloss.Style
	resultsTitle, resultsPath, resultsLine   lipgloss.Style

	tabActive, tabInactive, tabFill          lipgloss.Style
	menuBorder, menuTitle, menuDir, menuFile lipgloss.Style
	menuOpen, menuErr                        lipgloss.Style
}

func (th theme) tocLevel(depth int) lipgloss.Style {
	return th.tocLevels[min(depth, len(th.tocLevels)-1)]
}

func newTheme(dark bool, colors config.Theme) theme {
	ld := lipgloss.LightDark(dark)
	c := func(set, light, dark string) color.Color {
		if set != "" {
			return lipgloss.Color(set)
		}
		return ld(lipgloss.Color(light), lipgloss.Color(dark))
	}
	accent := c(colors.Accent, "25", "39")
	fg := c(colors.Text, "235", "252")
	mid := c(colors.Subtle, "240", "248")
	dim := c(colors.Dim, "246", "241")
	bar := c(colors.Bar, "254", "236")
	onAccent := c(colors.OnAccent, "255", "235")
	tocMode := c(colors.TOCMode, "130", "214")
	searchMode := c(colors.SearchMode, "29", "78")
	match := c(colors.Match, "228", "186")
	matchCur := c(colors.MatchCurrent, "208", "214")
	matchMark := c(colors.Match, "178", "186")
	matchMarkCur := c(colors.MatchCurrent, "202", "214")
	tabInactive := c(colors.TabInactive, "252", "238")
	errColor := c(colors.Error, "160", "203")
	black := lipgloss.Color("16")
	s := lipgloss.NewStyle
	return theme{
		dim:              s().Foreground(dim),
		tocTitle:         s().Foreground(dim).Bold(true),
		tocCursor:        s().Foreground(lipgloss.Color("255")).Background(accent).Bold(true),
		tocCurrent:       s().Foreground(accent).Bold(true),
		tocLevels:        []lipgloss.Style{s().Foreground(fg).Bold(true), s().Foreground(fg), s().Foreground(mid)},
		separator:        s().Foreground(dim),
		status:           s().Foreground(fg).Background(bar),
		statusDim:        s().Foreground(dim).Background(bar),
		statusMode:       s().Foreground(onAccent).Background(accent).Bold(true),
		statusModeTOC:    s().Foreground(onAccent).Background(tocMode).Bold(true),
		statusModeSearch: s().Foreground(onAccent).Background(searchMode).Bold(true),

		match:        s().Foreground(black).Background(match),
		matchCur:     s().Foreground(black).Background(matchCur).Bold(true),
		matchMark:    s().Foreground(matchMark),
		matchMarkCur: s().Foreground(matchMarkCur),

		bar:       s().Foreground(fg),
		barDim:    s().Foreground(dim),
		barPrompt: s().Foreground(accent).Bold(true),
		barChip:   s().Foreground(dim),
		barChipOn: s().Foreground(onAccent).Background(accent).Bold(true),

		resultsTitle: s().Foreground(dim).Bold(true),
		resultsPath:  s().Foreground(accent),
		resultsLine:  s().Foreground(dim),

		tabActive:   s().Foreground(onAccent).Background(accent).Bold(true),
		tabInactive: s().Foreground(fg).Background(tabInactive),
		tabFill:     s().Foreground(dim).Background(bar),

		menuBorder: s().Foreground(accent),
		menuTitle:  s().Foreground(accent).Bold(true),
		menuDir:    s().Foreground(accent).Bold(true),
		menuFile:   s().Foreground(fg),
		menuOpen:   s().Foreground(searchMode),
		menuErr:    s().Foreground(errColor),
	}
}
