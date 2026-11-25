package natural

import (
	"slices"
	"testing"
)

func TestCompare(t *testing.T) {
	names := []string{"10. Ten", "2. Two", "b", "1. One", "A", "01. Zero-padded", "file10", "file9"}
	slices.SortFunc(names, Compare)
	want := []string{"1. One", "01. Zero-padded", "2. Two", "10. Ten", "A", "b", "file9", "file10"}
	if !slices.Equal(names, want) {
		t.Errorf("got %q, want %q", names, want)
	}
}

func TestPathCompare(t *testing.T) {
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
	slices.SortFunc(paths, PathCompare)
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

// Both comparisons run inside sort comparators, which call them thousands of
// times for a large directory or result list, so neither may allocate: a copy
// of each name per comparison is a copy per comparison of the sort.
func TestCompareDoesNotAllocate(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"file9", "file10"},
		{"2. Two", "10. Ten"},
		{"École", "école"},
		{"/r/b/10/x.md", "/r/b/2/x.md"},
		{"/notes/file9.md", "/notes/file10.md"},
	} {
		if n := testing.AllocsPerRun(100, func() { Compare(tc.a, tc.b) }); n != 0 {
			t.Errorf("Compare(%q, %q) allocates %v times", tc.a, tc.b, n)
		}
		if n := testing.AllocsPerRun(100, func() { PathCompare(tc.a, tc.b) }); n != 0 {
			t.Errorf("PathCompare(%q, %q) allocates %v times", tc.a, tc.b, n)
		}
	}
}
