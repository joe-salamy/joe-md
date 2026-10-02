package ui

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joe-salamy/joe-md/internal/natural"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The menu is a pop-up file browser over the document that lists one
// directory at a time, like lf. Directories are entered and left with l / h,
// and files open in tabs. The name filter runs as you type because it only
// scans one directory's names, which can't lag the way searching file
// contents would.

// Menu is the open pop-up's state. showAll lives on App so it survives
// closing the menu.
type Menu struct {
	list      // over shown
	dir       string
	entries   []entry
	shown     []int // indexes into entries that pass the hidden and filter rules
	filter    textinput.Model
	filtering bool   // the filter input has the keyboard
	pending   string // the keys so far of a sequence ("g")
	err       string
}

type entry struct {
	name   string
	dir    bool
	parent bool // the ../ entry
	hidden bool // a dotfile, or ignored by git
	md     bool
}

var markdownExts = map[string]bool{
	".md": true, ".markdown": true, ".mdown": true, ".mdwn": true, ".mkd": true,
	".mkdn": true, ".mdx": true, ".mdtext": true, ".mdtxt": true,
}

func isMarkdown(name string) bool { return markdownExts[strings.ToLower(filepath.Ext(name))] }

// openMenu pops the menu up at dir with the cursor on the entry named sel.
func (a *App) openMenu(dir, sel string) tea.Cmd {
	a.menu = &Menu{filter: newInput(a.opts.Dark, "filter names")}
	if !a.menuLoad(dir, sel) {
		a.menu.dir = dir // show the error in an empty listing
	}
	return nil
}

// menuHere is o: the menu at the current file's directory, on that file.
func (a *App) menuHere() tea.Cmd {
	if a.pane != nil {
		return a.openMenu(filepath.Dir(a.pane.doc.Path), a.pane.doc.Name)
	}
	dir, err := filepath.Abs(".")
	if err != nil {
		dir = "."
	}
	return a.openMenu(dir, "")
}

// closeMenu hides the menu. With no tabs open there is nothing to go back to,
// so it quits instead.
func (a *App) closeMenu() tea.Cmd {
	a.menu = nil
	if len(a.tabs) == 0 {
		return tea.Quit
	}
	return nil
}

// menuLoad lists dir, putting the cursor on sel. On error the menu stays
// where it was and shows the error.
func (a *App) menuLoad(dir, sel string) bool {
	m := a.menu
	es, err := listDir(dir)
	if err != nil {
		m.err = err.Error()
		return false
	}
	m.dir, m.entries, m.err = dir, es, ""
	m.shown = nil // indexes the old listing
	m.filter.SetValue("")
	m.filtering = false
	m.filter.Blur()
	m.cursor, m.offset = 0, 0
	m.refilter(a.menuAll, a.menuRows())
	m.selectName(sel, a.menuRows())
	return true
}

// listDir reads dir: directories first, then files, each sorted by name
// ignoring case, after a ../ entry unless dir is the root.
func listDir(dir string) ([]entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	es := make([]entry, 0, len(des)+1)
	for _, de := range des {
		name := de.Name()
		isDir := de.IsDir()
		if de.Type()&fs.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = fi.IsDir()
			}
		}
		es = append(es, entry{name: name, dir: isDir, hidden: strings.HasPrefix(name, "."), md: !isDir && isMarkdown(name)})
	}
	ignored := gitIgnored(dir, es)
	for i := range es {
		if ignored[es[i].name] {
			es[i].hidden = true
		}
	}
	slices.SortFunc(es, func(a, b entry) int {
		if a.dir != b.dir {
			return natural.DirsFirst(a.dir)
		}
		return natural.Compare(a.name, b.name)
	})
	if filepath.Dir(dir) != dir {
		es = append([]entry{{name: "..", dir: true, parent: true}}, es...)
	}
	return es, nil
}

// gitIgnored asks git which entries of dir are ignored. Outside a repository
// (or without git) nothing is.
func gitIgnored(dir string, es []entry) map[string]bool {
	var in bytes.Buffer
	for _, e := range es {
		if e.hidden {
			continue // hidden anyway, and git refuses paths inside .git
		}
		in.WriteString(e.name)
		if e.dir {
			in.WriteByte('/') // so patterns like build/ match
		}
		in.WriteByte(0)
	}
	if in.Len() == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "check-ignore", "-z", "--stdin")
	cmd.Stdin = &in
	out, _ := cmd.Output() // exit 1 means none ignored, 128 not a repository
	ignored := map[string]bool{}
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			ignored[strings.TrimSuffix(string(p), "/")] = true
		}
	}
	return ignored
}

