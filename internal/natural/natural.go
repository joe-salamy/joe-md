// Package natural orders names the way people read them: ignoring case, with
// runs of digits compared by value.
package natural

import (
	"cmp"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Compare orders names case-insensitively, comparing runs of digits by value
// so "2. Foo" comes before "10. Bar". Ties fall back to the raw names. It
// returns -1, 0 or +1, like cmp.Compare.
//
// It lowercases as it goes, a rune at a time, instead of lowering both names
// up front: sorting is comparison-heavy, and a copy per comparison is a
// needless allocation for every pair of names in a large directory.
func Compare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			if c := compareDigits(a[si:i], b[sj:j]); c != 0 {
				return c
			}
			continue
		}
		if ca, cb := a[i], b[j]; ca < utf8.RuneSelf && cb < utf8.RuneSelf {
			if la, lb := lowerASCII(ca), lowerASCII(cb); la != lb {
				return cmp.Compare(la, lb)
			}
			i, j = i+1, j+1
			continue
		}
		ra, na := utf8.DecodeRuneInString(a[i:])
		rb, nb := utf8.DecodeRuneInString(b[j:])
		if la, lb := unicode.ToLower(ra), unicode.ToLower(rb); la != lb {
			return cmp.Compare(la, lb)
		}
		i, j = i+na, j+nb
	}
	return cmp.Or(cmp.Compare(len(a)-i, len(b)-j), strings.Compare(a, b))
}

// compareDigits compares two runs of ASCII digits by value: leading zeros
// don't count, so the longer significant run is the larger number, and equal
// lengths compare digit by digit.
func compareDigits(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	return cmp.Or(cmp.Compare(len(a), len(b)), strings.Compare(a, b))
}

// PathCompare orders file paths as the menu lists them: one directory level
// at a time, directories before files, each level's names by Compare. It
// walks the components in place, so comparing two paths in a sort allocates
// nothing.
func PathCompare(a, b string) int {
	a, b = filepath.ToSlash(a), filepath.ToSlash(b)
	for {
		ca, ra, aMore := cutComponent(a)
		cb, rb, bMore := cutComponent(b)
		if ca != cb {
			// The path that goes on is a directory at this level; the other is a file.
			if aMore != bMore {
				return DirsFirst(aMore)
			}
			return Compare(ca, cb)
		}
		switch {
		case !aMore && !bMore:
			return 0
		case !aMore:
			return -1 // one path ends here: the shorter comes first
		case !bMore:
			return 1
		}
		a, b = ra, rb
	}
}

// cutComponent splits the first component off slash-separated path p,
// reporting whether more components follow it.
func cutComponent(p string) (name, rest string, more bool) {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i], p[i+1:], true
	}
	return p, "", false
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

// lowerASCII lowercases an ASCII byte.
func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
