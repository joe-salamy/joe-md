// Package ui is the joe-md terminal interface.
package ui

import (
	"cmp"
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joe-salamy/joe-md/internal/config"
	"github.com/joe-salamy/joe-md/internal/doc"
	"github.com/joe-salamy/joe-md/internal/editor"
	"github.com/joe-salamy/joe-md/internal/search"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type Options struct {
	Style   string // glamour style name or JSON path
	Dark    bool   // terminal has a dark background
	MaxWrap int    // maximum word-wrap width
	NoTOC   bool   // start with the sidebar hidden
	NoBar   bool   // start with the search bar hidden
	NoTabs  bool   // start with the tab bar hidden
	MenuDir string // start with the file menu open here

	Keymap *Keymap      // nil for the default keys
	Colors config.Theme // colour changes; empty ones keep the defaults
	Scope  string       // scope ? starts in: file, dir or repo (the default)
	Mode   search.Mode  // literal and case of searches
}

type focus int

const (
	focusDoc focus = iota
	focusTOC
	focusResults
)

const wheelStep = 3

type App struct {
	opts     Options
	theme    theme
	renderer *doc.Renderer
	showTOC  bool

	// Tabs and panes; see tabs.go and split.go. pane is the current tab's
	// focused pane and toc its sidebar state; both are nil only while the
	// start-up menu is open with nothing else to show.
	tabs     []*Tab
	cur      int
	pane     *Pane
	toc      *TOC
	showTabs bool
	drag     *border // pane border being dragged with the mouse

	menu    *Menu // the pop-up file menu, nil when closed; see menu.go
	menuAll bool  // the menu lists hidden and non-markdown files too

	focus  focus
	width  int
	height int

	keymap  *Keymap
	count   int    // pending numeric prefix, 0 if none
	pending string // the keys so far of a sequence ("g", "]", ...)
	window  bool   // ctrl+w was pressed: the next key is a window command
	msg     string // one-shot status message
	help    *Help  // the help overlay, nil when closed; see help.go

	// Search bar and results; see search.go.
	input       textinput.Model
	typing      bool         // the search bar has the keyboard
	showBar     bool         // keep the search bar visible when not typing
	scope       search.Scope // scope of the search being typed
	crossScope  search.Scope // scope ? starts in: the last cross-file scope used
	mode        search.Mode  // literal and case for the next search
	history     []string
	histIdx     int    // position while recalling history; len(history) when not
	draft       string // what was typed before recalling history
	searching   string // query of the running interactive search, or ""
	seq         int    // id of the latest search; older results are dropped
	cancel      context.CancelFunc
	results     *Results
	showResults bool
}

// New opens one tab per document. With none, or with opts.MenuDir set, it
// starts with the file menu open.
func New(docs []*doc.Doc, opts Options) *App {
	a := &App{
		opts:       opts,
		theme:      newTheme(opts.Dark, opts.Style, opts.Colors),
		renderer:   doc.NewRenderer(opts.Style),
		keymap:     opts.Keymap,
		showTOC:    !opts.NoTOC,
		showTabs:   !opts.NoTabs,
		input:      newInput(opts.Dark),
		showBar:    !opts.NoBar,
		crossScope: search.Repo,
		mode:       opts.Mode,
	}
	if a.keymap == nil {
		a.keymap = DefaultKeymap()
	}
	if s, ok := search.ParseScope(opts.Scope); ok {
		a.crossScope = s
	}
	a.setPlaceholder()
	for _, d := range docs {
		a.tabs = append(a.tabs, newTab(NewPane(d)))
	}
	a.activate(0)
	if opts.MenuDir != "" || len(docs) == 0 {
		dir := cmp.Or(opts.MenuDir, ".")
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		a.openMenu(dir, "")
	}
	return a
}

func (a *App) Init() tea.Cmd { return nil }

// bodyHeight is the height of the document and sidebar: the screen minus the
// tab bar, status line, search bar and results panel.
func (a *App) bodyHeight() int {
	return max(a.height-1-a.tabBarHeight()-a.barHeight()-a.panelHeight(), 1)
}

// tocWidth is the sidebar width. It fits the widest of the tab's documents,
// so moving between panes does not shift them.
func (a *App) tocWidth() int {
	if !a.showTOC || a.pane == nil {
		return 0
	}
	w := 0
	for _, l := range a.tab().leaves() {
		w = max(w, l.pane.toc.Width(l.pane.doc, a.width))
	}
	return w
}

// bodyX is the screen column where the panes start, right of the sidebar.
func (a *App) bodyX() int {
	if a.showTOC && a.pane != nil {
		return a.tocWidth() + 1 // +1 for the separator column
	}
	return 0
}

// layout sizes and re-renders the current tab's panes after a resize, a
// split or a sidebar toggle.
func (a *App) layout() {
	if a.pane == nil || a.width == 0 {
		return
	}
	t := a.tab()
	x := a.bodyX()
	t.root.place(x, 0, max(a.width-x, 1), a.bodyHeight())
	status := 0
	if t.multi() {
		status = 1
	}
	for _, l := range t.leaves() {
		if err := l.pane.Layout(a.renderer, l.w, max(l.h-status, 1), a.opts.MaxWrap); err != nil {
			a.msg = "render error: " + err.Error()
		}
	}
	a.syncTOC()
}

// syncTOC points the sidebar cursor at the section being read, unless the
// user is driving the sidebar.
func (a *App) syncTOC() {
	if a.focus == focusDoc && a.pane != nil {
		a.toc.Select(a.pane.doc, a.pane.CurrentHeading(), a.bodyHeight())
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	snap := a.scrollSnapshot()
	cmd := a.update(msg)
	a.scrollBind(snap)
	return a, cmd
}

func (a *App) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.layout()
		if a.menu != nil {
			a.menu.offset = 0 // re-scroll the cursor into the new size
			a.menu.selectIdx(a.menu.cursor, a.menuRows())
		}
	case tea.KeyPressMsg:
		if a.menu != nil {
			a.msg = ""
			return a.menuKey(msg)
		}
		return a.key(msg)
	case tea.MouseWheelMsg:
		a.msg = ""
		switch {
		case a.menu != nil:
			return a.menuMouse(msg)
		case a.help != nil:
			a.helpMouse(msg)
		default:
			a.wheel(msg.Mouse())
		}
	case tea.MouseClickMsg:
		a.msg = ""
		switch {
		case a.menu != nil:
			return a.menuMouse(msg)
		case a.help != nil:
			a.helpMouse(msg)
			return nil
		}
		return a.click(msg.Mouse())
	case tea.MouseMotionMsg:
		if a.drag != nil {
			m := msg.Mouse()
			a.drag.drag(m.X, m.Y-a.tabBarHeight())
			a.layout()
		}
	case tea.MouseReleaseMsg:
		a.drag = nil
	case editDoneMsg:
		return a.editDone(msg)
	case searchDoneMsg:
		a.searchDone(msg)
	default:
		if a.menu != nil && a.menu.filtering {
			var cmd tea.Cmd
			a.menu.filter, cmd = a.menu.filter.Update(msg)
			return cmd
		}
		if a.typing { // cursor blink and friends
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			return cmd
		}
	}
	return nil
}