// refilter recomputes the visible entries, keeping the cursor on the same
// entry when it is still visible.
func (m *Menu) refilter(showAll bool, rows int) {
	sel := ""
	if e, ok := m.current(); ok {
		sel = e.name
	}
	terms := strings.Fields(strings.ToLower(m.filter.Value()))
	m.shown = m.shown[:0]
	for i, e := range m.entries {
		switch {
		case e.parent && len(terms) > 0:
			continue
		case !showAll && !e.parent && (e.hidden || !e.dir && !e.md):
			continue
		}
		name := strings.ToLower(e.name)
		ok := true
		for _, t := range terms {
			if !strings.Contains(name, t) {
				ok = false
				break
			}
		}
		if ok {
			m.shown = append(m.shown, i)
		}
	}
	m.cursor, m.offset = 0, 0
	m.selectName(sel, rows)
}

func (m *Menu) current() (entry, bool) {
	if m.cursor < 0 || m.cursor >= len(m.shown) {
		return entry{}, false
	}
	return m.entries[m.shown[m.cursor]], true
}

// selectName puts the cursor on the visible entry called name, if any. An
// empty name skips the ../ entry.
func (m *Menu) selectName(name string, rows int) {
	for i, j := range m.shown {
		e := m.entries[j]
		if name == "" && !e.parent || name != "" && e.name == name {
			m.selectIdx(i, rows)
			return
		}
	}
	m.selectIdx(0, rows)
}

func (m *Menu) selectIdx(i, rows int) { m.list.selectIdx(i, len(m.shown), rows) }

func (m *Menu) scroll(n, rows int) { m.list.scroll(n, len(m.shown), rows) }

// menuActivate is l / enter: go into a directory or open a file.
func (a *App) menuActivate(mode openMode) tea.Cmd {
	m := a.menu
	e, ok := m.current()
	switch {
	case !ok:
		return nil
	case e.parent:
		a.menuUp()
	case e.dir:
		a.menuLoad(filepath.Join(m.dir, e.name), "")
	default:
		if a.openAt(filepath.Join(m.dir, e.name), mode) {
			a.menu = nil
			a.focus = focusDoc
		} else {
			m.err, a.msg = a.msg, ""
		}
	}
	return nil
}

// menuUp is h: the parent directory, with the cursor on where we came from.
func (a *App) menuUp() {
	dir := a.menu.dir
	if parent := filepath.Dir(dir); parent != dir {
		a.menuLoad(parent, filepath.Base(dir))
	}
}

func (a *App) menuKey(msg tea.KeyPressMsg) tea.Cmd {
	m, k := a.menu, msg.String()
	if m.filtering {
		var none string
		if cmd, ok := a.dispatch(&none, k, 0, ctxFilter); ok {
			return cmd
		}
		var cmd tea.Cmd
		before := m.filter.Value()
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != before {
			m.refilter(a.menuAll, a.menuRows())
		}
		return cmd
	}
	cmd, _ := a.dispatch(&m.pending, k, 0, ctxMenu)
	return cmd
}

func (m *Menu) stopFilter() {
	m.filtering = false
	m.filter.Blur()
}

func (m *Menu) setFilter(s string, showAll bool, rows int) {
	m.filter.SetValue(s)
	m.refilter(showAll, rows)
}

func (a *App) menuHome() {
	if home, err := os.UserHomeDir(); err == nil {
		a.menuLoad(home, "")
	}
}

// menuRect is where the pop-up sits on screen.
func (a *App) menuRect() (x, y, w, h int) {
	return a.centered(min(max(a.width*3/5, 48), 96), max(a.height*2/3, 12))
}

// menuRows is how many entries the pop-up shows.
func (a *App) menuRows() int {
	_, _, _, h := a.menuRect()
	return popupRows(h)
}

