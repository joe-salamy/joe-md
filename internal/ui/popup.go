package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Pop-ups (the file menu and the help) are bordered boxes drawn over the
// screen, and they, the sidebar and the results panel all show a scrolling
// list with a cursor.

// list is the cursor and scroll position of a list of n items shown rows at
// a time.
type list struct {
	cursor int // selected item
	offset int // first item in view
}

// selectIdx moves the cursor to item i, clamped, and scrolls it into view.
func (l *list) selectIdx(i, n, rows int) {
	l.cursor = max(0, min(i, n-1))
	if l.cursor < l.offset {
		l.offset = l.cursor
	} else if l.cursor >= l.offset+rows {
		l.offset = l.cursor - rows + 1
	}
}

// scroll moves the view d items without moving the cursor.
func (l *list) scroll(d, n, rows int) {
	l.offset = max(0, min(l.offset+d, n-rows))
}

// centered is where a box of up to w by h cells sits in the middle of the
// screen.
func (a *App) centered(w, h int) (x, y, width, height int) {
	w, h = min(w, a.width), min(h, a.height)
	return (a.width - w) / 2, (a.height - h) / 2, w, h
}

// popupRows is how many lines a box h rows high shows: all but the borders
// and the footer.
func popupRows(h int) int { return max(h-3, 1) }

// drawBox draws a pop-up w by h cells: title in the top border, then lines
// (popupRows(h) of them), then a footer with left and right parts.
func drawBox(th theme, title string, lines []string, left, right string, w, h int) []string {
	inner := max(w-2, 1)
	b := th.menuBorder.Render
	out := make([]string, 0, h)
	out = append(out, b("╭─")+th.menuTitle.Render(title)+
		b(strings.Repeat("─", max(inner-1-ansi.StringWidth(title), 0))+"╮"))
	for _, l := range lines {
		out = append(out, b("│")+fit(l, inner)+b("│"))
	}
	left = ansi.Truncate(left, max(inner-ansi.StringWidth(right), 0), "")
	gap := inner - ansi.StringWidth(left) - ansi.StringWidth(right)
	out = append(out, b("│")+fit(left+"\x1b[0m"+strings.Repeat(" ", max(gap, 0))+right, inner)+b("│"))
	out = append(out, b("╰"+strings.Repeat("─", inner)+"╯"))
	return out[:min(len(out), h)]
}

// popupMouse sorts out a mouse event for a pop-up at x, y, w by h: the
// wheel's scroll step (0 if it isn't the wheel), and whether it is a left
// click outside the box, which closes it.
func popupMouse(msg tea.MouseMsg, x, y, w, h int) (step int, outside bool) {
	m := msg.Mouse()
	switch msg.(type) {
	case tea.MouseWheelMsg:
		switch m.Button {
		case tea.MouseWheelUp:
			return -wheelStep, false
		case tea.MouseWheelDown:
			return wheelStep, false
		}
	case tea.MouseClickMsg:
		inside := m.X >= x && m.X < x+w && m.Y >= y && m.Y < y+h
		return 0, m.Button == tea.MouseLeft && !inside
	}
	return 0, false
}

// overlay draws box over the screen lines at column x, row y. The screen
// line's own style resumes after the box because TruncateLeft keeps the
// escape codes before the cut.
func overlay(screen, box []string, x, y, width int) {
	for i, l := range box {
		r := y + i
		if r < 0 || r >= len(screen) {
			continue
		}
		s := screen[r]
		left := ansi.Truncate(s, x, "")
		left += "\x1b[0m" + strings.Repeat(" ", max(x-ansi.StringWidth(left), 0))
		right := ansi.TruncateLeft(s, x+ansi.StringWidth(l), "")
		screen[r] = fit(left+l+"\x1b[0m"+right, width)
	}
}
