package doc

import (
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
)

// Renderer renders documents with glamour, caching rendered blocks so
// re-renders (resize, reload) only redo blocks that changed. It keeps the
// blocks of the last few widths, so panes of different widths share it
// without evicting each other.
type Renderer struct {
	style  string
	mu     sync.Mutex
	cache  map[int]map[string][]string     // width -> block text -> lines
	terms  map[int][]*glamour.TermRenderer // width -> idle renderers, as many as maxIdleTerms
	widths []int                           // the cached widths, least recently used first
}

// maxWidths is how many widths the cache holds: enough for a few panes side
// by side and a resize.
const maxWidths = 4

// maxIdleTerms is how many idle glamour renderers a width keeps. Building one
// parses the style, so a resize should not rebuild them; keeping one per core
// for every width would cost more memory than it saves.
const maxIdleTerms = 2

// minCachedBlocks is the smallest cache a document is allowed before it can
// be dropped for a smaller one: room for a handful of small files, so
// switching tabs does not re-render them.
const minCachedBlocks = 4096

// seqBlocks is the largest batch of blocks rendered without worker
// goroutines, which cost more than they save for a handful of blocks.
const seqBlocks = 16

func NewRenderer(style string) *Renderer {
	return &Renderer{
		style: style,
		cache: map[int]map[string][]string{},
		terms: map[int][]*glamour.TermRenderer{},
	}
}

// widthCache returns the cache of blocks rendered at width, making it the
// most recently used and evicting the least if there are too many. r.mu must
// be held.
func (r *Renderer) widthCache(width int) map[string][]string {
	if i := slices.Index(r.widths, width); i >= 0 {
		r.widths = append(slices.Delete(r.widths, i, i+1), width)
		return r.cache[width]
	}
	if len(r.widths) == maxWidths {
		delete(r.cache, r.widths[0])
		delete(r.terms, r.widths[0])
		r.widths = r.widths[1:]
	}
	c := map[string][]string{}
	r.cache[width] = c
	r.widths = append(r.widths, width)
	return c
}

