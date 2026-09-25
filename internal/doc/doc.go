// Package doc parses markdown into top-level blocks with exact source line
// ranges, renders them with glamour, and maps between source and rendered
// line numbers.
package doc

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	Heading  int    // index into Doc.Headings, or -1
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
	refs     string // link reference definitions, appended to every block
}

// Load reads and parses the markdown file at path.
func Load(path string) (*Doc, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := Parse(src)
	d.Path = path
	d.Name = filepath.Base(path)
	return d, nil
}

// md must use the same block-level extensions glamour uses so that our block
// boundaries agree with what glamour renders.
var md = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList))

// Parse splits src into top-level blocks.
func Parse(src []byte) *Doc {
	src = blankFrontMatter(src)
	lineStarts := []int{0}
	for i, c := range src {
		if c == '\n' && i+1 < len(src) {
			lineStarts = append(lineStarts, i+1)
		}
	}
	lineOf := func(pos int) int {
		return sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > pos }) - 1
	}
	lines := bytes.Split(bytes.TrimSuffix(src, []byte("\n")), []byte("\n"))

	d := &Doc{Source: src, Lines: len(lines)}
	root := md.Parser().Parse(text.NewReader(src))

	// Collect block start lines. Link reference definitions render to
	// nothing, so they are not blocks of their own; they are appended to every
	// block instead so reference-style links still resolve.
	type start struct {
		line int
		node ast.Node
	}
	var starts []start
	var refs []string
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
			refs = append(refs, joinLines(lines, l, end))
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
	d.refs = strings.Join(refs, "\n")

	for i, s := range starts {
		b := Block{SrcStart: s.line, Next: d.Lines, Heading: -1}
		if i == 0 {
			b.SrcStart = 0 // leading blank lines / front matter belong to the first block
		}
		if i+1 < len(starts) {
			b.Next = starts[i+1].line
		}
		b.SrcEnd = b.Next
		for b.SrcEnd > b.SrcStart+1 && (refLine[b.SrcEnd-1] || len(bytes.TrimSpace(lines[b.SrcEnd-1])) == 0) {
			b.SrcEnd--
		}
		b.Text = joinLines(lines, s.line, b.Next)
		if h, ok := s.node.(*ast.Heading); ok {
			b.Heading = len(d.Headings)
			d.Headings = append(d.Headings, Heading{
				Level: h.Level,
				Text:  strings.TrimSpace(inlineText(h, src)),
				Block: i,
			})
		}
		d.Blocks = append(d.Blocks, b)
	}
	return d
}

// renderText is the markdown handed to glamour for block i.
func (d *Doc) renderText(i int) string {
	if d.refs == "" {
		return d.Blocks[i].Text
	}
	return d.Blocks[i].Text + "\n\n" + d.refs + "\n"
}

func joinLines(lines [][]byte, from, to int) string {
	return string(bytes.Join(lines[from:to], []byte("\n"))) + "\n"
}

// blankFrontMatter replaces a leading YAML front matter block with empty lines,
// hiding it from the renderer while keeping line numbers intact.
func blankFrontMatter(src []byte) []byte {
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
		return src
	}
	lines := bytes.SplitAfter(src, []byte("\n"))
	for i := 1; i < len(lines); i++ {
		t := bytes.TrimRight(lines[i], "\r\n")
		if bytes.Equal(t, []byte("---")) || bytes.Equal(t, []byte("...")) {
			out := make([]byte, 0, len(src))
			for j := 0; j <= i; j++ {
				out = append(out, '\n')
			}
			for _, l := range lines[i+1:] {
				out = append(out, l...)
			}
			return out
		}
	}
	return src
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
		case *ast.String:
			sb.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}
