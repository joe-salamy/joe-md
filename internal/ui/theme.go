package ui

import (
	"image/color"

	"github.com/joe-salamy/joe-md/internal/config"

	"charm.land/lipgloss/v2"
)

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

// newTheme builds the UI styles: colours set in colors win, then the palette
// that goes with the glamour style, then the dark/light defaults.
func newTheme(dark bool, style string, colors config.Theme) theme {
	p := palettes[style].over(colors)
	colors = p.Theme
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
	cursorText := c(p.cursorText, "255", "255")
	matchText := c(p.matchText, "16", "16")
	s := lipgloss.NewStyle
	return theme{
		dim:              s().Foreground(dim),
		tocTitle:         s().Foreground(dim).Bold(true),
		tocCursor:        s().Foreground(cursorText).Background(accent).Bold(true),
		tocCurrent:       s().Foreground(accent).Bold(true),
		tocLevels:        []lipgloss.Style{s().Foreground(fg).Bold(true), s().Foreground(fg), s().Foreground(mid)},
		separator:        s().Foreground(dim),
		status:           s().Foreground(fg).Background(bar),
		statusDim:        s().Foreground(dim).Background(bar),
		statusMode:       s().Foreground(onAccent).Background(accent).Bold(true),
		statusModeTOC:    s().Foreground(onAccent).Background(tocMode).Bold(true),
		statusModeSearch: s().Foreground(onAccent).Background(searchMode).Bold(true),

		match:        s().Foreground(matchText).Background(match),
		matchCur:     s().Foreground(matchText).Background(matchCur).Bold(true),
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
