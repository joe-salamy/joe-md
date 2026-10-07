// Package doc parses markdown into top-level blocks with exact source line
// ranges, renders them with glamour, and maps between source and rendered
// line numbers.
package doc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	renders  []string // the markdown handed to glamour for each block
}

// ref is a link reference definition: its label, normalised as normLabel
// does, and its source text.
type ref struct{ label, text string }

// ErrBinary is Load's error for a file that isn't text.
var ErrBinary = errors.New("not a text file")

// Load reads and parses the markdown file at path. The doc's path is
// absolute, so file identity, display and editing never depend on how the
// file was named on its way in.
func Load(path string) (*Doc, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isBinary(src) {
		return nil, fmt.Errorf("%s is %w", filepath.Base(path), ErrBinary)
	}
	d := Parse(src)
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
	// nothing, so they are not blocks of their own; they are appended to the
	// blocks that use them instead so reference-style links still resolve.
	type start struct {
		line int
		node ast.Node
	}
	var starts []start
	var refs []ref
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
			refs = append(refs, ref{label, joinLines(lines, l, end)})
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
		} else if p, ok := s.node.(*ast.Paragraph); ok && isStandaloneBold(p, src) {
			if text := strings.TrimSpace(inlineText(p, src)); text != "" {
				b.Heading = len(d.Headings)
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
	return d
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
func withRefs(text string, refs []ref) string {
	norm := normLabel(text)
	var used []string
	for _, r := range refs {
		if strings.Contains(norm, "["+r.label+"]") && !strings.Contains(text, r.text) {
			used = append(used, strings.TrimSuffix(r.text, "\n"))
		}
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

func joinLines(lines [][]byte, from, to int) string {
	return string(bytes.Join(lines[from:to], []byte("\n"))) + "\n"
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
	lines := bytes.SplitAfter(src, []byte("\n"))
	if len(lines) < 2 || !yamlKey.Match(bytes.TrimRight(lines[1], "\r\n")) &&
		!isFence(lines[1]) {
		return src
	}
	for i := 1; i < len(lines); i++ {
		if isFence(lines[i]) {
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
		case *ast.String:
			sb.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}
