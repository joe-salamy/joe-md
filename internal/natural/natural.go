// Package natural orders names the way people read them: ignoring case, with
// runs of digits compared by value.
package natural

import (
	"cmp"
	"path/filepath"
	"strings"
)

// Compare orders names case-insensitively, comparing runs of digits by value
// so "2. Foo" comes before "10. Bar". Ties fall back to the raw names. It
// returns -1, 0 or +1, like cmp.Compare.
func Compare(a, b string) int {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(la) && j < len(lb) {
		if isDigit(la[i]) && isDigit(lb[j]) {
			si, sj := i, j
			for i < len(la) && isDigit(la[i]) {
				i++
			}
			for j < len(lb) && isDigit(lb[j]) {
				j++
			}
			na := strings.TrimLeft(la[si:i], "0")
			nb := strings.TrimLeft(lb[sj:j], "0")
			if c := cmp.Or(cmp.Compare(len(na), len(nb)), strings.Compare(na, nb)); c != 0 {
				return c
			}
			continue
		}
		if la[i] != lb[j] {
			return cmp.Compare(la[i], lb[j])
		}
		i++
		j++
	}
	return cmp.Or(cmp.Compare(len(la)-i, len(lb)-j), strings.Compare(a, b))
}

// PathCompare orders file paths as the menu lists them: one directory level
// at a time, directories before files, each level's names by Compare.
func PathCompare(a, b string) int {
	pa := strings.Split(filepath.ToSlash(a), "/")
	pb := strings.Split(filepath.ToSlash(b), "/")
	for k := 0; k < len(pa) && k < len(pb); k++ {
		if pa[k] == pb[k] {
			continue
		}
		// The path that goes on is a directory at this level; the other is a file.
		aDir, bDir := k < len(pa)-1, k < len(pb)-1
		if aDir != bDir {
			return DirsFirst(aDir)
		}
		return Compare(pa[k], pb[k])
	}
	return cmp.Compare(len(pa), len(pb))
}

// DirsFirst is the order of a directory and a file, from whether the first
// of the two is the directory: -1 if it is, +1 if not.
func DirsFirst(firstIsDir bool) int {
	if firstIsDir {
		return -1
	}
	return 1
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
