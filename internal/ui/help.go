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
	offset  int
	pending string // the keys so far of a sequence ("g")
}

func (h *Help) scroll(d, rows, total int) {
	h.offset = max(0, min(h.offset+d, total-rows))
}

// helpRect is where the overlay sits on screen.
func (a *App) helpRect() (x, y, w, h int) {
	w = min(max(a.width*3/4, 60), 100, a.width)
	h = min(max(a.height-4, 12), a.height)
	return (a.width - w) / 2, (a.height - h) / 2, w, h
}

// helpRows is how many lines of text the overlay shows.
func (a *App) helpRows() int {
	_, _, _, h := a.helpRect()
	return max(h-3, 1)
}

// helpLines is the overlay's text at its current width.
func (a *App) helpLines() []string {
	_, _, w, _ := a.helpRect()
	return a.keymap.helpText(max(w-2, 20), a.theme)
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
		row := func(keys, desc string) {
			d := th.bar.Render(desc)
			if w := ansi.StringWidth(keys); w <= keyW {
				out = append(out, "   "+th.barPrompt.Render(keys)+strings.Repeat(" ", keyW-w+2)+d)
				return
			}
			for _, l := range strings.Split(ansi.Wordwrap(keys, width-3, " "), "\n") {
				out = append(out, "   "+th.barPrompt.Render(l))
			}
			out = append(out, "   "+strings.Repeat(" ", keyW+2)+d)
		}
		for _, x := range actions {
			if x.group != g {
				continue
			}
			ks := km.display(x)
			if len(ks) == 0 {
				ks = []string{"(unbound)"}
			}
			row(strings.Join(ks, " "), x.desc)
		}
		for _, f := range fixedKeys[g] {
			row(f.keys, f.desc)
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
	inner := max(w-2, 1)
	rows := a.helpRows()
	lines := a.helpLines()
	a.help.scroll(0, rows, len(lines)) // clamp after a resize
	b := th.menuBorder.Render

	title := " Keys "
	out := make([]string, 0, h)
	out = append(out, b("╭─")+th.menuTitle.Render(title)+b(strings.Repeat("─", max(inner-1-len(title), 0))+"╮"))
	for r := range rows {
		line := ""
		if i := a.help.offset + r; i < len(lines) {
			line = lines[i]
		}
		out = append(out, b("│")+fit(line, inner)+b("│"))
	}
	pos := strconv.Itoa(min(a.help.offset+rows, len(lines))) + "/" + strconv.Itoa(len(lines))
	right := th.dim.Render(" " + pos + " ")
	left := th.dim.Render(" j k scroll · esc close")
	gap := inner - ansi.StringWidth(left) - ansi.StringWidth(right)
	out = append(out, b("│")+fit(left+strings.Repeat(" ", max(gap, 0))+right, inner)+b("│"))
	out = append(out, b("╰"+strings.Repeat("─", inner)+"╯"))
	return out[:min(len(out), h)]
}

// helpMouse: the wheel scrolls the overlay and a click outside closes it.
func (a *App) helpMouse(msg tea.MouseMsg) {
	m := msg.Mouse()
	x, y, w, h := a.helpRect()
	switch msg.(type) {
	case tea.MouseWheelMsg:
		switch m.Button {
		case tea.MouseWheelUp:
			a.help.scroll(-wheelStep, a.helpRows(), len(a.helpLines()))
		case tea.MouseWheelDown:
			a.help.scroll(wheelStep, a.helpRows(), len(a.helpLines()))
		}
	case tea.MouseClickMsg:
		if m.X < x || m.X >= x+w || m.Y < y || m.Y >= y+h {
			a.help = nil
		}
	}
}