func (r *Renderer) newTerm(width int) (*glamour.TermRenderer, error) {
	return glamour.NewTermRenderer(
		glamour.WithStylePath(r.style),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
}

// getTerms borrows n renderers for width, reusing idle ones before building
// the rest. They go back with putTerms.
func (r *Renderer) getTerms(width, n int) ([]*glamour.TermRenderer, error) {
	r.mu.Lock()
	pool := r.terms[width]
	var terms []*glamour.TermRenderer
	if k := min(n, len(pool)); k > 0 {
		terms = append(terms, pool[len(pool)-k:]...)
		r.terms[width] = pool[:len(pool)-k]
	}
	r.mu.Unlock()
	for len(terms) < n {
		tr, err := r.newTerm(width)
		if err != nil {
			r.putTerms(width, terms)
			return nil, err
		}
		terms = append(terms, tr)
	}
	return terms, nil
}

// putTerms gives renderers back to their width's pool, unless the width was
// evicted while they were in use.
func (r *Renderer) putTerms(width int, terms []*glamour.TermRenderer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.cache[width]; !ok {
		return
	}
	if n := maxIdleTerms - len(r.terms[width]); n > 0 {
		r.terms[width] = append(r.terms[width], terms[:min(n, len(terms))]...)
	}
}

// Rendered is a document rendered at a fixed width.
type Rendered struct {
	Lines []string
	spans []span // parallel to Doc.Blocks
}

// span places a block in rendered output. The block's lines are
// [RenStart, RenEnd); one blank separator line follows each block.
type span struct {
	RenStart, RenEnd int
}

// Render lays out every block of d. Glamour pads blocks with a varying number
// of blank lines, so each block is trimmed and blocks are joined by exactly one
// blank line, which matches glamour's whole-document output.
func (r *Renderer) Render(d *Doc, width int) (*Rendered, error) {
	if width < 10 {
		width = 10
	}
	if d.plain {
		return renderPlain(d, width), nil
	}
	texts := make([]string, len(d.Blocks))
	var todo []string
	seen := map[string]bool{}
	r.mu.Lock()
	cache := r.widthCache(width)
	if len(cache) > max(2*len(d.Blocks), minCachedBlocks) {
		// The cache holds far more than this document needs: the session has
		// moved on to other files, so start it over rather than keep every
		// block ever rendered in memory.
		cache = map[string][]string{}
		r.cache[width] = cache
	}
	for i := range d.Blocks {
		t := d.renderText(i)
		texts[i] = t
		if _, ok := cache[t]; !ok && !seen[t] {
			seen[t] = true
			todo = append(todo, t)
		}
	}
	r.mu.Unlock()

	if err := r.renderAll(cache, todo, width); err != nil {
		return nil, err
	}

	out := &Rendered{Lines: []string{""}, spans: make([]span, len(d.Blocks))}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, t := range texts {
		lines := cache[t]
		s := span{RenStart: len(out.Lines)}
		out.Lines = append(out.Lines, lines...)
		s.RenEnd = len(out.Lines)
		out.spans[i] = s
		if len(lines) > 0 {
			out.Lines = append(out.Lines, "")
		}
	}
	return out, nil
}

// plainMargin is the left margin of plain text, the same as glamour's
// built-in styles give a markdown document.
const plainMargin = "  "

// renderPlain lays out a plain-text doc as written: each line wrapped to the
// width, with no blank line between blocks since each block is one line.
func renderPlain(d *Doc, width int) *Rendered {
	limit := width - 2*len(plainMargin)
	out := &Rendered{Lines: make([]string, 1, len(d.Blocks)+2), spans: make([]span, len(d.Blocks))}
	for i, b := range d.Blocks {
		s := span{RenStart: len(out.Lines)}
		for l := range strings.SplitSeq(ansi.Wrap(expandTabs(b.Text), limit, ""), "\n") {
			out.Lines = append(out.Lines, plainMargin+l)
		}
		s.RenEnd = len(out.Lines)
		out.spans[i] = s
	}
	out.Lines = append(out.Lines, "")
	return out
}

// tabWidth is the tab stop of plain text.
const tabWidth = 4

// expandTabs replaces tabs with spaces to the next tab stop, since the
// terminal's own tab stops ignore the margin and wrapping.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var sb strings.Builder
	col := 0
	for _, c := range s {
		if c == '\t' {
			n := tabWidth - col%tabWidth
			sb.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		sb.WriteRune(c)
		col += ansi.StringWidth(string(c))
	}
	return sb.String()
}

// renderAll renders texts into cache, in parallel for large documents. Each
// worker owns its own TermRenderer since they are not goroutine-safe.
func (r *Renderer) renderAll(cache map[string][]string, texts []string, width int) error {
	if len(texts) == 0 {
		return nil
	}
	workers := min(runtime.NumCPU(), len(texts)/seqBlocks+1)
	terms, err := r.getTerms(width, workers)
	if err != nil {
		return err
	}
	defer r.putTerms(width, terms)

	if len(terms) == 1 {
		for _, t := range texts {
			r.renderBlock(cache, terms[0], t)
		}
		return nil
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	for _, tr := range terms {
		wg.Add(1)
		go func(tr *glamour.TermRenderer) {
			defer wg.Done()
			for t := range jobs {
				r.renderBlock(cache, tr, t)
			}
		}(tr)
	}
	for _, t := range texts {
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	return nil
}

// renderBlock renders one block into cache. A block that fails to render is
// shown as its source rather than failing the whole document.
func (r *Renderer) renderBlock(cache map[string][]string, tr *glamour.TermRenderer, text string) {
	s, err := tr.Render(text)
	if err != nil {
		s = text
	}
	lines := trimBlank(strings.Split(s, "\n"))
	r.mu.Lock()
	cache[text] = lines
	r.mu.Unlock()
}

func isBlank(l string) bool { return strings.TrimSpace(ansi.Strip(l)) == "" }

func trimBlank(lines []string) []string {
	for len(lines) > 0 && isBlank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && isBlank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// BlockAt returns the index of the block containing source line src.
func (d *Doc) BlockAt(src int) int {
	i := sort.Search(len(d.Blocks), func(i int) bool { return d.Blocks[i].SrcStart > src }) - 1
	return max(i, 0)
}

// SourceToRendered maps a 0-based source line to a rendered line. Lines inside
// a block are interpolated linearly across the block's rendered height.
func (r *Rendered) SourceToRendered(d *Doc, src int) int {
	if len(d.Blocks) == 0 {
		return 0
	}
	i := d.BlockAt(src)
	b, s := d.Blocks[i], r.spans[i]
	if src >= b.SrcEnd {
		return s.RenEnd // trailing blank lines map to the separator after the block
	}
	return s.RenStart + lerp(src-b.SrcStart, b.SrcEnd-b.SrcStart, s.RenEnd-s.RenStart)
}

// RenderedToSource maps a rendered line back to a 0-based source line. The
// blank separator after a block maps to the start of the next block.
func (r *Rendered) RenderedToSource(d *Doc, ren int) int {
	if len(d.Blocks) == 0 {
		return 0
	}
	i := sort.Search(len(r.spans), func(i int) bool { return r.spans[i].RenStart > ren }) - 1
	if i < 0 {
		return 0
	}
	b, s := d.Blocks[i], r.spans[i]
	if ren >= s.RenEnd {
		return min(b.Next, d.Lines-1)
	}
	// Rounding up makes this the exact inverse of SourceToRendered: it returns
	// the first source line that maps to ren.
	return b.SrcStart + lerpUp(ren-s.RenStart, s.RenEnd-s.RenStart, b.SrcEnd-b.SrcStart)
}

// BlockSpan is the range [start, end) of rendered lines of block i.
func (r *Rendered) BlockSpan(i int) (start, end int) {
	return r.spans[i].RenStart, r.spans[i].RenEnd
}

// HeadingLine is the rendered line of heading h.
func (r *Rendered) HeadingLine(d *Doc, h int) int {
	return r.spans[d.Headings[h].Block].RenStart
}

// lerp scales an offset within a range of size from to one of size to.
func lerp(off, from, to int) int {
	if from <= 0 || to <= 0 {
		return 0
	}
	return min(off*to/from, to-1)
}

func lerpUp(off, from, to int) int {
	if from <= 0 || to <= 0 {
		return 0
	}
	return min((off*to+from-1)/from, to-1)
}
