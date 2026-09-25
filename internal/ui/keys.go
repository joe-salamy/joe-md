package ui

import (
	"fmt"
	"strings"

	"joe-md/internal/config"
	"joe-md/internal/search"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Every key is bound through the action table below. Each action has a name
// within a context, default keys, and a description; the settings file can
// replace an action's keys by name. The same table drives dispatch, the help
// overlay, -dump-config and KEYS.md.
//
// Contexts:
//
//	normal   the document, and keys that work everywhere outside a pop-up
//	toc      the sidebar, when focused (falls back to normal)
//	results  the results list, when focused (falls back to normal)
//	window   the key after normal.window (ctrl+w)
//	search   the search bar while typing (other keys edit the text)
//	menu     the file menu
//	filter   the file menu's name filter while typing
//	help     the help overlay
//
// A key is spelled as tea's KeyPressMsg.String() spells it. A sequence is
// keys joined by spaces ("g g"); its first keys start the sequence.

// call is what an action gets: the count typed before it (0 if none), n (the
// count, at least 1) and the key that triggered it.
type call struct {
	count, n int
	key      string
}

type action struct {
	ctx, name string
	group     string // help and KEYS.md section
	desc      string
	keys      []string // defaults
	run       func(a *App, c call) tea.Cmd
}

func keys(k ...string) []string { return k }

// actions is the table; it's filled in init because the run functions refer
// back to App methods that use the keymap.
var actions []*action

// groups orders the help sections.
var groups = []string{
	"Scrolling", "File", "Search", "Search bar", "Results list", "Tabs", "Panes",
	"Focus and layout", "Table of contents", "Quitting", "Help", "File menu", "Menu filter", "Help overlay",
}

// groupNotes follow a group's table in the help and KEYS.md.
var groupNotes = map[string]string{
	"Scrolling":         "`{n}gg` and `{n}G` go to source line *n*.",
	"Search bar":        "Other keys edit the text, readline style: `ctrl+w`, `ctrl+u` and `ctrl+k` delete a word, to the start and to the end; `ctrl+a` and `ctrl+e` go to the start and end; `alt+b` and `alt+f` move by words.",
	"Results list":      "`{n}gg` and `{n}G` go to result *n*.",
	"Tabs":              "`{n}gt` goes to tab *n*. `tab_n` goes to the tab numbered by the last digit of its key.",
	"Panes":             "These follow the window prefix. A count before the prefix or after it resizes by that much: `5 ctrl+w >` or `ctrl+w 5 >`.",
	"Table of contents": "`{n}gg` and `{n}G` go to heading *n*.",
	"Menu filter":       "Other keys edit the filter, which matches as you type.",
}

func init() {
	half := func(a *App) int { return max(a.bodyHeight()/2, 1) }
	page := func(a *App) int { return max(a.bodyHeight()-2, 1) }
	do := func(f func(a *App, c call)) func(a *App, c call) tea.Cmd {
		return func(a *App, c call) tea.Cmd { f(a, c); return nil }
	}
	docOnly := func(f func(a *App, c call)) func(a *App, c call) tea.Cmd {
		return func(a *App, c call) tea.Cmd {
			if a.pane != nil {
				f(a, c)
				a.syncTOC()
			}
			return nil
		}
	}
	add := func(ctx, group string, list ...*action) {
		for _, x := range list {
			x.ctx, x.group = ctx, group
			actions = append(actions, x)
		}
	}

	add("normal", "Scrolling",
		&action{name: "scroll_down", desc: "one line down", keys: keys("j", "down", "ctrl+e", "ctrl+n", "enter"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(c.n) })},
		&action{name: "scroll_up", desc: "one line up", keys: keys("k", "up", "ctrl+y", "ctrl+p"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(-c.n) })},
		&action{name: "half_page_down", desc: "half a page down", keys: keys("ctrl+d"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(c.n * half(a)) })},
		&action{name: "half_page_up", desc: "half a page up", keys: keys("ctrl+u"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(-c.n * half(a)) })},
		&action{name: "page_down", desc: "a page down", keys: keys("ctrl+f", "pgdown", "space", "f"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(c.n * page(a)) })},
		&action{name: "page_up", desc: "a page up", keys: keys("ctrl+b", "pgup", "b"),
			run: docOnly(func(a *App, c call) { a.pane.ScrollBy(-c.n * page(a)) })},
		&action{name: "top", desc: "top, or source line n", keys: keys("g g", "home"),
			run: docOnly(func(a *App, c call) {
				if c.count > 0 {
					a.gotoSourceLine(c.count)
				} else {
					a.pane.ScrollTo(0)
				}
			})},
		&action{name: "bottom", desc: "bottom, or source line n", keys: keys("G", "end"),
			run: docOnly(func(a *App, c call) {
				if c.count > 0 {
					a.gotoSourceLine(c.count)
				} else {
					a.pane.ScrollToBottom()
				}
			})},
		&action{name: "next_heading", desc: "next heading", keys: keys("] ]", "}"),
			run: docOnly(func(a *App, c call) { a.pane.GotoHeading(a.pane.NextHeading(c.n)) })},
		&action{name: "prev_heading", desc: "previous heading", keys: keys("[ [", "{"),
			run: docOnly(func(a *App, c call) { a.pane.GotoHeading(a.pane.NextHeading(-c.n)) })},
	)
	add("normal", "File",
		&action{name: "edit", desc: "edit in micro at the top line", keys: keys("e"),
			run: func(a *App, c call) tea.Cmd { return a.edit() }},
		&action{name: "reload", desc: "reload from disk", keys: keys("r"),
			run: func(a *App, c call) tea.Cmd {
				changed, cmd := a.reload()
				if changed {
					a.msg = "reloaded"
				} else if a.msg == "" {
					a.msg = "unchanged"
				}
				return cmd
			}},
		&action{name: "show_path", desc: "show the file's path", keys: keys("ctrl+g"),
			run: docOnly(func(a *App, c call) { a.msg = a.pane.doc.Path })},
		&action{name: "open_menu", desc: "open the file menu", keys: keys("o"),
			run: func(a *App, c call) tea.Cmd { return a.menuHere() }},
	)
	add("normal", "Search",
		&action{name: "search_file", desc: "search this file", keys: keys("/"),
			run: func(a *App, c call) tea.Cmd { return a.startSearch(search.File) }},
		&action{name: "search_files", desc: "search the directory or repo", keys: keys("?"),
			run: func(a *App, c call) tea.Cmd { return a.startSearch(a.crossScope) }},
		&action{name: "next_match", desc: "next match", keys: keys("n"),
			run: docOnly(func(a *App, c call) { a.nextMatch(c.n) })},
		&action{name: "prev_match", desc: "previous match", keys: keys("N"),
			run: docOnly(func(a *App, c call) { a.nextMatch(-c.n) })},
		&action{name: "toggle_results", desc: "open / close the results list", keys: keys("g r"),
			run: do(func(a *App, c call) { a.toggleResults() })},
		&action{name: "next_result", desc: "next result, in the focused pane", keys: keys("] q"),
			run: docOnly(func(a *App, c call) { a.stepResult(c.n) })},
		&action{name: "prev_result", desc: "previous result, in the focused pane", keys: keys("[ q"),
			run: docOnly(func(a *App, c call) { a.stepResult(-c.n) })},
		&action{name: "toggle_search_bar", desc: "show / hide the search bar", keys: keys("ctrl+s"),
			run: do(func(a *App, c call) { a.showBar = !a.showBar; a.layout() })},
	)
	add("search", "Search bar",
		&action{name: "submit", desc: "search (empty repeats the last search)", keys: keys("enter"),
			run: func(a *App, c call) tea.Cmd { return a.submit() }},
		&action{name: "cancel", desc: "cancel", keys: keys("esc"),
			run: do(func(a *App, c call) { a.stopTyping() })},
		&action{name: "clear", desc: "clear the input (again to cancel)", keys: keys("ctrl+c"),
			run: do(func(a *App, c call) {
				if a.input.Value() != "" {
					a.input.SetValue("")
				} else {
					a.stopTyping()
				}
			})},
		&action{name: "scope_next", desc: "next scope: file → dir → repo", keys: keys("tab"),
			run: do(func(a *App, c call) { a.scope = a.scope.Next(1) })},
		&action{name: "scope_prev", desc: "previous scope", keys: keys("shift+tab"),
			run: do(func(a *App, c call) { a.scope = a.scope.Next(-1) })},
		&action{name: "toggle_literal", desc: "regex / literal text", keys: keys("ctrl+r"),
			run: do(func(a *App, c call) { a.mode.Literal = !a.mode.Literal; a.setPlaceholder() })},
		&action{name: "history_prev", desc: "previous search in history", keys: keys("up", "ctrl+p"),
			run: do(func(a *App, c call) { a.historyStep(-1) })},
		&action{name: "history_next", desc: "next search in history", keys: keys("down", "ctrl+n"),
			run: do(func(a *App, c call) { a.historyStep(1) })},
	)

	rsel := func(f func(r *Results, c call, rows int) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { r, rows := a.results, a.resultRows(); r.Select(f(r, c, rows), rows) })
	}
	add("results", "Results list",
		&action{name: "down", desc: "next result", keys: keys("j", "down", "ctrl+n"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor + c.n })},
		&action{name: "up", desc: "previous result", keys: keys("k", "up", "ctrl+p"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor - c.n })},
		&action{name: "half_down", desc: "half a page down", keys: keys("ctrl+d"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor + c.n*max(rows/2, 1) })},
		&action{name: "half_up", desc: "half a page up", keys: keys("ctrl+u"),
			run: rsel(func(r *Results, c call, rows int) int { return r.cursor - c.n*max(rows/2, 1) })},
		&action{name: "first", desc: "first result, or result n", keys: keys("g g", "home"),
			run: rsel(func(r *Results, c call, rows int) int { return c.n - 1 })},
		&action{name: "last", desc: "last result, or result n", keys: keys("G", "end"),
			run: rsel(func(r *Results, c call, rows int) int {
				if c.count > 0 {
					return c.count - 1
				}
				return len(r.res.Matches) - 1
			})},
		&action{name: "open", desc: "open in a new tab", keys: keys("enter", "l"),
			run: do(func(a *App, c call) { a.openResult(openNewTab) })},
		&action{name: "open_here", desc: "open in the focused pane", keys: keys("O"),
			run: do(func(a *App, c call) { a.openResult(openReplace) })},
		&action{name: "open_vsplit", desc: "open in a new pane beside", keys: keys("v"),
			run: do(func(a *App, c call) { a.openResult(openVSplit) })},
		&action{name: "open_hsplit", desc: "open in a new pane below", keys: keys("s"),
			run: do(func(a *App, c call) { a.openResult(openHSplit) })},
		&action{name: "close", desc: "close the list", keys: keys("q", "esc"),
			run: do(func(a *App, c call) { a.closeResults() })},
	)

	add("normal", "Tabs",
		&action{name: "next_tab", desc: "next tab, or tab n", keys: keys("g t"),
			run: do(func(a *App, c call) {
				if c.count > 0 {
					a.gotoTab(c.count)
				} else {
					a.stepTab(1)
				}
			})},
		&action{name: "prev_tab", desc: "previous tab", keys: keys("g T"),
			run: do(func(a *App, c call) { a.stepTab(-c.n) })},
		&action{name: "tab_n", desc: "tab 1 … 9", keys: keys("alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8", "alt+9"),
			run: do(func(a *App, c call) {
				if d := c.key[len(c.key)-1]; d >= '1' && d <= '9' {
					a.gotoTab(int(d - '0'))
				}
			})},
		&action{name: "close_tab", desc: "close the tab (not the last one)", keys: keys("x"),
			run: do(func(a *App, c call) {
				if len(a.tabs) == 1 {
					a.msg = "last tab (q quits)"
				} else {
					a.closeTab()
				}
			})},
		&action{name: "toggle_tab_bar", desc: "show / hide the tab bar", keys: keys("alt+t"),
			run: do(func(a *App, c call) { a.showTabs = !a.showTabs; a.layout() })},
	)

	add("normal", "Panes",
		&action{name: "window", desc: "prefix for the pane keys below", keys: keys("ctrl+w"),
			run: do(func(a *App, c call) { a.window = true; a.count = c.count })},
	)
	win := func(f func(a *App, c call)) func(a *App, c call) tea.Cmd {
		return func(a *App, c call) tea.Cmd {
			if a.pane != nil {
				f(a, c)
				a.syncTOC()
			}
			return nil
		}
	}
	resize := func(vert bool, sign int) func(a *App, c call) tea.Cmd {
		return win(func(a *App, c call) {
			if a.tab().resize(vert, sign*c.n) {
				a.layout()
			}
		})
	}
	add("window", "Panes",
		&action{name: "split_vertical", desc: "split side by side", keys: keys("v", "ctrl+v"),
			run: win(func(a *App, c call) { a.splitPane(true) })},
		&action{name: "split_horizontal", desc: "split stacked", keys: keys("s", "S", "ctrl+s"),
			run: win(func(a *App, c call) { a.splitPane(false) })},
		&action{name: "focus_left", desc: "focus the pane left (past the edge: the sidebar)", keys: keys("h", "ctrl+h", "left"),
			run: win(func(a *App, c call) { a.moveFocus("h") })},
		&action{name: "focus_down", desc: "focus the pane below (past the edge: the results)", keys: keys("j", "ctrl+j", "down"),
			run: win(func(a *App, c call) { a.moveFocus("j") })},
		&action{name: "focus_up", desc: "focus the pane above", keys: keys("k", "ctrl+k", "up"),
			run: win(func(a *App, c call) { a.moveFocus("k") })},
		&action{name: "focus_right", desc: "focus the pane right", keys: keys("l", "ctrl+l", "right"),
			run: win(func(a *App, c call) { a.moveFocus("l") })},
		&action{name: "next_pane", desc: "next pane, then sidebar and results", keys: keys("w", "ctrl+w"),
			run: win(func(a *App, c call) { a.cycleFocus(c.n) })},
		&action{name: "prev_pane", desc: "previous pane", keys: keys("W"),
			run: win(func(a *App, c call) { a.cycleFocus(-c.n) })},
		&action{name: "close", desc: "close the pane (the tab if it's the last)", keys: keys("c"),
			run: win(func(a *App, c call) {
				switch {
				case a.tab().multi():
					a.closePane()
				case len(a.tabs) > 1:
					a.closeTab()
				default:
					a.msg = "last pane (q quits)"
				}
			})},
		&action{name: "quit", desc: "close the pane, like q", keys: keys("q", "ctrl+q"),
			run: func(a *App, c call) tea.Cmd {
				if !a.closePane() {
					return tea.Quit
				}
				a.syncTOC()
				return nil
			}},
		&action{name: "only", desc: "close every other pane", keys: keys("o", "ctrl+o"),
			run: win(func(a *App, c call) { a.tab().only(); a.layout() })},
		&action{name: "equalize", desc: "make all panes the same size", keys: keys("="),
			run: win(func(a *App, c call) { a.tab().root.equalize(); a.layout() })},
		&action{name: "wider", desc: "n columns wider", keys: keys(">"), run: resize(true, 1)},
		&action{name: "narrower", desc: "n columns narrower", keys: keys("<"), run: resize(true, -1)},
		&action{name: "taller", desc: "n rows taller", keys: keys("+"), run: resize(false, 1)},
		&action{name: "shorter", desc: "n rows shorter", keys: keys("-"), run: resize(false, -1)},
		&action{name: "scrollbind", desc: "scrollbind on / off for the pane", keys: keys("b"),
			run: win(func(a *App, c call) {
				a.pane.bind = !a.pane.bind
				a.msg = "scrollbind off"
				if a.pane.bind {
					a.msg = "scrollbind on"
				}
			})},
	)

	add("normal", "Focus and layout",
		&action{name: "focus_next", desc: "cycle focus: panes, sidebar, results", keys: keys("tab"),
			run: docOnly(func(a *App, c call) { a.cycleFocus(1) })},
		&action{name: "focus_prev", desc: "cycle focus backwards", keys: keys("shift+tab"),
			run: docOnly(func(a *App, c call) { a.cycleFocus(-1) })},
		&action{name: "toggle_toc", desc: "show / hide the table of contents", keys: keys("ctrl+t"),
			run: do(func(a *App, c call) {
				a.showTOC = !a.showTOC
				if !a.showTOC && a.focus == focusTOC {
					a.focus = focusDoc
				}
				a.layout()
			})},
	)

	tsel := func(f func(a *App, c call) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { a.tocSelect(f(a, c)) })
	}
	add("toc", "Table of contents",
		&action{name: "down", desc: "next heading (the document follows)", keys: keys("j", "down", "ctrl+n"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor + c.n })},
		&action{name: "up", desc: "previous heading", keys: keys("k", "up", "ctrl+p"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor - c.n })},
		&action{name: "half_down", desc: "half a page down", keys: keys("ctrl+d"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor + c.n*half(a) })},
		&action{name: "half_up", desc: "half a page up", keys: keys("ctrl+u"),
			run: tsel(func(a *App, c call) int { return a.toc.cursor - c.n*half(a) })},
		&action{name: "first", desc: "first heading, or heading n", keys: keys("g g", "home"),
			run: tsel(func(a *App, c call) int { return c.n - 1 })},
		&action{name: "last", desc: "last heading, or heading n", keys: keys("G", "end"),
			run: tsel(func(a *App, c call) int {
				if c.count > 0 {
					return c.count - 1
				}
				return len(a.pane.doc.Headings) - 1
			})},
		&action{name: "back", desc: "back to the document", keys: keys("enter", "l", "esc", "h"),
			run: do(func(a *App, c call) { a.focus = focusDoc })},
	)

	add("normal", "Quitting",
		&action{name: "quit", desc: "close the pane, then the tab, then quit", keys: keys("q"),
			run: func(a *App, c call) tea.Cmd {
				if !a.closePane() {
					return tea.Quit
				}
				a.syncTOC()
				return nil
			}},
		&action{name: "quit_all", desc: "quit", keys: keys("Q", "ctrl+c"),
			run: func(a *App, c call) tea.Cmd { return tea.Quit }},
	)
	add("normal", "Help",
		&action{name: "help", desc: "show the keys", keys: keys("f1", "g ?"),
			run: do(func(a *App, c call) { a.help = &Help{} })},
	)

	msel := func(f func(m *Menu, c call, rows int) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { m, rows := a.menu, a.menuRows(); m.selectIdx(f(m, c, rows), rows) })
	}
	add("menu", "File menu",
		&action{name: "down", desc: "next entry", keys: keys("j", "down", "ctrl+n"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor + c.n })},
		&action{name: "up", desc: "previous entry", keys: keys("k", "up", "ctrl+p"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor - c.n })},
		&action{name: "half_down", desc: "half a page down", keys: keys("ctrl+d", "pgdown"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor + c.n*max(rows/2, 1) })},
		&action{name: "half_up", desc: "half a page up", keys: keys("ctrl+u", "pgup"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor - c.n*max(rows/2, 1) })},
		&action{name: "first", desc: "first entry", keys: keys("g g", "home"),
			run: msel(func(m *Menu, c call, rows int) int { return 0 })},
		&action{name: "last", desc: "last entry", keys: keys("G", "end"),
			run: msel(func(m *Menu, c call, rows int) int { return len(m.shown) - 1 })},
		&action{name: "open", desc: "enter the directory, or open the file in a new tab", keys: keys("l", "right", "enter"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openNewTab) }},
		&action{name: "open_here", desc: "open the file in the focused pane", keys: keys("O"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openReplace) }},
		&action{name: "open_vsplit", desc: "open the file in a new pane beside", keys: keys("v"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openVSplit) }},
		&action{name: "open_hsplit", desc: "open the file in a new pane below", keys: keys("s"),
			run: func(a *App, c call) tea.Cmd { return a.menuActivate(openHSplit) }},
		&action{name: "parent", desc: "parent directory", keys: keys("h", "left", "-", "backspace"),
			run: do(func(a *App, c call) { a.menuUp() })},
		&action{name: "home", desc: "home directory", keys: keys("~", "g h"),
			run: do(func(a *App, c call) { a.menuHome() })},
		&action{name: "repo_root", desc: "root of the git repository", keys: keys("g r"),
			run: do(func(a *App, c call) {
				if root := search.RepoRoot(a.menu.dir); root != "" {
					a.menuLoad(root, "")
				} else {
					a.menu.err = "not in a git repository"
				}
			})},
		&action{name: "toggle_all", desc: "show / hide hidden, ignored and non-markdown files", keys: keys("."),
			run: do(func(a *App, c call) { a.menuAll = !a.menuAll; a.menu.refilter(a.menuAll, a.menuRows()) })},
		&action{name: "refresh", desc: "re-read the directory", keys: keys("r"),
			run: do(func(a *App, c call) {
				sel := ""
				if e, ok := a.menu.current(); ok {
					sel = e.name
				}
				a.menuLoad(a.menu.dir, sel)
			})},
		&action{name: "filter", desc: "filter names", keys: keys("/"),
			run: func(a *App, c call) tea.Cmd { a.menu.filtering = true; return a.menu.filter.Focus() }},
		&action{name: "close", desc: "close the menu (esc clears a filter first)", keys: keys("esc", "q", "o", "ctrl+c"),
			run: func(a *App, c call) tea.Cmd {
				if c.key == "esc" && a.menu.filter.Value() != "" {
					a.menu.filter.SetValue("")
					a.menu.refilter(a.menuAll, a.menuRows())
					return nil
				}
				return a.closeMenu()
			}},
		&action{name: "quit", desc: "quit", keys: keys("Q"),
			run: func(a *App, c call) tea.Cmd { return tea.Quit }},
	)
	add("filter", "Menu filter",
		&action{name: "accept", desc: "stop filtering and open the selection", keys: keys("enter"),
			run: func(a *App, c call) tea.Cmd { a.menu.stopFilter(); return a.menuActivate(openNewTab) }},
		&action{name: "cancel", desc: "clear the filter and stop", keys: keys("esc"),
			run: do(func(a *App, c call) { a.menu.stopFilter(); a.menu.setFilter("", a.menuAll, a.menuRows()) })},
		&action{name: "clear", desc: "clear the filter (again to stop)", keys: keys("ctrl+c"),
			run: do(func(a *App, c call) {
				if a.menu.filter.Value() == "" {
					a.menu.stopFilter()
				} else {
					a.menu.setFilter("", a.menuAll, a.menuRows())
				}
			})},
		&action{name: "down", desc: "next entry", keys: keys("down", "ctrl+n"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor + 1 })},
		&action{name: "up", desc: "previous entry", keys: keys("up", "ctrl+p"),
			run: msel(func(m *Menu, c call, rows int) int { return m.cursor - 1 })},
	)

	hscroll := func(f func(a *App, c call) int) func(a *App, c call) tea.Cmd {
		return do(func(a *App, c call) { a.help.scroll(f(a, c), a.helpRows(), len(a.helpLines())) })
	}
	add("help", "Help overlay",
		&action{name: "down", desc: "scroll down", keys: keys("j", "down", "ctrl+n", "ctrl+e", "enter"),
			run: hscroll(func(a *App, c call) int { return c.n })},
		&action{name: "up", desc: "scroll up", keys: keys("k", "up", "ctrl+p", "ctrl+y"),
			run: hscroll(func(a *App, c call) int { return -c.n })},
		&action{name: "page_down", desc: "a page down", keys: keys("ctrl+d", "ctrl+f", "pgdown", "space"),
			run: hscroll(func(a *App, c call) int { return c.n * max(a.helpRows()-1, 1) })},
		&action{name: "page_up", desc: "a page up", keys: keys("ctrl+u", "ctrl+b", "pgup", "b"),
			run: hscroll(func(a *App, c call) int { return -c.n * max(a.helpRows()-1, 1) })},
		&action{name: "top", desc: "top", keys: keys("g g", "home"),
			run: hscroll(func(a *App, c call) int { return -len(a.helpLines()) })},
		&action{name: "bottom", desc: "bottom", keys: keys("G", "end"),
			run: hscroll(func(a *App, c call) int { return len(a.helpLines()) })},
		&action{name: "close", desc: "close the help", keys: keys("esc", "q", "f1", "g ?"),
			run: do(func(a *App, c call) { a.help = nil })},
	)
}

