// Package doc parses markdown into top-level blocks with exact source line
// ranges, renders them with glamour, and maps between source and rendered
// line numbers. Plain-text files are one block per line, shown as written.
package doc

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Block is one top-level markdown block. Lines are 0-based.
type Block struct {
	SrcStart int    // first source line of the block
	SrcEnd   int    // one past the last non-blank line of the block
	Next     int    // first line of the following block (or line count)
	Text     string // source text rendered for this block
}

// Heading is a top-level markdown heading, used for the table of contents.
type Heading struct {
	Level int
	Text  string
	Block int // index into Doc.Blocks
}

type Doc struct {
	Path     string
	Name     string
	Source   []byte
	Lines    int // number of source lines
	Blocks   []Block
	Headings []Heading
	renders  []string // the markdown handed to glamour for each block

	// MinHeadingLevel is the smallest heading level, or 6 when there are
	// none: the sidebar indents relative to it.
	MinHeadingLevel int
	hash            uint64 // content fingerprint, for scrollbind

	// plain is a plain-text file: one block per line, shown as written
	// instead of rendered as markdown.
	plain bool
}

// SameContent reports whether d and o hold the same source text. It is for
// panes that were split from one file but hold separate loads; a nil Doc
// never matches.
func (d *Doc) SameContent(o *Doc) bool {
	if d == o {
		return true
	}
	if d == nil || o == nil {
		return false
	}
	if d.hash != 0 && o.hash != 0 {
		return d.hash == o.hash
	}
	return bytes.Equal(d.Source, o.Source)
}

// ErrBinary is Load's error for a file that isn't text.
var ErrBinary = errors.New("not a text file")

// ErrNotFile is Load's error for a path that isn't a regular file.
var ErrNotFile = errors.New("not a regular file")

// IsPlainText reports whether the file name is a plain-text file, which is
// shown as written rather than rendered as markdown: its line breaks, and
// characters like * and #, mean nothing to markdown.
func IsPlainText(name string) bool { return strings.EqualFold(filepath.Ext(name), ".txt") }

// Load reads and parses the file at path, as markdown or, for a plain-text
// file, as plain text. The doc's path is absolute, so file identity, display
// and editing never depend on how the file was named on its way in.
func Load(path string) (*Doc, error) {
	// Only regular files are read: opening a FIFO waits for a writer that may
	// never come, and a device or socket has no end.
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is %w", filepath.Base(path), ErrNotFile)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isBinary(src) {
		return nil, fmt.Errorf("%s is %w", filepath.Base(path), ErrBinary)
	}
	var d *Doc
	if IsPlainText(path) {
		d = ParsePlain(src)
	} else {
		d = Parse(src)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	d.Path = path
	d.Name = filepath.Base(path)
	return d, nil
}

// isBinary guesses like git does: a NUL byte near the start.
func isBinary(b []byte) bool {
	return bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0
}

// ParsePlain splits plain text into one block per line, so source and
// rendered lines map exactly and blank lines are kept.
func ParsePlain(orig []byte) *Doc {
	src := sanitizeControls(orig)
	lines := strings.Split(strings.TrimSuffix(string(src), "\n"), "\n")
	d := &Doc{Source: orig, Lines: len(lines), MinHeadingLevel: 6, plain: true}
	d.Blocks = make([]Block, len(lines))
	for i, l := range lines {
		d.Blocks[i] = Block{SrcStart: i, SrcEnd: i + 1, Next: i + 1, Text: strings.ReplaceAll(l, "\r", "")}
	}
	h := fnv.New64a()
	_, _ = h.Write(orig)
	d.hash = h.Sum64()
	return d
}

// md must use the same block-level extensions glamour uses so that our block
// boundaries agree with what glamour renders.
var md = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList))

