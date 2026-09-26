package ui

import "github.com/joe-salamy/joe-md/internal/config"

// palette is the UI colours that go with one of glamour's coloured styles.
// Unset colours keep the dark/light defaults.
type palette struct {
	config.Theme
	cursorText string // TOC cursor text, drawn on the accent colour
	matchText  string // text drawn on the match colours
}

// palettes are keyed by glamour style name. dark, light, notty and ascii
// keep the defaults, as do styles loaded from a JSON path.
var palettes = map[string]palette{
	"dracula": {
		Theme: config.Theme{
			Accent:       "#bd93f9",
			Text:         "#f8f8f2",
			Subtle:       "#bfbfbf",
			Dim:          "#6272a4",
			Bar:          "#44475a",
			OnAccent:     "#282a36",
			TOCMode:      "#ffb86c",
			SearchMode:   "#50fa7b",
			Match:        "#f1fa8c",
			MatchCurrent: "#ffb86c",
			TabInactive:  "#44475a",
			Error:        "#ff5555",
		},
		cursorText: "#282a36",
		matchText:  "#282a36",
	},
	"tokyo-night": {
		Theme: config.Theme{
			Accent:       "#7aa2f7",
			Text:         "#c0caf5",
			Subtle:       "#a9b1d6",
			Dim:          "#565f89",
			Bar:          "#24283b",
			OnAccent:     "#1a1b26",
			TOCMode:      "#ff9e64",
			SearchMode:   "#9ece6a",
			Match:        "#e0af68",
			MatchCurrent: "#ff9e64",
			TabInactive:  "#292e42",
			Error:        "#f7768e",
		},
		cursorText: "#1a1b26",
		matchText:  "#1a1b26",
	},
	"pink": {
		Theme: config.Theme{
			Accent:   "212",
			OnAccent: "235",
			TOCMode:  "99",
		},
	},
}

// over returns p with the colours set in t replacing its own.
func (p palette) over(t config.Theme) palette {
	pick := func(set, base string) string {
		if set != "" {
			return set
		}
		return base
	}
	b := &p.Theme
	b.Accent = pick(t.Accent, b.Accent)
	b.Text = pick(t.Text, b.Text)
	b.Subtle = pick(t.Subtle, b.Subtle)
	b.Dim = pick(t.Dim, b.Dim)
	b.Bar = pick(t.Bar, b.Bar)
	b.OnAccent = pick(t.OnAccent, b.OnAccent)
	b.TOCMode = pick(t.TOCMode, b.TOCMode)
	b.SearchMode = pick(t.SearchMode, b.SearchMode)
	b.Match = pick(t.Match, b.Match)
	b.MatchCurrent = pick(t.MatchCurrent, b.MatchCurrent)
	b.TabInactive = pick(t.TabInactive, b.TabInactive)
	b.Error = pick(t.Error, b.Error)
	return p
}
