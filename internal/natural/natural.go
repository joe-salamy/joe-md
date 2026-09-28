// Package natural orders names the way people read them: ignoring case, with
// runs of digits compared by value.
package natural

import (
	"path/filepath"
	"strings"
)

// Less orders names case-insensitively, comparing runs of digits by value so
// "2. Foo" comes before "10. Bar". Ties fall back to the raw names.
func Less(a, b string) bool {
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
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if la[i] != lb[j] {
			return la[i] < lb[j]
		}
		i++
		j++
	}
	if len(la)-i != len(lb)-j {
		return len(la)-i < len(lb)-j
	}
	return a < b
}

// PathLess orders file paths as the menu lists them: one directory level at a
// time, directories before files, each level's names by Less.
func PathLess(a, b string) bool {
	pa := strings.Split(filepath.ToSlash(a), "/")
	pb := strings.Split(filepath.ToSlash(b), "/")
	for k := 0; k < len(pa) && k < len(pb); k++ {
		if pa[k] == pb[k] {
			continue
		}
		// The path that goes on is a directory at this level; the other is a file.
		aDir, bDir := k < len(pa)-1, k < len(pb)-1
		if aDir != bDir {
			return aDir
		}
		return Less(pa[k], pb[k])
	}
	return len(pa) < len(pb)
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