// Keymap is the effective bindings: the defaults with the settings file's
// changes.
type Keymap struct {
	keys     map[*action][]string
	bind     map[string]map[string]*action // context -> key sequence -> action
	prefixes map[string]map[string]bool    // context -> incomplete sequences
}

// singleKeyContexts are the typing contexts, where a sequence can't start.
var singleKeyContexts = map[string]bool{"search": true, "filter": true}

// DefaultKeymap is the keymap with no changes.
func DefaultKeymap() *Keymap {
	km, err := NewKeymap(nil)
	if err != nil {
		panic(err)
	}
	return km
}

// NewKeymap applies changes, changes[context][action] = keys, to the
// defaults. A key given to an action is taken from any other action in the
// same context.
func NewKeymap(changes map[string]map[string][]string) (*Keymap, error) {
	km := &Keymap{keys: map[*action][]string{}}
	byName := map[string]map[string]*action{}
	for _, x := range actions {
		if byName[x.ctx] == nil {
			byName[x.ctx] = map[string]*action{}
		}
		byName[x.ctx][x.name] = x
		km.keys[x] = x.keys
	}
	for _, ctx := range config.SortedKeys(changes) {
		names, ok := byName[ctx]
		if !ok {
			return nil, fmt.Errorf("keys.%s: unknown context (have %s)", ctx, strings.Join(config.SortedKeys(byName), ", "))
		}
		for _, name := range config.SortedKeys(changes[ctx]) {
			x, ok := names[name]
			if !ok {
				return nil, fmt.Errorf("keys.%s.%s: unknown action", ctx, name)
			}
			var ks []string
			for _, k := range changes[ctx][name] {
				k, err := normalizeKey(k)
				if err != nil {
					return nil, fmt.Errorf("keys.%s.%s: %w", ctx, name, err)
				}
				if singleKeyContexts[ctx] && strings.Contains(k, " ") {
					return nil, fmt.Errorf("keys.%s.%s: %q: sequences don't work while typing", ctx, name, k)
				}
				ks = append(ks, k)
			}
			for other, oks := range km.keys {
				if other.ctx == ctx && other != x {
					km.keys[other] = without(oks, ks)
				}
			}
			km.keys[x] = ks
		}
	}

	km.bind = map[string]map[string]*action{}
	km.prefixes = map[string]map[string]bool{}
	for _, x := range actions {
		if km.bind[x.ctx] == nil {
			km.bind[x.ctx] = map[string]*action{}
			km.prefixes[x.ctx] = map[string]bool{}
		}
		for _, k := range km.keys[x] {
			km.bind[x.ctx][k] = x
			parts := strings.Split(k, " ")
			for i := 1; i < len(parts); i++ {
				km.prefixes[x.ctx][strings.Join(parts[:i], " ")] = true
			}
		}
	}
	for ctx, prefixes := range km.prefixes {
		for p := range prefixes {
			if x := km.bind[ctx][p]; x != nil {
				return nil, fmt.Errorf("keys.%s: %q is bound to %s but also starts a longer sequence", ctx, p, x.name)
			}
		}
	}
	return km, nil
}

