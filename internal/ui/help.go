package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Help is the key overlay's state. Its text comes from the keymap, so it
// shows the keys as the settings file bound them.
type Help struct {
	list           // over the lines; only offset is used
	pending string // the keys so far of a sequence ("g")
	search  textinput.Model
	typing  bool // the search input has the keyboard
	scope   int  // index into helpScopes
}

// helpScopes are what the search matches: all is actions or keys.
var helpScopes = []string{"all", "actions", "keys"}

func (a *App) openHelp() {
	a.help = &Help{search: newInput(a.opts.Dark, "search the keys")}
	a.help.search.SetWidth(a.helpSearchWidth())
}

// helpSearchWidth is the room the search input has in the footer, beside
// the scope and the widest position.
func (a *App) helpSearchWidth() int {
	_, _, w, _ := a.helpRect()
	return max(w-2-3-len(" [actions] "+a.hint(ctxHelpSearch, "scope_next")+": scope")-len(" 9999/9999 "), 1)
}

// helpRefilter shows the top of the text after the query or scope changed.
func (a *App) helpRefilter() {
	a.help.offset = 0
}

func (a *App) helpKey(msg tea.KeyPressMsg) tea.Cmd {
	h, k := a.help, msg.String()
	if h.typing {
		var none string
		if cmd, ok := a.dispatch(&none, k, 0, ctxHelpSearch); ok {
			return cmd
		}
		var cmd tea.Cmd
		before := h.search.Value()
		h.search, cmd = h.search.Update(msg)
		if h.search.Value() != before {
			a.helpRefilter()
		}
		return cmd
	}
	cmd, _ := a.dispatch(&h.pending, k, 0, ctxHelp)
	return cmd
}

func (h *Help) stopSearch() {
	h.typing = false
	h.search.Blur()
}

// setSearch replaces the query.
func (a *App) setHelpSearch(s string) {
	a.help.search.SetValue(s)
	a.helpRefilter()
}

// helpRect is where the overlay sits on screen.
func (a *App) helpRect() (x, y, w, h int) {
	return a.centered(min(max(a.width*3/4, 60), 76), max(a.height-4, 12))
}

// helpRows is how many lines of text the overlay shows.
func (a *App) helpRows() int {
	_, _, _, h := a.helpRect()
	return popupRows(h)
}

// helpLines is the overlay's text at its current width and search. The
// keymap and theme never change, so it is only laid out again when the width
// or the search does.
func (a *App) helpLines() []string {
	_, _, w, _ := a.helpRect()
	w = max(w-2, 20)
	key := helpCacheKey{w: w}
	if a.help != nil {
		key.query, key.scope = a.help.search.Value(), a.help.scope
	}
	if a.helpCache == nil || a.helpCacheKey != key {
		a.helpCache, a.helpCacheKey = a.keymap.helpText(w, a.theme, key.query, key.scope), key
	}
	return a.helpCache
}

// helpCacheKey is what the help's text was laid out for.
type helpCacheKey struct {
	w     int
	query string
	scope int
}

// helpMatch reports whether a row matches the query in the scope: actions
// has every term, ignoring case, in the description or group name; keys has
// the query in the keys as shown.
func helpMatch(group, keys, desc, query string, scope int) bool {
	acts := func() bool {
		text := strings.ToLower(desc + " " + group)
		for _, t := range strings.Fields(strings.ToLower(query)) {
			if !strings.Contains(text, t) {
				return false
			}
		}
		return true
	}
	ks := func() bool { return strings.Contains(keys, strings.TrimSpace(query)) }
	switch helpScopes[scope] {
	case "actions":
		return acts()
	case "keys":
		return ks()
	}
	return acts() || ks()
}

// helpText lays out every group of actions: a heading, then one row per
// action with its keys and description. A query keeps only the matching
// rows, and the groups that have any, without their notes.
func (km *Keymap) helpText(width int, th theme, query string, scope int) []string {
	keyW := min(28, width*2/5)
	filtered := strings.TrimSpace(query) != ""
	var out []string
	for _, g := range groups {
		type line struct{ keys, desc string }
		var lines []line
		for _, r := range km.rows(g) {
			var alts []string
			for _, ks := range r.keys {
				alts = append(alts, strings.Join(ks, " "))
			}
			l := line{strings.Join(alts, " / "), r.desc}
			if !filtered || helpMatch(g, l.keys, l.desc, query, scope) {
				lines = append(lines, l)
			}
		}
		if filtered && len(lines) == 0 {
			continue
		}
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
		for _, l := range lines {
			row(l.keys, l.desc)
		}
		if n := groupNotes[g]; n != "" && !filtered {
			n = strings.NewReplacer("`", "", "*", "").Replace(n)
			for _, l := range strings.Split(ansi.Wordwrap(n, width-4, " "), "\n") {
				out = append(out, "   "+th.dim.Render(l))
			}
		}
	}
	if len(out) == 0 {
		out = append(out, th.dim.Render(" no matching keys"))
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
	var left string
	if hs := a.help; hs.typing || hs.search.Value() != "" {
		left = th.barPrompt.Render(" / ") + hs.search.View() + th.dim.Render(" ["+helpScopes[hs.scope]+"] "+a.hint(ctxHelpSearch, "scope_next")+": scope")
	} else {
		left = th.dim.Render(" " + a.hint(ctxHelp, "down") + " " + a.hint(ctxHelp, "up") + " scroll · " + a.hint(ctxHelp, "search") + " search · " + a.hint(ctxHelp, "close") + " close")
	}
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
