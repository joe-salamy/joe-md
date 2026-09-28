package natural

import (
	"slices"
	"sort"
	"testing"
)

func TestLess(t *testing.T) {
	names := []string{"10. Ten", "2. Two", "b", "1. One", "A", "01. Zero-padded", "file10", "file9"}
	sort.Slice(names, func(i, j int) bool { return Less(names[i], names[j]) })
	want := []string{"1. One", "01. Zero-padded", "2. Two", "10. Ten", "A", "b", "file9", "file10"}
	if !slices.Equal(names, want) {
		t.Errorf("got %q, want %q", names, want)
	}
}

func TestPathLess(t *testing.T) {
	paths := []string{
		"/r/notes-old.md",
		"/r/Zeta.md",
		"/r/notes/a.md",
		"/r/10.md",
		"/r/alpha.md",
		"/r/2.md",
		"/r/b/10/x.md",
		"/r/b/2/x.md",
		"/r/b/c.md",
	}
	sort.Slice(paths, func(i, j int) bool { return PathLess(paths[i], paths[j]) })
	want := []string{
		"/r/b/2/x.md",
		"/r/b/10/x.md",
		"/r/b/c.md",
		"/r/notes/a.md",
		"/r/2.md",
		"/r/10.md",
		"/r/alpha.md",
		"/r/notes-old.md",
		"/r/Zeta.md",
	}
	if !slices.Equal(paths, want) {
		t.Errorf("got %q, want %q", paths, want)
	}
}