// normalizeKey tidies a key from the settings file: extra spaces go, and
// "ctrl-d" becomes "ctrl+d" as tea spells it.
func normalizeKey(k string) (string, error) {
	parts := strings.Fields(k)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty key")
	}
	for i, p := range parts {
		for _, mod := range []string{"ctrl-", "alt-", "shift-", "super-"} {
			for strings.HasPrefix(p, mod) && len(p) > len(mod) {
				p = strings.TrimSuffix(mod, "-") + "+" + p[len(mod):]
			}
		}
		if len(p) == 1 && p[0] >= '0' && p[0] <= '9' && i == 0 {
			return "", fmt.Errorf("%q: digits are counts", k)
		}
		if p == " " {
			p = "space"
		}
		parts[i] = p
	}
	return strings.Join(parts, " "), nil
}

func without(xs, drop []string) []string {
	var out []string
	for _, x := range xs {
		keep := true
		for _, d := range drop {
			if x == d {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, x)
		}
	}
	return out
}

// lookup finds what key sequence seq does in the contexts, in order: an
// action, or more keys to come.
func (km *Keymap) lookup(seq string, ctxs ...string) (x *action, prefix bool) {
	for _, c := range ctxs {
		if x := km.bind[c][seq]; x != nil {
			return x, false
		}
	}
	for _, c := range ctxs {
		if km.prefixes[c][seq] {
			return nil, true
		}
	}
	return nil, false
}