func (a *App) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	a.msg = ""
	if a.typing {
		return a.inputKey(msg)
	}
	if a.help != nil {
		cmd, _ := a.dispatch(&a.help.pending, k, 0, "help")
		return cmd
	}
	// Digits are a count, before a command or between ctrl+w and its key.
	if len(k) == 1 && k[0] >= '0' && k[0] <= '9' && a.pending == "" && (a.count > 0 || k != "0") {
		a.count = min(a.count*10+int(k[0]-'0'), 1_000_000)
		return nil
	}
	count := a.count
	a.count = 0

	var ctxs []string
	switch {
	case a.window:
		ctxs = []string{"window"}
	case a.focus == focusTOC:
		ctxs = []string{"toc", "normal"}
	case a.focus == focusResults:
		ctxs = []string{"results", "normal"}
	default:
		ctxs = []string{"normal"}
	}
	win := a.window
	cmd, _ := a.dispatch(&a.pending, k, count, ctxs...)
	if a.pending != "" {
		a.count = count // keep the count for the rest of the sequence
	} else if win {
		a.window = false
	}
	a.syncTOC()
	return cmd
}

// moveFocus is ctrl+w h/j/k/l. Past the edge panes, h reaches the sidebar
// and j the results panel.
func (a *App) moveFocus(dir string) {
	switch a.focus {
	case focusTOC:
		if dir == "l" {
			a.focus = focusDoc
		}
		return
	case focusResults:
		if dir == "k" {
			a.focus = focusDoc
		}
		return
	}
	if n := a.tab().neighbor(dir); n != nil {
		a.focusLeaf(n)
		return
	}
	switch {
	case dir == "h" && a.showTOC:
		a.focus = focusTOC
	case dir == "j" && a.showResults:
		a.focus = focusResults
	}
}

// tocSelect moves the sidebar cursor and scrolls the document along with it,
// so browsing the table of contents previews each section.
func (a *App) tocSelect(i int) {
	a.toc.Select(a.pane.doc, i, a.bodyHeight())
	a.pane.GotoHeading(a.toc.cursor)
}