// Parse splits src into top-level blocks.
func Parse(orig []byte) *Doc {
	src := sanitizeControls(blankFrontMatter(orig))
	lineStarts := []int{0} // the offset of each line
	for i, c := range src {
		if c == '\n' && i+1 < len(src) {
			lineStarts = append(lineStarts, i+1)
		}
	}
	lineOf := func(pos int) int {
		return sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > pos }) - 1
	}

	d := &Doc{Source: orig, Lines: len(lineStarts)} // Source keeps the front matter
	root := md.Parser().Parse(text.NewReader(src))

	// Collect block start lines. Link reference definitions render to
	// nothing, so they are not blocks of their own; they are appended to the
	// blocks that use them instead so reference-style links still resolve.
	type start struct {
		line int
		node ast.Node
	}
	var starts []start
	refs := map[string]string{} // normalised label -> definition text
	refLine := map[int]bool{}
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if n.Pos() < 0 {
			continue // no position; its text is absorbed by the previous block
		}
		l := lineOf(n.Pos())
		if n.Kind() == ast.KindLinkReferenceDefinition {
			end := l + 1
			if nx := n.NextSibling(); nx != nil && nx.Pos() >= 0 {
				end = lineOf(nx.Pos())
			}
			label := normLabel(string(n.(*ast.LinkReferenceDefinition).Label))
			if _, ok := refs[label]; !ok { // the first definition of a label wins
				refs[label] = linesText(src, lineStarts, l, end)
			}
			for i := l; i < end; i++ {
				refLine[i] = true
			}
			continue
		}
		if len(starts) > 0 && starts[len(starts)-1].line == l {
			continue
		}
		starts = append(starts, start{l, n})
	}

	// Pseudo-heading level for standalone bold lines: one below the deepest
	// real heading (capped at 6), or 1 when the file has no real headings.
	boldLevel := 1
	for _, s := range starts {
		if h, ok := s.node.(*ast.Heading); ok {
			boldLevel = max(boldLevel, min(h.Level+1, 6))
		}
	}

	for i, s := range starts {
		b := Block{SrcStart: s.line, Next: d.Lines}
		if i == 0 {
			b.SrcStart = 0 // leading blank lines / front matter belong to the first block
		}
		if i+1 < len(starts) {
			b.Next = starts[i+1].line
		}
		b.SrcEnd = b.Next
		for b.SrcEnd > b.SrcStart+1 && (refLine[b.SrcEnd-1] || len(bytes.TrimSpace(lineBytes(src, lineStarts, b.SrcEnd-1))) == 0) {
			b.SrcEnd--
		}
		b.Text = linesText(src, lineStarts, s.line, b.Next)
		if h, ok := s.node.(*ast.Heading); ok {
			d.Headings = append(d.Headings, Heading{
				Level: h.Level,
				Text:  strings.TrimSpace(inlineText(h, src)),
				Block: i,
			})
		} else if p, ok := s.node.(*ast.Paragraph); ok && isStandaloneBold(p, src) {
			if text := strings.TrimSpace(inlineText(p, src)); text != "" {
				d.Headings = append(d.Headings, Heading{
					Level: boldLevel,
					Text:  text,
					Block: i,
				})
			}
		}
		d.Blocks = append(d.Blocks, b)
		d.renders = append(d.renders, withRefs(b.Text, refs))
	}
	d.MinHeadingLevel = 6
	for _, h := range d.Headings {
		d.MinHeadingLevel = min(d.MinHeadingLevel, h.Level)
	}
	h := fnv.New64a()
	_, _ = h.Write(orig)
	d.hash = h.Sum64()
	return d
}

// sanitizeControls replaces terminal control characters (notably ESC, which
// starts ANSI and OSC sequences) with spaces before the source is parsed and
// rendered, so a document cannot drive the terminal. Newlines, carriage
// returns and tabs are kept: they are ordinary markdown.
func sanitizeControls(src []byte) []byte {
	first := controlAt(src)
	if first < 0 {
		return src
	}
	var b bytes.Buffer
	b.Grow(len(src))
	b.Write(src[:first])
	for i := first; i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if isTerminalControl(r) {
			b.WriteByte(' ')
		} else {
			b.Write(src[i : i+size])
		}
		i += size
	}
	return b.Bytes()
}

// controlAt returns the offset of the first terminal control character in
// src, or -1. It reads ASCII a byte at a time, which is most of a document.
func controlAt(src []byte) int {
	for i := 0; i < len(src); {
		if c := src[i]; c < utf8.RuneSelf {
			if c == 0x7f || c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
				return i
			}
			i++
			continue
		}
		r, size := utf8.DecodeRune(src[i:])
		if isTerminalControl(r) {
			return i
		}
		i += size
	}
	return -1
}

func isTerminalControl(r rune) bool {
	return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
}

// isStandaloneBold reports whether p is a single bold span and nothing else:
// one level-2 emphasis child, ignoring whitespace-only text. Some note
// generators use such lines as section headers, so they join the TOC.
func isStandaloneBold(p *ast.Paragraph, src []byte) bool {
	found := false
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok && len(bytes.TrimSpace(t.Segment.Value(src))) == 0 {
			continue
		}
		e, ok := c.(*ast.Emphasis)
		if !ok || e.Level != 2 || found {
			return false
		}
		found = true
	}
	return found
}

