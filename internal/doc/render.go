package doc

import (
	"runtime"
	"sort"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
)

// Renderer renders documents with glamour at a given width, caching rendered
// blocks so re-renders (resize, reload) only redo blocks that changed.
type Renderer struct {
	style string
	width int
	mu    sync.Mutex
	cache map[string][]string
}

func NewRenderer(style string) *Renderer {
	return &Renderer{style: style, cache: map[string][]string{}}
}

func (r *Renderer) newTerm(width int) (*glamour.TermRenderer, error) {
	return glamour.NewTermRenderer(
		glamour.WithStylePath(r.style),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
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
	r.mu.Lock()
	if width != r.width {
		r.width = width
		r.cache = map[string][]string{}
	}
	var todo []string
	seen := map[string]bool{}
	for i := range d.Blocks {
		t := d.renderText(i)
		if _, ok := r.cache[t]; !ok && !seen[t] {
			seen[t] = true
			todo = append(todo, t)
		}
	}
	r.mu.Unlock()

	if err := r.renderAll(todo, width); err != nil {
		return nil, err
	}

	out := &Rendered{Lines: []string{""}, spans: make([]span, len(d.Blocks))}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range d.Blocks {
		lines := r.cache[d.renderText(i)]
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

// renderAll renders texts into the cache, in parallel for large documents.
// Each worker owns its own TermRenderer since they are not goroutine-safe.
func (r *Renderer) renderAll(texts []string, width int) error {
	workers := min(runtime.NumCPU(), len(texts)/16+1)
	jobs := make(chan string)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr, err := r.newTerm(width)
			if err != nil {
				errs <- err
				for range jobs {
				}
				return
			}
			for t := range jobs {
				s, err := tr.Render(t)
				if err != nil {
					s = t // fall back to raw source rather than failing the whole doc
				}
				lines := trimBlank(strings.Split(s, "\n"))
				r.mu.Lock()
				r.cache[t] = lines
				r.mu.Unlock()
			}
		}()
	}
	for _, t := range texts {
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
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
