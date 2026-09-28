package ui

import (
	"slices"
	"sort"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	names := []string{"10. Ten", "2. Two", "b", "1. One", "A", "01. Zero-padded", "file10", "file9"}
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	want := []string{"1. One", "01. Zero-padded", "2. Two", "10. Ten", "A", "b", "file9", "file10"}
	if !slices.Equal(names, want) {
		t.Errorf("got %q, want %q", names, want)
	}
}
