package ui

import (
	"bytes"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/joe-salamy/joe-md/internal/doc"

	"github.com/charmbracelet/x/ansi"
)

// A tab's panes form a split tree, like vim's windows: leaves hold panes,
// inner nodes lay their children out side by side (vert) or stacked. Side by
// side children are separated by a │ column; stacked ones need no separator
// because every pane has its own status row once there is more than one.

const (
	paneMinWidth  = 10
	paneMinHeight = 3 // two lines of text and the pane's status row
)

type Node struct {
	parent *Node
	pane   *Pane // non-nil exactly for leaves
	vert   bool  // inner node: children side by side, not stacked
	kids   []*Node
	frac   float64 // share of the parent's size

	x, y, w, h int // last layout; x is a screen column, y a row of the body
}

func leaf(p *Pane) *Node { return &Node{pane: p, frac: 1} }

// leaves returns the panes of n in order: left to right, top to bottom.
func (n *Node) leaves() []*Node {
	if n.pane != nil {
		return []*Node{n}
	}
	var out []*Node
	for _, k := range n.kids {
		out = append(out, k.leaves()...)
	}
	return out
}

func (n *Node) index() int {
	for i, k := range n.parent.kids {
		if k == n {
			return i
		}
	}
	return -1
}

// size is n's extent along the parent's direction.
func (n *Node) size(vert bool) int {
	if vert {
		return n.w
	}
	return n.h
}

// minSize is the smallest extent n fits in along a direction.
func (n *Node) minSize(vert bool) int {
	if n.pane != nil {
		if vert {
			return paneMinWidth
		}
		return paneMinHeight
	}
	s := 0
	for _, k := range n.kids {
		if n.vert == vert {
			s += k.minSize(vert)
		} else {
			s = max(s, k.minSize(vert))
		}
	}
	if n.vert && vert {
		s += len(n.kids) - 1 // separators
	}
	return s
}

// place lays n out in a rectangle, sharing it among the children by frac.
func (n *Node) place(x, y, w, h int) {
	n.x, n.y, n.w, n.h = x, y, w, h
	if n.pane != nil {
		return
	}
	total, sep := h, 0
	if n.vert {
		total, sep = w, 1
	}
	avail := total - sep*(len(n.kids)-1)
	pos, cum := 0, 0.0
	for i, k := range n.kids {
		cum += k.frac
		end := int(math.Round(cum * float64(avail)))
		if i == len(n.kids)-1 {
			end = avail
		}
		size := max(end-pos, 1)
		if n.vert {
			k.place(x+pos+i*sep, y, size, h)
		} else {
			k.place(x, y+pos, w, size)
		}
		pos += size
	}
}

// setFracs recomputes the children's shares from cell sizes.
func (n *Node) setFracs(sizes []int) {
	sum := 0
	for _, s := range sizes {
		sum += s
	}
	for i, k := range n.kids {
		k.frac = float64(sizes[i]) / float64(max(sum, 1))
	}
}

// moveBorder puts the border after child i so that child i is size cells,
// as far as both children's minimum sizes allow.
func (n *Node) moveBorder(i, size int) {
	a, b := n.kids[i], n.kids[i+1]
	total := a.size(n.vert) + b.size(n.vert)
	lo, hi := a.minSize(n.vert), total-b.minSize(n.vert)
	if lo > hi {
		return
	}
	sizes := make([]int, len(n.kids))
	for j, k := range n.kids {
		sizes[j] = k.size(n.vert)
	}
	sizes[i] = max(lo, min(size, hi))
	sizes[i+1] = total - sizes[i]
	n.setFracs(sizes)
}

// equalize gives every child of every node the same share.
func (n *Node) equalize() {
	for _, k := range n.kids {
		k.frac = 1 / float64(len(n.kids))
		k.equalize()
	}
}

// replace puts m where n is in the tree.
func (n *Node) replace(m *Node) {
	m.parent, m.frac = n.parent, n.frac
	if n.parent != nil {
		n.parent.kids[n.index()] = m
	}
}

// merge dissolves inner node n into its parent when both lay their children
// out the same way, which a split tree never does.
func (n *Node) merge() {
	gp := n.parent
	if gp == nil || n.pane != nil || gp.vert != n.vert {
		return
	}
	j := n.index()
	for _, k := range n.kids {
		k.parent, k.frac = gp, k.frac*n.frac
	}
	gp.kids = slices.Concat(gp.kids[:j], n.kids, gp.kids[j+1:])
}

// Tab is one tab page: a split tree of panes, one of which has focus.
// Search results, history and the bar/sidebar toggles are shared by all tabs.
type Tab struct {
	root  *Node
	focus *Node // a leaf
}

func newTab(p *Pane) *Tab {
	n := leaf(p)
	return &Tab{root: n, focus: n}
}

func (t *Tab) leaves() []*Node { return t.root.leaves() }

