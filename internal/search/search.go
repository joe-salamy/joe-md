// Package search runs ripgrep and parses its JSON output. A regex query holds
// space-separated terms; a line matches only if every term does. A literal
// query is one phrase, matched with any whitespace or inline markup between
// its words.
package search

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/joe-salamy/joe-md/internal/natural"
)

// Scope is how much ripgrep searches around the current file.
type Scope int

const (
	File Scope = iota // the current file
	Open              // every open file
	Dir               // the current file's directory, recursively
	Repo              // the enclosing git repository (or Dir outside one)
)

var scopeNames = [...]string{"file", "open", "dir", "repo"}

func (s Scope) String() string { return scopeNames[s] }

// Next cycles file → open → dir → repo → file (or backwards for step < 0).
func (s Scope) Next(step int) Scope {
	n := Scope(len(scopeNames))
	return ((s+Scope(step))%n + n) % n
}

// Root returns the path ripgrep searches for scope s around file, which
// must be absolute (as doc paths are), so ripgrep reports absolute paths.
func Root(file string, s Scope) string {
	switch s {
	case File:
		return file
	case Repo:
		if r := RepoRoot(filepath.Dir(file)); r != "" {
			return r
		}
	}
	return filepath.Dir(file)
}

// RepoRoot is the nearest directory at or above dir containing .git (a
// directory, or a file for worktrees and submodules), or "" if there is none.
func RepoRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Case is how a search treats letter case.
type Case int

const (
	IgnoreCase Case = iota // rg -i
	SmartCase              // rg -S: case-sensitive only if the query has capitals
	MatchCase              // rg -s
)

var caseNames = [...]string{"ignore", "smart", "sensitive"}

func (c Case) String() string { return caseNames[c] }

// Scope and Case are written by name in the settings file.

func (s Scope) MarshalText() ([]byte, error) { return []byte(s.String()), nil }
func (c Case) MarshalText() ([]byte, error)  { return []byte(c.String()), nil }

func (s *Scope) UnmarshalText(b []byte) error { return unmarshalName(s, "scope", scopeNames[:], b) }
func (c *Case) UnmarshalText(b []byte) error  { return unmarshalName(c, "case", caseNames[:], b) }