// Keys returns the keys bound to an action.
func (km *Keymap) Keys(ctx, name string) []string {
	for _, x := range actions {
		if x.ctx == ctx && x.name == name {
			return km.keys[x]
		}
	}
	return nil
}

// Bindings lists every action with its default keys, for -dump-config.
func Bindings() []config.Binding {
	var out []config.Binding
	for _, ctx := range []string{"normal", "window", "toc", "results", "search", "menu", "filter", "help"} {
		for _, x := range actions {
			if x.ctx == ctx {
				out = append(out, config.Binding{Context: x.ctx, Action: x.name, Keys: x.keys, Desc: x.desc})
			}
		}
	}
	return out
}

// displayKey shows a key sequence compactly: "g g" as gg, "ctrl+w v" as is.
func displayKey(k string) string {
	parts := strings.Split(k, " ")
	for _, p := range parts {
		if len(p) != 1 {
			return k
		}
	}
	return strings.Join(parts, "")
}

// display is how the help and KEYS.md show an action's keys: window keys
// get the prefix in front.
func (km *Keymap) display(x *action) []string {
	prefix := ""
	if x.ctx == "window" {
		if p := km.Keys("normal", "window"); len(p) > 0 {
			prefix = displayKey(p[0]) + " "
		}
	}
	out := make([]string, 0, len(km.keys[x]))
	for _, k := range km.keys[x] {
		out = append(out, prefix+displayKey(k))
	}
	return out
}