// renderText is the markdown handed to glamour for block i.
func (d *Doc) renderText(i int) string {
	if i < len(d.renders) {
		return d.renders[i]
	}
	return d.Blocks[i].Text
}

// withRefs appends to a block's text the link reference definitions it
// uses, unless it holds them already (the definitions after a block are in
// its text). Leaving out the rest keeps a block's text, and so its place in
// the render cache, the same when an unrelated definition changes.
//
// It scans the block's own bracket groups instead of every definition, so a
// document with a long reference list stays linear to parse.
func withRefs(text string, refs map[string]string) string {
	if len(refs) == 0 || !strings.Contains(text, "[") {
		return text
	}
	var used []string
	for i := 0; i < len(text); {
		j := strings.IndexByte(text[i:], '[')
		if j < 0 {
			break
		}
		start := i + j + 1
		k := strings.IndexByte(text[start:], ']')
		if k < 0 {
			break
		}
		if def, ok := refs[normLabel(text[start:start+k])]; ok && !strings.Contains(text, def) {
			used = append(used, strings.TrimSuffix(def, "\n"))
		}
		i = start + k + 1
	}
	if len(used) == 0 {
		return text
	}
	return text + "\n\n" + strings.Join(used, "\n") + "\n"
}

var bracketSpace = strings.NewReplacer("[ ", "[", " ]", "]")

// normLabel matches link labels as CommonMark does, near enough: ignoring
// case and runs of whitespace, including any just inside the brackets.
func normLabel(s string) string {
	return bracketSpace.Replace(strings.ToLower(strings.Join(strings.Fields(s), " ")))
}

// lineBytes returns line i of src, newline included.
func lineBytes(src []byte, lineStarts []int, i int) []byte {
	end := len(src)
	if i+1 < len(lineStarts) {
		end = lineStarts[i+1]
	}
	return src[lineStarts[i]:end]
}

// linesText returns lines [from, to) of src as one newline-terminated string,
// the form glamour is given.
func linesText(src []byte, lineStarts []int, from, to int) string {
	end := len(src)
	if to < len(lineStarts) {
		end = lineStarts[to]
	}
	text := src[lineStarts[from]:end]
	if len(text) == 0 || text[len(text)-1] != '\n' {
		return string(text) + "\n"
	}
	return string(text)
}

// yamlKey is the first line of front matter: a YAML key, or a comment.
var yamlKey = regexp.MustCompile(`^(#|[A-Za-z0-9_][\w.-]*[ \t]*:([ \t]|$))`)

// blankFrontMatter replaces a leading YAML front matter block with empty lines,
// hiding it from the renderer while keeping line numbers intact. The line
// after the opening --- must start the YAML, so a document that merely opens
// with a thematic break keeps its text.
func blankFrontMatter(src []byte) []byte {
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
		return src
	}
	pos := bytes.IndexByte(src, '\n') + 1
	line, _ := nextLine(src, pos)
	if line == nil || !yamlKey.Match(bytes.TrimRight(line, "\r\n")) && !isFence(line) {
		return src
	}
	// Scan for the closing fence rather than splitting the whole document
	// into lines: most of it is below the front matter and not looked at.
	n := 1 // the opening ---
	for pos < len(src) {
		n++
		line, next := nextLine(src, pos)
		if isFence(line) {
			out := make([]byte, 0, len(src))
			for range n {
				out = append(out, '\n')
			}
			return append(out, src[next:]...)
		}
		pos = next
	}
	return src
}

// nextLine returns the line starting at pos, newline included, and the offset
// of the line after it.
func nextLine(src []byte, pos int) (line []byte, next int) {
	if pos >= len(src) {
		return nil, len(src)
	}
	if i := bytes.IndexByte(src[pos:], '\n'); i >= 0 {
		return src[pos : pos+i+1], pos + i + 1
	}
	return src[pos:], len(src)
}

func isFence(l []byte) bool {
	t := bytes.TrimRight(l, "\r\n")
	return bytes.Equal(t, []byte("---")) || bytes.Equal(t, []byte("..."))
}

func inlineText(n ast.Node, src []byte) string {
	var sb strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			sb.Write(t.Segment.Value(src))
			if t.SoftLineBreak() {
				sb.WriteByte(' ')
			}
		case *ast.AutoLink:
			sb.Write(t.Label(src))
		case *ast.String:
			sb.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}