// unmarshalName sets *v to the index of name b in names, the inverse of the
// String methods; what names the setting in the error.
func unmarshalName[T ~int](v *T, what string, names []string, b []byte) error {
	i := slices.Index(names, string(b))
	if i < 0 {
		return fmt.Errorf("%s %q: must be %s or %s", what, b, strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
	}
	*v = T(i)
	return nil
}

// Mode is how a query is matched.
type Mode struct {
	Literal bool // a fixed string (rg -F), not a regex
	Case    Case
}

// Request is one search.
type Request struct {
	Query string // see Terms
	Mode
	Scope Scope
	Root  string   // file or directory to search, from Root
	Files []string // open files to search, for Open (absolute paths)
	Limit int      // stop after this many matching lines; 0 means the default
}

// Default limits: a list of results across files is only useful up to a
// point, but n / N within one file should reach every match.
const (
	DefaultLimit     = 5000
	DefaultFileLimit = 1_000_000
)

// Match is one matching line.
type Match struct {
	Path  string   // as ripgrep printed it (absolute, since Root is)
	Line  int      // 1-based
	Text  string   // the line, without its line ending
	Spans [][2]int // byte ranges of the matches within Text
}

// Terms returns the distinct matched strings on the line.
func (m Match) Terms() []string {
	var out []string
	for _, s := range m.Spans {
		if t := m.Text[s[0]:s[1]]; t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

type Result struct {
	Matches   []Match // sorted by path (natural.PathCompare), then line
	Files     int     // number of distinct files in Matches
	Truncated bool    // Limit was hit
}

// Terms splits the query into the terms that must all match a line. A regex
// query splits on spaces; a literal query is one phrase, trimmed.
func (r Request) Terms() []string {
	if r.Literal {
		if q := strings.TrimSpace(r.Query); q != "" {
			return []string{q}
		}
		return nil
	}
	return strings.Fields(r.Query)
}

// Patterns compiles each term as a Go regexp that matches what ripgrep would,
// so matches can be found again in text ripgrep never saw. Go's syntax is
// close to ripgrep's but not identical; a term Go can't compile is an error.
// The patterns are multi-line, so ^ and $ still match at each line of a text
// holding several.
func (r Request) Patterns() ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, t := range r.Terms() {
		expr := "(?m)" + r.expr(t)
		if r.Case == IgnoreCase || r.Case == SmartCase && !hasUpper(t, r.Literal) {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

// expr is the regexp ripgrep runs for term: the term itself, or for a
// literal query its phrase (see phrase).
func (r Request) expr(term string) string {
	if r.Literal {
		return phrase(term)
	}
	return term
}

// Inline markup that may sit next to the spaces of a phrase: emphasis and
// code marks, link brackets and targets, and HTML tags.
const markup = "(?:[*_~`]|\\[|\\]\\([^)]*\\)|\\]|<[^>]*>)*"

// phrase is the regexp for literal phrase q: its words, quoted, with any
// whitespace between them (a no-break space included), and markup allowed on
// either side of it, so `one **fixed** string` matches "one fixed string".
// The syntax is common to Go and ripgrep.
func phrase(q string) string {
	words := strings.Fields(q)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	return strings.Join(words, markup+"[\\s\\x{a0}]+"+markup)
}

// hasUpper reports whether term has a capital letter, as ripgrep's smart case
// sees it: in a regex, an escape such as \W or \S is not a capital.
func hasUpper(term string, literal bool) bool {
	escaped := false
	for _, c := range term {
		switch {
		case escaped:
			escaped = false
		case c == '\\' && !literal:
			escaped = true
		case unicode.IsUpper(c):
			return true
		}
	}
	return false
}

// Run searches with ripgrep. Directory scopes only search markdown files; a
// single file, and the open scope's explicit files, are searched whatever
// their extension. The terms (see Terms) are ANDed: a line matches only if
// every term matches it, and then carries the spans of every term, so the
// results list and the pane highlight them all.
//
// The rarest term searches the scope, and the lines it finds go in batches
// through the other terms (see narrow), so the search reads the files once
// and stops as soon as more than the limit of lines match every term.
func Run(parent context.Context, req Request) (Result, error) {
	rg, err := exec.LookPath("rg")
	if err != nil {
		return Result{}, errors.New("ripgrep (rg) not found in PATH")
	}
	limit := req.Limit
	switch {
	case limit > 0:
	case req.Scope == File:
		limit = DefaultFileLimit
	default:
		limit = DefaultLimit
	}
	var terms []string
	for _, t := range req.Terms() {
		if !slices.Contains(terms, t) {
			terms = append(terms, t)
		}
	}
	if len(terms) == 0 {
		return Result{}, nil
	}
	if len(terms) > 1 {
		terms = byRarity(parent, rg, req, terms)
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var out, batch []Match
	flush := func() error {
		ms := batch
		batch = nil
		var err error
		for _, t := range terms[1:] {
			if len(ms) == 0 {
				break
			}
			if ms, err = narrow(ctx, rg, req, t, ms); err != nil {
				return err
			}
		}
		out = append(out, ms...)
		return nil
	}
	var flushErr error
	err = stream(ctx, rg, req, terms[0], nil, func(m Match) bool {
		batch = append(batch, m)
		if len(batch) == narrowBatch || len(terms) == 1 {
			flushErr = flush()
		}
		return flushErr == nil && len(out) <= limit
	})
	if err == nil && flushErr == nil && len(out) <= limit {
		flushErr = flush()
	}
	if err := parent.Err(); err != nil {
		return Result{}, err // superseded by a newer search
	}
	if err != nil {
		return Result{}, err
	}
	if flushErr != nil {
		return Result{}, flushErr
	}
	return finish(out, limit), nil
}

// narrowBatch is how many lines of the first term go through the others at
// once: enough that few ripgrep runs are needed, few enough that a search
// of common terms stops soon after it has enough.
const narrowBatch = 8192

// byRarity orders terms by how many lines each matches, fewest first,
// counting them all at once. A term that fails to count goes last; the
// search proper reports its error.
func byRarity(ctx context.Context, rg string, req Request, terms []string) []string {
	counts := make([]int, len(terms))
	var wg sync.WaitGroup
	for i, t := range terms {
		wg.Go(func() { counts[i] = count(ctx, rg, req, t) })
	}
	wg.Wait()
	idx := make([]int, len(terms))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(counts[a], counts[b]) })
	out := make([]string, len(terms))
	for i, j := range idx {
		out[i] = terms[j]
	}
	return out
}

// count is how many lines term matches in the scope, or MaxInt on an error.
func count(ctx context.Context, rg string, req Request, term string) int {
	cmdArgs := append(append(args(req, false), "--count", "--no-filename", "--", req.expr(term)), paths(req, false)...)
	out, err := exec.CommandContext(ctx, rg, cmdArgs...).Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return math.MaxInt
	}
	n := 0
	for _, f := range strings.Fields(string(out)) {
		c, _ := strconv.Atoi(f)
		n += c
	}
	return n
}

// narrow keeps the lines of ms that term matches too, adding its spans. The
// lines' text goes to ripgrep on stdin, so term matches exactly as it would
// in the files, which aren't read again.
func narrow(ctx context.Context, rg string, req Request, term string, ms []Match) ([]Match, error) {
	var in bytes.Buffer
	for _, m := range ms {
		in.WriteString(m.Text)
		in.WriteByte('\n')
	}
	var out []Match
	err := stream(ctx, rg, req, term, &in, func(h Match) bool {
		if h.Line >= 1 && h.Line <= len(ms) {
			m := ms[h.Line-1] // stdin's line n is the n-th candidate
			m.Spans = append(slices.Clip(m.Spans), h.Spans...)
			out = append(out, m)
		}
		return true
	})
	return out, err
}

// args are ripgrep's options for searching the scope, or stdin.
func args(req Request, stdin bool) []string {
	out := []string{[...]string{"-i", "-S", "-s"}[req.Case]}
	switch {
	case stdin:
		out = append(out, "--text")
	case req.Scope != File && req.Scope != Open:
		out = append(out, "-t", "markdown")
	}
	return out
}

// paths are what ripgrep searches for req: stdin, the open files, or the root.
func paths(req Request, stdin bool) []string {
	switch {
	case stdin:
		return []string{"-"}
	case req.Scope == Open:
		return req.Files
	default:
		return []string{req.Root}
	}
}

// stream runs term through ripgrep over the scope's paths, or over stdin if
// it is set, handing each matching line to f until f returns false.
func stream(ctx context.Context, rg string, req Request, term string, stdin io.Reader, f func(Match) bool) error {
	cmdArgs := append(append(args(req, stdin != nil), "--json"), "--", req.expr(term))
	cmdArgs = append(cmdArgs, paths(req, stdin != nil)...)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg, cmdArgs...)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	found, stopped := false, false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		m, ok := parseMatch(sc.Bytes())
		if !ok {
			continue
		}
		found = true
		if !f(m) {
			stopped = true
			cancel() // kill rg; we have enough
			break
		}
	}
	// A line too long for the scanner ends the loop early. Kill ripgrep
	// before waiting: it would block writing to a pipe no one reads.
	scanErr := sc.Err()
	if scanErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if scanErr != nil {
		return fmt.Errorf("reading ripgrep output: %w", scanErr)
	}

	// Exit 1 means no matches. Exit 2 means an error, but ripgrep keeps going
	// past unreadable files, so only report it when nothing was found.
	var exit *exec.ExitError
	if waitErr != nil && !stopped && !found && ctx.Err() == nil {
		if !errors.As(waitErr, &exit) || exit.ExitCode() != 1 {
			if msg := firstLines(stderr.String()); msg != "" {
				return errors.New(msg)
			}
			return waitErr
		}
	}
	return nil
}

// finish sorts each match's spans, dropping any two terms matched alike,
// sorts matches by path, in menu order, then line, keeps the first limit, and
// counts the files.
func finish(ms []Match, limit int) Result {
	for i := range ms {
		slices.SortFunc(ms[i].Spans, func(a, b [2]int) int {
			return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
		})
		ms[i].Spans = slices.Compact(ms[i].Spans)
	}
	slices.SortStableFunc(ms, func(a, b Match) int {
		return cmp.Or(natural.PathCompare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})
	truncated := len(ms) > limit
	if truncated {
		ms = ms[:limit]
	}
	res := Result{Matches: ms, Truncated: truncated}
	for i, m := range ms {
		if i == 0 || m.Path != ms[i-1].Path {
			res.Files++
		}
	}
	return res
}

// rgText is ripgrep's encoding of possibly non-UTF-8 data.
type rgText struct {
	Text  *string `json:"text"`
	Bytes string  `json:"bytes"`
}

func (t rgText) String() string {
	if t.Text != nil {
		return *t.Text
	}
	b, _ := base64.StdEncoding.DecodeString(t.Bytes)
	return string(b)
}

type rgMessage struct {
	Type string `json:"type"`
	Data struct {
		Path       rgText `json:"path"`
		Lines      rgText `json:"lines"`
		LineNumber int    `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}

// parseMatch decodes one line of `rg --json` output if it is a match.
func parseMatch(line []byte) (Match, bool) {
	if !bytes.Contains(line, []byte(`"type":"match"`)) {
		return Match{}, false
	}
	var msg rgMessage
	if err := json.Unmarshal(line, &msg); err != nil || msg.Type != "match" {
		return Match{}, false
	}
	d := msg.Data
	m := Match{
		Path: d.Path.String(),
		Line: d.LineNumber,
		Text: strings.TrimRight(d.Lines.String(), "\r\n"),
	}
	for _, s := range d.Submatches {
		start, end := min(s.Start, len(m.Text)), min(s.End, len(m.Text))
		if start < end {
			m.Spans = append(m.Spans, [2]int{start, end})
		}
	}
	return m, true
}

// firstLines flattens ripgrep's (often multi-line) error message.
func firstLines(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "^") {
			parts = append(parts, l)
		}
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " ")
}
