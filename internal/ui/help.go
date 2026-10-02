package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Help is the key overlay's state. Its text comes from the keymap, so it
// shows the keys as the settings file bound them.
type Help struct {
	list           // over the lines; only offset is used
	pending string // the keys so far of a sequence ("g")
}

// helpRect is where the overlay sits on screen.
func (a *App) helpRect() (x, y, w, h int) {
	return a.centered(min(max(a.width*3/4, 60), 100), max(a.height-4, 12))
}

// helpRows is how many lines of text the overlay shows.
func (a *App) helpRows() int {
	_, _, _, h := a.helpRect()
	return popupRows(h)
}

// helpLines is the overlay's text at its current width. The keymap and
// theme never change, so it is only laid out again when the width does.
func (a *App) helpLines() []string {
	_, _, w, _ := a.helpRect()
	w = max(w-2, 20)
	if a.helpCache == nil || a.helpCacheW != w {
		a.helpCache, a.helpCacheW = a.keymap.helpText(w, a.theme), w
	}
	return a.helpCache
}

// helpText lays out every group of actions: a heading, then one row per
// action with its keys and description.
func (km *Keymap) helpText(width int, th theme) []string {
	keyW := min(28, width*2/5)
	var out []string
	for _, g := range groups {
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, th.menuTitle.Render(" "+g))
		indent := strings.Repeat(" ", keyW+5)
		row := func(keys, desc string) {
			// The description wraps under itself, clear of the keys.
			var d []string
			for _, l := range strings.Split(ansi.Wordwrap(desc, max(width-keyW-5, 10), " "), "\n") {
				d = append(d, th.bar.Render(l))
			}
			if w := ansi.StringWidth(keys); w <= keyW {
				out = append(out, "   "+th.barPrompt.Render(keys)+strings.Repeat(" ", keyW-w+2)+d[0])
				d = d[1:]
			} else {
				for _, l := range strings.Split(ansi.Wordwrap(keys, width-3, " "), "\n") {
					out = append(out, "   "+th.barPrompt.Render(l))
				}
			}
			for _, l := range d {
				out = append(out, indent+l)
			}
		}
		for _, r := range km.rows(g) {
			var alts []string
			for _, ks := range r.keys {
				alts = append(alts, strings.Join(ks, " "))
			}
			row(strings.Join(alts, " / "), r.desc)
		}
		if n := groupNotes[g]; n != "" {
			n = strings.NewReplacer("`", "", "*", "").Replace(n)
			for _, l := range strings.Split(ansi.Wordwrap(n, width-4, " "), "\n") {
				out = append(out, "   "+th.dim.Render(l))
			}
		}
	}
	return out
}

// helpView draws the overlay: a titled box with a footer.
func (a *App) helpView() []string {
	th := a.theme
	_, _, w, h := a.helpRect()
	rows := a.helpRows()
	lines := a.helpLines()
	shown := make([]string, rows)
	copy(shown, lines[min(a.help.offset, len(lines)):])
	pos := strconv.Itoa(min(a.help.offset+rows, len(lines))) + "/" + strconv.Itoa(len(lines))
	left := th.dim.Render(" " + a.hint(ctxHelp, "down") + " " + a.hint(ctxHelp, "up") + " scroll · " + a.hint(ctxHelp, "close") + " close")
	return drawBox(th, " Keys ", shown, left, th.dim.Render(" "+pos+" "), w, h)
}

// helpMouse: the wheel scrolls the overlay and a click outside closes it.
func (a *App) helpMouse(msg tea.MouseMsg) {
	x, y, w, h := a.helpRect()
	step, outside := popupMouse(msg, x, y, w, h)
	a.help.scroll(step, len(a.helpLines()), a.helpRows())
	if outside {
		a.help = nil
	}
}