// cycleFocus moves focus d steps through the visible parts: the sidebar,
// each pane in order, then the results panel.
func (a *App) cycleFocus(d int) {
	type stop struct {
		f focus
		n *Node
	}
	var order []stop
	cur := 0
	add := func(f focus, show bool) {
		if show {
			if a.focus == f {
				cur = len(order)
			}
			order = append(order, stop{f, a.tab().focus})
		}
	}
	add(focusTOC, a.showTOC)
	for _, l := range a.tab().leaves() {
		if a.focus == focusDoc && l == a.tab().focus {
			cur = len(order)
		}
		order = append(order, stop{focusDoc, l})
	}
	add(focusResults, a.showResults)
	s := order[((cur+d)%len(order)+len(order))%len(order)]
	a.focus = s.f
	if s.n != a.tab().focus {
		a.focusLeaf(s.n)
	}
	a.syncTOC()
}

// editDoneMsg arrives when micro exits.
type editDoneMsg struct {
	session *editor.Session
	warn    string // plugin install problem, reported once micro exits
	err     error
}

// edit suspends the viewer and opens micro with its cursor on the line at the
// top of the pane.
func (a *App) edit() tea.Cmd {
	cmd, s, err := editor.Command(a.pane.doc.Path, a.pane.TopSource()+1)
	if cmd == nil {
		a.msg = "edit: " + err.Error()
		return nil
	}
	warn := ""
	if err != nil {
		warn = "micro plugin not installed (" + err.Error() + "), line not tracked"
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editDoneMsg{s, warn, err} })
}

// editDone reloads the file and puts micro's final cursor line at the top of
// the pane: the same line that went into micro comes back out. If the cursor
// never moved, the view stays exactly where it was.
func (a *App) editDone(m editDoneMsg) tea.Cmd {
	line := m.session.Result()
	changed, cmd := a.reload()
	if line != m.session.Line {
		a.pane.GotoSource(min(line, a.pane.doc.Lines) - 1)
	}
	a.syncTOC()
	switch {
	case m.err != nil:
		a.msg = "micro: " + m.err.Error()
	case m.warn != "":
		a.msg = m.warn
	case changed:
		a.msg = "reloaded after edit"
	}
	return cmd
}

// reload re-reads the focused pane's file from disk, in every pane showing
// it, and reports whether it changed. The command re-runs the searches of
// the panes that changed. Errors go to the status line.
func (a *App) reload() (bool, tea.Cmd) {
	path := a.pane.doc.Path
	changed := false
	var cmds []tea.Cmd
	for _, p := range a.allPanes() {
		if !samePath(p.doc.Path, path) {
			continue
		}
		c, err := p.Reload(a.renderer, a.opts.MaxWrap)
		if err != nil {
			a.msg = "reload failed: " + err.Error()
		}
		if c {
			changed = true
			cmds = append(cmds, a.refreshSearch(p))
		}
	}
	if changed {
		a.layout()
	}
	return changed, tea.Batch(cmds...)
}

// gotoSourceLine jumps to 1-based source line n, like vim's :n.
func (a *App) gotoSourceLine(n int) {
	a.pane.GotoSource(min(n, a.pane.doc.Lines) - 1)
	a.msg = "line " + strconv.Itoa(n)
}

func (a *App) inTOC(m tea.Mouse) bool {
	return a.showTOC && m.X < a.tocWidth() && m.Y < a.bodyHeight()
}

// inPanel reports whether m is over the results panel, and on which row.
func (a *App) inPanel(m tea.Mouse) (row int, ok bool) {
	row = m.Y - a.bodyHeight()
	return row, a.panelHeight() > 0 && row >= 0 && row < a.panelHeight()
}

func (a *App) wheel(m tea.Mouse) {
	if m.Y < a.tabBarHeight() {
		if m.Button == tea.MouseWheelUp {
			a.stepTab(-1)
		} else if m.Button == tea.MouseWheelDown {
			a.stepTab(1)
		}
		return
	}
	m.Y -= a.tabBarHeight() // from here on, rows count from the top of the body
	step := wheelStep
	if m.Button == tea.MouseWheelUp {
		step = -step
	} else if m.Button != tea.MouseWheelDown {
		return
	}
	if _, ok := a.inPanel(m); ok {
		a.results.Scroll(step, a.resultRows())
		return
	}
	if a.inTOC(m) {
		a.toc.Scroll(a.pane.doc, step, a.bodyHeight())
		return
	}
	if a.pane == nil {
		return
	}
	// The wheel scrolls the pane under the mouse, focused or not.
	if l := a.tab().leafAt(m.X, m.Y); l != nil {
		l.pane.ScrollBy(step)
	}
	a.syncTOC()
}

