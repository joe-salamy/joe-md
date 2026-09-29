package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The help overlay and KEYS.md both list the action table by group; this is
// the formatting they share, and KEYS.md itself.

// display is how the help and KEYS.md show an action's keys: window keys
// get the prefix in front.
func (km *Keymap) display(x *action) []string {
	prefix := ""
	if x.ctx == ctxWindow {
		if p := km.Keys(ctxNormal, "window"); len(p) > 0 {
			prefix = displayKey(p[0]) + " "
		}
	}
	out := make([]string, 0, len(km.keys[x]))
	for _, k := range collapseDigits(km.keys[x]) {
		out = append(out, prefix+displayKey(k))
	}
	return out
}

// collapseDigits shortens a run of keys that differ only in a last digit
// counting up by one, three or more long: alt+1 … alt+9 become "alt+1…9".
func collapseDigits(ks []string) []string {
	digit := func(k string) (string, byte, bool) {
		if d := k[len(k)-1]; len(k) > 1 && d >= '0' && d <= '9' {
			return k[:len(k)-1], d, true
		}
		return "", 0, false
	}
	var out []string
	for i := 0; i < len(ks); {
		j := i + 1
		if p, d, ok := digit(ks[i]); ok {
			for j < len(ks) {
				q, e, ok := digit(ks[j])
				if !ok || q != p || e != d+byte(j-i) {
					break
				}
				j++
			}
			if j-i >= 3 {
				out = append(out, ks[i]+"…"+string(ks[j-1][len(ks[j-1])-1]))
				i = j
				continue
			}
		}
		out = append(out, ks[i])
		i++
	}
	return out
}

// helpRow is one line of the help and KEYS.md: one action, several merged
// ones (see action.row), or a fixed key.
type helpRow struct {
	acts []*action  // none for a fixed key
	keys [][]string // alternatives, shown split by " / "; one when zipped
	desc string
}

// rows lists group g's lines. Merged actions with as many keys each are
// zipped: j down and k up become j/k down/up.
func (km *Keymap) rows(g string) []helpRow {
	var out []helpRow
	for _, x := range actions {
		if x.group != g {
			continue
		}
		if n := len(out); x.row != "" && n > 0 {
			if prev := out[n-1].acts; prev[len(prev)-1].row == x.row && prev[0].ctx == x.ctx {
				out[n-1].acts = append(prev, x)
				continue
			}
		}
		out = append(out, helpRow{acts: []*action{x}})
	}
	for i := range out {
		r := &out[i]
		r.desc = r.acts[0].desc
		if len(r.acts) > 1 {
			r.desc = r.acts[0].row
		}
		var ks [][]string
		for _, x := range r.acts {
			k := km.display(x)
			if len(k) == 0 {
				k = []string{"(unbound)"}
			}
			ks = append(ks, k)
		}
		r.keys = zipKeys(ks)
	}
	for _, f := range fixedKeys[g] {
		out = append(out, helpRow{keys: [][]string{{f.keys}}, desc: f.desc})
	}
	return out
}

// zipKeys pairs up the keys of merged actions when each has as many:
// [ctrl+d pgdown] and [ctrl+u pgup] become [ctrl+d/u pgdown/pgup].
// Otherwise they stay apart.
func zipKeys(ks [][]string) [][]string {
	if len(ks) == 1 {
		return ks
	}
	for _, k := range ks[1:] {
		if len(k) != len(ks[0]) {
			return ks
		}
	}
	out := make([]string, len(ks[0]))
	for i := range out {
		col := make([]string, len(ks))
		for j := range ks {
			col[j] = ks[j][i]
		}
		out[i] = zipKey(col)
	}
	return [][]string{out}
}

// zipKey joins keys with "/" after any shared modifier or prefix key:
// ctrl+w h and ctrl+w j become ctrl+w h/j.
func zipKey(ks []string) string {
	p := ks[0]
	for _, k := range ks[1:] {
		for !strings.HasPrefix(k, p) {
			p = p[:len(p)-1]
		}
	}
	p = p[:strings.LastIndexAny(p, " +")+1]
	rest := make([]string, len(ks))
	for i, k := range ks {
		if rest[i] = k[len(p):]; rest[i] == "" {
			return strings.Join(ks, "/")
		}
	}
	return p + strings.Join(rest, "/")
}

// Markdown is KEYS.md: every group's actions in a table, with the name the
// settings file uses for each.
func (km *Keymap) Markdown() string {
	var b strings.Builder
	b.WriteString(`# joe-md keys

<!-- Generated from the action table in internal/ui/actions.go; don't edit by hand.
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
		for _, r := range km.rows(g) {
			var alts []string
			for _, ks := range r.keys {
				var q []string
				for _, k := range ks {
					q = append(q, "`"+strings.ReplaceAll(k, "|", `\|`)+"`")
				}
				alts = append(alts, strings.Join(q, " "))
			}
			name := "(fixed)"
			if len(r.acts) > 0 {
				names := []string{"`" + string(r.acts[0].ctx) + "." + r.acts[0].name + "`"}
				for _, x := range r.acts[1:] {
					names = append(names, "`"+x.name+"`")
				}
				name = strings.Join(names, " / ")
			}
			rows = append(rows, row{strings.Join(alts, " / "), r.desc, name})
		}
		for _, r := range rows {
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
