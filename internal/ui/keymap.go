package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/joe-salamy/joe-md/internal/config"

	tea "charm.land/bubbletea/v2"
)

// Keymap is the effective bindings: the defaults with the settings file's
// changes.
type Keymap struct {
	keys     map[*action][]string
	byName   map[keyCtx]map[string]*action // context -> action name -> action
	bind     map[keyCtx]map[string]*action // context -> key sequence -> action
	prefixes map[keyCtx]map[string]bool    // context -> incomplete sequences
}

// singleKeyContexts are the typing contexts, where a sequence can't start.
var singleKeyContexts = map[keyCtx]bool{ctxSearch: true, ctxFilter: true}

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
	km := &Keymap{keys: map[*action][]string{}, byName: map[keyCtx]map[string]*action{}}
	byName := km.byName
	for _, x := range actions {
		if byName[x.ctx] == nil {
			byName[x.ctx] = map[string]*action{}
		}
		byName[x.ctx][x.name] = x
		km.keys[x] = x.keys
	}
	for _, name := range sortedKeys(changes) {
		ctx := keyCtx(name)
		names, ok := byName[ctx]
		if !ok {
			return nil, fmt.Errorf("keys.%s: unknown context (have %s)", ctx, strings.Join(contextNames(), ", "))
		}
		for _, name := range sortedKeys(changes[name]) {
			x, ok := names[name]
			if !ok {
				return nil, fmt.Errorf("keys.%s.%s: unknown action", ctx, name)
			}
			var ks []string
			for _, k := range changes[string(ctx)][name] {
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
					km.keys[other] = slices.DeleteFunc(slices.Clone(oks), func(k string) bool { return slices.Contains(ks, k) })
				}
			}
			km.keys[x] = ks
		}
	}

	km.bind = map[keyCtx]map[string]*action{}
	km.prefixes = map[keyCtx]map[string]bool{}
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

// sortedKeys returns m's keys in order, for stable error messages.
func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

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

// lookup finds what key sequence seq does in the contexts, in order: an
// action, or more keys to come.
func (km *Keymap) lookup(seq string, ctxs ...keyCtx) (x *action, prefix bool) {
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
func (km *Keymap) Keys(ctx keyCtx, name string) []string {
	return km.keys[km.byName[ctx][name]]
}

// contextNames lists the contexts in order, for error messages.
func contextNames() []string {
	out := make([]string, len(contexts))
	for i, c := range contexts {
		out[i] = string(c)
	}
	return out
}

// Bindings lists every action with its default keys, for -dump-config.
func Bindings() []config.Binding {
	var out []config.Binding
	for _, ctx := range contexts {
		for _, x := range actions {
			if x.ctx == ctx {
				out = append(out, config.Binding{Context: string(x.ctx), Action: x.name, Keys: x.keys, Desc: x.desc})
			}
		}
	}
	return out
}

// dispatch runs key k in the contexts, tracking sequences in *pending. It
// reports whether the key was used, either as an action or as the start of
// a sequence.
func (a *App) dispatch(pending *string, k string, count int, ctxs ...keyCtx) (tea.Cmd, bool) {
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
	if p := a.keymap.Keys(ctxNormal, "window"); len(p) > 0 {
		return displayKey(p[0])
	}
	return "window"
}

// hint is an action's first key, for the hints in the bars.
func (a *App) hint(ctx keyCtx, name string) string {
	if k := a.keymap.Keys(ctx, name); len(k) > 0 {
		return displayKey(k[0])
	}
	return "unbound"
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