func (a *App) click(m tea.Mouse) tea.Cmd {
	if m.Y < a.tabBarHeight() {
		// Left click switches tabs, middle click closes them, like browsers.
		if i := a.tabAt(m.X); i >= 0 {
			switch m.Button {
			case tea.MouseLeft:
				a.activate(i)
				a.syncTOC()
			case tea.MouseMiddle:
				a.activate(i)
				if !a.closeTab() {
					return tea.Quit
				}
			}
		}
		return nil
	}
	if m.Button != tea.MouseLeft {
		return nil
	}
	m.Y -= a.tabBarHeight() // from here on, rows count from the top of the body
	if row, ok := a.inPanel(m); ok {
		if i := a.results.At(row); i >= 0 {
			a.results.Select(i, a.resultRows())
			a.openResult(openNewTab)
		}
		return nil
	}
	if a.barHeight() > 0 && m.Y == a.bodyHeight()+a.panelHeight() {
		if !a.typing {
			return a.startSearch(search.File)
		}
		return nil
	}
	if !a.inTOC(m) {
		if m.Y >= a.bodyHeight() || a.pane == nil {
			return nil
		}
		t := a.tab()
		a.drag = t.root.borderAt(m.X, m.Y)
		if l := t.leafAt(m.X, m.Y); l != nil {
			a.focus = focusDoc
			if l != t.focus {
				a.focusLeaf(l)
			}
		}
		return nil
	}
	if i := a.toc.HeadingAt(a.pane.doc, m.Y); i >= 0 {
		a.tocSelect(i)
	}
	return nil
}

func (a *App) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "joe-md"
	if a.pane != nil {
		v.WindowTitle += ": " + a.pane.doc.Name
	}
	if a.width == 0 || a.height == 0 {
		return v
	}

	var screen []string
	if a.tabBarHeight() > 0 {
		screen = append(screen, a.tabBar())
	}
	body := make([]string, a.bodyHeight())
	if a.pane != nil {
		body = a.renderNode(a.tab(), a.tab().root)
	}
	if a.showTOC && a.pane != nil {
		tw := a.tocWidth()
		side := a.toc.Render(a.pane.doc, a.pane.CurrentHeading(), a.focus == focusTOC, tw, len(body), a.theme)
		sep := a.theme.separator.Render("│")
		for y := range body {
			body[y] = side[y] + "\x1b[0m" + sep + body[y]
		}
	}

	screen = append(screen, body...)
	if h := a.panelHeight(); h > 0 {
		screen = append(screen, a.results.Render(a.width, h, a.focus == focusResults, a.theme)...)
	}
	if a.barHeight() > 0 {
		screen = append(screen, a.barView())
	}
	screen = append(screen, a.statusLine())
	if a.menu != nil {
		x, y, _, _ := a.menuRect()
		overlay(screen, a.menuView(), x, y, a.width)
	}
	if a.help != nil {
		x, y, _, _ := a.helpRect()
		overlay(screen, a.helpView(), x, y, a.width)
	}
	v.SetContent(strings.Join(screen, "\n"))
	return v
}

func (a *App) statusLine() string {
	th := a.theme
	mode := th.statusMode.Render(" DOC ")
	switch {
	case a.help != nil:
		mode = th.statusModeTOC.Render(" HELP ")
	case a.menu != nil:
		mode = th.statusModeTOC.Render(" MENU ")
	case a.typing:
		mode = th.statusModeSearch.Render(" SEARCH ")
	case a.focus == focusTOC:
		mode = th.statusModeTOC.Render(" TOC ")
	case a.focus == focusResults:
		mode = th.statusModeSearch.Render(" RESULTS ")
	}
	if a.pane == nil {
		return fit(mode+th.status.Render(" "+a.msg+strings.Repeat(" ", a.width)), a.width)
	}
	left := mode + th.status.Render(" "+a.pane.doc.Name+" ")
	if len(a.tabs) > 1 && a.tabBarHeight() == 0 {
		left += th.statusDim.Render("[" + strconv.Itoa(a.cur+1) + "/" + strconv.Itoa(len(a.tabs)) + "] ")
	}
	if a.msg != "" {
		left += th.statusDim.Render(" " + a.msg + " ")
	}

	keys := displayKey(a.pending)
	if a.window {
		keys = strings.TrimSpace(a.windowKey() + " " + keys)
	}
	if a.count > 0 {
		keys = strconv.Itoa(a.count) + keys
	}
	pos := "Ln " + strconv.Itoa(a.pane.TopSource()+1) + "/" + strconv.Itoa(a.pane.doc.Lines) +
		"  " + a.pane.Percent() + " "
	right := th.statusDim.Render(keys+"   ") + th.status.Render(pos)

	gap := a.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return ansi.Truncate(left+th.status.Render(" ")+right, a.width, "")
	}
	return left + th.status.Render(strings.Repeat(" ", gap)) + right
}