// menuView draws the pop-up: a titled box of entries with a footer showing
// the filter or key hints.
func (a *App) menuView() []string {
	m, th := a.menu, a.theme
	_, _, w, h := a.menuRect()
	inner := max(w-2, 1)
	rows := a.menuRows()

	title := tildePath(m.dir)
	if over := ansi.StringWidth(title) - (inner - 4); over > 0 {
		title = ansi.TruncateLeft(title, over+1, "…")
	}
	lines := make([]string, 0, rows)

	open := map[string]bool{}
	for _, p := range a.allPanes() {
		open[p.doc.Path] = true
	}
	// With nothing but ../ listed, the row after it explains why.
	empty := len(m.shown) == 0 || len(m.shown) == 1 && m.entries[m.shown[0]].parent
	for r := range rows {
		i := m.offset + r
		line := ""
		switch {
		case i < len(m.shown):
			line = a.menuEntry(m.entries[m.shown[i]], i == m.cursor, open[filepath.Join(m.dir, m.entries[m.shown[i]].name)], inner)
		case i == len(m.shown) && empty && m.err != "":
			line = th.menuErr.Render(" " + m.err)
		case i == len(m.shown) && empty && m.filter.Value() != "":
			line = th.dim.Render(" (no names match)")
		case i == len(m.shown) && empty:
			line = th.dim.Render(" (nothing here · . shows hidden and non-markdown files)")
		}
		lines = append(lines, line)
	}

	right := th.dim.Render(" " + m.count(m.cursor) + " ")
	var left string
	if m.filtering || m.filter.Value() != "" {
		left = th.barPrompt.Render(" / ") + m.filter.View()
	} else if m.err != "" && !empty {
		left = th.menuErr.Render(" " + m.err)
	} else {
		all := "all"
		if a.menuAll {
			all = "md only"
		}
		h := func(name string) string { return a.hint(ctxMenu, name) }
		left = th.dim.Render(" " + h("filter") + " filter · " + h("toggle_all") + " " + all + " · " + h("parent") + " up · " +
			h("open_here") + " here · " + h("open_vsplit") + "/" + h("open_hsplit") + " split · " + h("close") + " close")
	}
	return drawBox(th, " "+title+" ", lines, left, right, w, h)
}

// count is the footer's position: entry i+1 of how many are shown.
func (m *Menu) count(i int) string {
	n := strconv.Itoa(len(m.shown))
	if len(m.shown) == 0 {
		return n
	}
	return strconv.Itoa(i+1) + "/" + n
}

// filterWidth is the room the filter input has in the footer, beside the
// widest count it can show.
func (a *App) filterWidth() int {
	_, _, w, _ := a.menuRect()
	widest := " " + a.menu.count(len(a.menu.shown)-1) + " "
	return max(w-2-ansi.StringWidth(widest)-4, 1)
}

func (a *App) menuEntry(e entry, cursor, open bool, width int) string {
	th := a.theme
	mark := "  "
	if open {
		mark = "• "
	}
	name := e.name
	if e.dir {
		name += "/"
	}
	text := " " + mark + name
	if cursor {
		text = ansi.Truncate(text, width, "…")
		return th.tocCursor.Render(text + strings.Repeat(" ", max(width-ansi.StringWidth(text), 0)))
	}
	style := th.menuFile
	switch {
	case e.hidden || !e.dir && !e.md:
		style = th.dim
	case e.dir:
		style = th.menuDir
	}
	return th.menuOpen.Render(" "+mark) + style.Render(ansi.Truncate(name, max(width-3, 1), "…"))
}

// menuMouse handles the mouse while the menu is open: the wheel scrolls it,
// a click on an entry opens it and a click outside closes the menu.
func (a *App) menuMouse(msg tea.MouseMsg) tea.Cmd {
	x, y, w, h := a.menuRect()
	step, outside := popupMouse(msg, x, y, w, h)
	a.menu.scroll(step, a.menuRows())
	if outside {
		return a.closeMenu()
	}
	m := msg.Mouse()
	if _, click := msg.(tea.MouseClickMsg); !click || m.Button != tea.MouseLeft {
		return nil
	}
	row := m.Y - y - 1
	if i := a.menu.offset + row; row >= 0 && row < a.menuRows() && i < len(a.menu.shown) {
		a.menu.selectIdx(i, a.menuRows())
		return a.menuActivate(openNewTab)
	}
	return nil
}