// dispatch runs key k in the contexts, tracking sequences in *pending. It
// reports whether the key was used, either as an action or as the start of
// a sequence.
func (a *App) dispatch(pending *string, k string, count int, ctxs ...string) (tea.Cmd, bool) {
	seq := k
	if *pending != "" {
		seq = *pending + " " + k
	}
	x, prefix := a.keymap.lookup(seq, ctxs...)
	switch {
	case x != nil:
		*pending = ""
		return x.run(a, call{count: count, n: max(count, 1), key: k}), true
	case prefix:
		*pending = seq
		return nil, true
	}
	*pending = ""
	return nil, false
}

// windowKey is how the status line shows the pending window prefix.
func (a *App) windowKey() string {
	if p := a.keymap.Keys("normal", "window"); len(p) > 0 {
		return displayKey(p[0])
	}
	return "window"
}

// hint is an action's first key, for the hints in the bars.
func (a *App) hint(ctx, name string) string {
	if k := a.keymap.Keys(ctx, name); len(k) > 0 {
		return displayKey(k[0])
	}
	return "unbound"
}

// Markdown is KEYS.md: every group's actions in a table, with the name the
// settings file uses for each.
func (km *Keymap) Markdown() string {
	var b strings.Builder
	b.WriteString(`# joe-md keys

<!-- Generated from the action table in internal/ui/keys.go; don't edit by hand.
     Regenerate with: UPDATE_KEYS_MD=1 go test ./internal/ui -run TestKeysMD -->

Counts work like vim (` + "`5j`, `3]]`, `42G`, `3 ctrl+w >`" + `). Inside joe-md, ` + "`f1`" + ` or
` + "`g?`" + ` shows this list. Every key can be changed in the settings file
(` + "`joe-md -dump-config`" + ` prints one to start from): the Name column is the
action's name there, under ` + "`[keys.<context>]`" + `. The [README](README.md)
explains each feature.
`)
	for _, g := range groups {
		type row struct{ keys, desc, name string }
		var rows []row
		kw, dw, nw := len("Key"), len("Action"), len("Name")
		for _, x := range actions {
			if x.group != g {
				continue
			}
			var ks []string
			for _, k := range km.display(x) {
				ks = append(ks, "`"+strings.ReplaceAll(k, "|", `\|`)+"`")
			}
			r := row{strings.Join(ks, " "), x.desc, "`" + x.ctx + "." + x.name + "`"}
			if r.keys == "" {
				r.keys = "(unbound)"
			}
			rows = append(rows, r)
			kw, dw, nw = max(kw, ansi.StringWidth(r.keys)), max(dw, ansi.StringWidth(r.desc)), max(nw, len(r.name))
		}
		pad := func(s string, w int) string { return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) }
		fmt.Fprintf(&b, "\n## %s\n\n", g)
		fmt.Fprintf(&b, "| %s | %s | %s |\n", pad("Key", kw), pad("Action", dw), pad("Name", nw))
		fmt.Fprintf(&b, "|%s|%s|%s|\n", strings.Repeat("-", kw+2), strings.Repeat("-", dw+2), strings.Repeat("-", nw+2))
		for _, r := range rows {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", pad(r.keys, kw), pad(r.desc, dw), pad(r.name, nw))
		}
		if n := groupNotes[g]; n != "" {
			fmt.Fprintf(&b, "\n%s\n", n)
		}
	}
	b.WriteString(`
## Mouse

Click a tab to switch, middle-click to close it, and scroll over the tab bar
to step through tabs. Click a pane to focus it, drag a border to resize it,
and scroll to move the pane under the pointer. Click a heading in the sidebar
to jump to it. In the file menu, click an entry to open it and click outside
to close it; the same goes for the help.
`)
	return b.String()
}