// multi reports whether the tab has more than one pane, which is when each
// pane gets a status row.
func (t *Tab) multi() bool { return t.root.pane == nil }

// leafOf returns the leaf showing path, preferring the focused one, or nil.
func (t *Tab) leafOf(path string) *Node {
	if samePath(t.focus.pane.doc.Path, path) {
		return t.focus
	}
	for _, l := range t.leaves() {
		if samePath(l.pane.doc.Path, path) {
			return l
		}
	}
	return nil
}

// split puts p next to the focused pane, right of it (vert) or below, and
// focuses it.
func (t *Tab) split(p *Pane, vert bool) {
	n := leaf(p)
	t.insert(n, t.focus, vert, false, 0.5)
	t.focus = n
}

// insert puts leaf n beside node m (vert) or above or below it, before or
// after it, giving n share of m's space. Like vim, inserting in a node's own
// direction adds a sibling; otherwise m becomes a new node holding both.
func (t *Tab) insert(n, m *Node, vert, before bool, share float64) {
	share = max(0.1, min(share, 0.9))
	var in *Node
	if par := m.parent; par != nil && par.vert == vert {
		in = par
		n.frac = m.frac * share
		m.frac -= n.frac
	} else {
		in = &Node{vert: vert}
		m.replace(in)
		if m == t.root {
			t.root = in
		}
		in.kids = []*Node{m}
		m.parent, m.frac = in, 1-share
		n.frac = share
	}
	n.parent = in
	i := m.index()
	if !before {
		i++
	}
	in.kids = slices.Insert(in.kids, i, n)
}

// has reports whether node n is in the tab's tree.
func (t *Tab) has(n *Node) bool {
	for c := n; c != t.root; c = c.parent {
		if c == nil || c.parent == nil || c.index() < 0 {
			return false
		}
	}
	return true
}

// canSplit reports whether the focused pane has room to be split.
func (t *Tab) canSplit(vert bool) bool {
	f := t.focus
	if vert {
		return f.w >= 2*paneMinWidth+1
	}
	h := f.h
	if !t.multi() {
		h -= 2 // both halves will need a status row
	}
	return h >= 2*paneMinHeight
}

// close removes the focused pane, giving its space to a neighbour, which
// gets focus. The tab must have more than one pane.
func (t *Tab) close() {
	f := t.focus
	par := f.parent
	i := f.index()
	par.kids = append(par.kids[:i], par.kids[i+1:]...)
	next := max(i-1, 0) // like vim, the pane before takes the space
	par.kids[next].frac += f.frac
	heir := par.kids[next]
	if len(par.kids) == 1 {
		only := par.kids[0]
		par.replace(only)
		if par == t.root {
			t.root = only
		}
		only.merge()
	}
	ls := heir.leaves()
	if next < i {
		t.focus = ls[len(ls)-1] // the side nearest the closed pane
	} else {
		t.focus = ls[0]
	}
}

// only is ctrl+w o: close every pane but the focused one.
func (t *Tab) only() {
	t.root = t.focus
	t.focus.parent, t.focus.frac = nil, 1
}

// resize grows the focused pane by d cells, side to side (vert) or up and
// down, taking the space from the next pane over (the previous if last).
func (t *Tab) resize(vert bool, d int) bool {
	for c := t.focus; c.parent != nil; c = c.parent {
		par := c.parent
		if par.vert != vert {
			continue
		}
		i := c.index()
		if i+1 < len(par.kids) {
			par.moveBorder(i, c.size(vert)+d)
		} else {
			par.moveBorder(i-1, par.kids[i-1].size(vert)-d)
		}
		return true
	}
	return false
}

// neighbor is the pane next to the focused one in a direction (h j k l), or
// nil. Of several, it picks the one level with the focused pane's top-left.
func (t *Tab) neighbor(dir string) *Node {
	f := t.focus
	overlap := func(a, al, b, bl int) bool { return a < b+bl && b < a+al }
	var best *Node
	for _, l := range t.leaves() {
		var ok, level bool
		switch dir {
		case "h":
			ok, level = l.x+l.w+1 == f.x && overlap(l.y, l.h, f.y, f.h), l.y <= f.y && f.y < l.y+l.h
		case "l":
			ok, level = f.x+f.w+1 == l.x && overlap(l.y, l.h, f.y, f.h), l.y <= f.y && f.y < l.y+l.h
		case "k":
			ok, level = l.y+l.h == f.y && overlap(l.x, l.w, f.x, f.w), l.x <= f.x && f.x < l.x+l.w
		case "j":
			ok, level = f.y+f.h == l.y && overlap(l.x, l.w, f.x, f.w), l.x <= f.x && f.x < l.x+l.w
		}
		if ok && (best == nil || level) {
			best = l
		}
	}
	return best
}

// leafAt returns the pane at screen column x, body row y, or nil.
func (t *Tab) leafAt(x, y int) *Node {
	for _, l := range t.leaves() {
		if x >= l.x && x < l.x+l.w && y >= l.y && y < l.y+l.h {
			return l
		}
	}
	return nil
}

