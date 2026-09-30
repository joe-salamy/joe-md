package ui

import (
	"github.com/atotto/clipboard"

	tea "charm.land/bubbletea/v2"
)

// copyText puts s on the clipboard two ways: OSC 52, which the terminal
// handles (and which works over ssh), and the system's clipboard tool
// (wl-copy, xclip, xsel, pbcopy) for terminals that ignore OSC 52. Either
// may fail silently, so the message only says what was copied.
func (a *App) copyText(s string) tea.Cmd {
	a.msg = "copied " + s
	return tea.Batch(tea.SetClipboard(s), func() tea.Msg {
		_ = clipboard.WriteAll(s)
		return nil
	})
}