// border is the draggable line after child i of an inner node.
type border struct {
	n *Node
	i int
}

// borderAt finds the border at x, y: a │ separator column, or the status row
// of a pane stacked above another.
func (n *Node) borderAt(x, y int) *border {
	if n.pane != nil || x < n.x || x >= n.x+n.w || y < n.y || y >= n.y+n.h {
		return nil
	}
	for i, k := range n.kids[:len(n.kids)-1] {
		if n.vert && x == k.x+k.w || !n.vert && y == k.y+k.h-1 {
			return &border{n, i}
		}
	}
	for _, k := range n.kids {
		if b := k.borderAt(x, y); b != nil {
			return b
		}
	}
	return nil
}

// drag moves the border to screen column x or body row y.
func (b *border) drag(x, y int) {
	a := b.n.kids[b.i]
	if b.n.vert {
		b.n.moveBorder(b.i, x-a.x)
	} else {
		b.n.moveBorder(b.i, y-a.y+1)
	}
}

// renderNode draws n in exactly n.h lines of n.w cells.
func (a *App) renderNode(t *Tab, n *Node) []string {
	var out []string
	switch {
	case n.pane != nil:
		out = n.pane.Render(a.theme)
		if t.multi() {
			out = append(out, a.paneStatus(n, n == t.focus))
		}
	case n.vert:
		cols := make([][]string, len(n.kids))
		for i, k := range n.kids {
			cols[i] = a.renderNode(t, k)
		}
		sep := a.theme.separator.Render("│")
		out = make([]string, n.h)
		for y := range out {
			var sb strings.Builder
			for i, c := range cols {
				if i > 0 {
					sb.WriteString(sep)
				}
				l := strings.Repeat(" ", n.kids[i].w)
				if y < len(c) {
					l = c[y]
				}
				sb.WriteString(l + "\x1b[0m")
			}
			out[y] = sb.String()
		}
	default:
		for _, k := range n.kids {
			out = append(out, a.renderNode(t, k)...)
		}
	}
	for len(out) < n.h {
		out = append(out, "")
	}
	out = out[:n.h]
	for y := range out {
		out[y] = fit(out[y], n.w)
	}
	return out
}

// paneStatus is a pane's own status row, shown when a tab has several: the
// file, the scrollbind mark and the position, highlighted on the focused pane.
func (a *App) paneStatus(n *Node, focused bool) string {
	p := n.pane
	style := a.theme.tabInactive
	if focused {
		style = a.theme.tabActive
	}
	left := " " + p.doc.Name
	if p.bind {
		left += " [bind]"
	}
	right := " " + strconv.Itoa(p.TopSource()+1) + "/" + strconv.Itoa(p.doc.Lines) + "  " + p.Percent() + " "
	gap := n.w - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return style.Render(ansi.Truncate(left, n.w, ""))
	}
	return style.Render(left + strings.Repeat(" ", gap) + right)
}

// scrollSnap records where the current tab's bound panes are, so scrollBind
// can tell which one the user scrolled.
type scrollSnap struct {
	tab   *Tab
	panes []*Pane
	pos   []int           // offsets
	src   []int           // source lines at the top
	view  []*doc.Rendered // a re-rendered pane moved without being scrolled
}

func (a *App) scrollSnapshot() scrollSnap {
	var s scrollSnap
	if t := a.tab(); t != nil && t.multi() {
		s.tab = t
		for _, l := range t.leaves() {
			if l.pane.bind {
				s.panes = append(s.panes, l.pane)
				s.pos = append(s.pos, l.pane.offset)
				s.src = append(s.src, l.pane.TopSource())
				s.view = append(s.view, l.pane.view)
			}
		}
	}
	return s
}

// scrollBind scrolls the bound panes along with the one that moved (the
// focused one if several did), by the same number of source lines, as vim
// does. Panes showing the same rendering (one file, one width) move by the
// same number of rendered lines instead, which keeps them exactly in step.
func (a *App) scrollBind(s scrollSnap) {
	if len(s.panes) < 2 || a.tab() != s.tab {
		return
	}
	moved := -1
	for i, p := range s.panes {
		if p.bind && p.view == s.view[i] && p.offset != s.pos[i] && (moved < 0 || p == a.pane) {
			moved = i
		}
	}
	if moved < 0 {
		return
	}
	m := s.panes[moved]
	d, dsrc := m.offset-s.pos[moved], m.TopSource()-s.src[moved]
	for i, p := range s.panes {
		if i == moved || !p.bind || p.offset != s.pos[i] || !a.livePane(p) {
			continue
		}
		if p.wrap == m.wrap && (p.doc == m.doc || bytes.Equal(p.doc.Source, m.doc.Source)) {
			p.ScrollBy(d)
		} else if dsrc != 0 {
			p.ScrollToSource(s.src[i] + dsrc)
		}
	}
}
