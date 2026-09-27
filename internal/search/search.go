// Package search runs ripgrep and parses its JSON output. A regex query holds
// space-separated terms; a line matches only if every term does. A literal
// query is one term, spaces and all.
package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Scope is how much ripgrep searches around the current file.
type Scope int

const (
	File Scope = iota // the current file
	Dir               // the current file's directory, recursively
	Repo              // the enclosing git repository (or Dir outside one)
)

var scopeNames = [...]string{"file", "dir", "repo"}

func (s Scope) String() string { return scopeNames[s] }

// Next cycles file → dir → repo → file (or backwards for step < 0).
func (s Scope) Next(step int) Scope {
	n := Scope(len(scopeNames))
	return ((s+Scope(step))%n + n) % n
}

// Root returns the absolute path ripgrep searches for scope s around file.
func Root(file string, s Scope) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	switch s {
	case File:
		return abs
	case Repo:
		if r := RepoRoot(filepath.Dir(abs)); r != "" {
			return r
		}
	}
	return filepath.Dir(abs)
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

// ParseCase is the inverse of Case.String.
func ParseCase(s string) (Case, bool) {
	for i, n := range caseNames {
		if n == s {
			return Case(i), true
		}
	}
	return 0, false
}

// ParseScope is the inverse of Scope.String.
func ParseScope(s string) (Scope, bool) {
	for i, n := range scopeNames {
		if n == s {
			return Scope(i), true
		}
	}
	return 0, false
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
	Root  string // file or directory to search, from Root
	Limit int    // stop after this many matching lines; 0 means the default
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
		if t := m.Text[s[0]:s[1]]; t != "" && !contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

type Result struct {
	Matches   []Match // sorted by path, then line
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
func (r Request) Patterns() ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, t := range r.Terms() {
		expr := t
		if r.Literal {
			expr = regexp.QuoteMeta(t)
		}
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
// single file is searched whatever its extension. The terms (see Terms) are
// ANDed: a line matches only if every term matches it, and then carries the
// spans of every term, so the results list and the pane highlight them all.
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
	terms := req.Terms()
	if len(terms) == 0 {
		return Result{}, nil
	}
	seen := map[string]bool{}
	uniq := make([]string, 0, len(terms))
	for _, t := range terms {
		if !seen[t] {
			seen[t] = true
			uniq = append(uniq, t)
		}
	}
	if len(uniq) == 1 {
		ms, truncated, err := runOne(parent, rg, req, uniq[0], limit)
		if err != nil {
			return Result{}, err
		}
		return finish(ms, truncated), nil
	}
	// One ripgrep run per term, intersected on file and line: the terms must
	// share a line, not just a file.
	type key struct {
		path string
		line int
	}
	byLine := map[key]*Match{}
	var truncated bool
	for i, t := range uniq {
		if err := parent.Err(); err != nil {
			return Result{}, err // superseded by a newer search
		}
		ms, trunc, err := runOne(parent, rg, req, t, limit)
		if err != nil {
			return Result{}, err
		}
		truncated = truncated || trunc
		if i == 0 {
			for j := range ms {
				m := ms[j]
				byLine[key{m.Path, m.Line}] = &m
			}
		} else {
			keep := map[key]bool{}
			for j := range ms {
				k := key{ms[j].Path, ms[j].Line}
				base, ok := byLine[k]
				if !ok || keep[k] {
					continue
				}
				// The file may have changed between runs; clamp so Terms
				// can't slice past the base text.
				for _, s := range ms[j].Spans {
					if end := min(s[1], len(base.Text)); s[0] < end {
						base.Spans = append(base.Spans, [2]int{s[0], end})
					}
				}
				keep[k] = true
			}
			for k := range byLine {
				if !keep[k] {
					delete(byLine, k)
				}
			}
		}
		if len(byLine) == 0 {
			break
		}
	}
	out := make([]Match, 0, len(byLine))
	for _, m := range byLine {
		out = append(out, *m)
	}
	return finish(out, truncated), nil
}

// runOne runs one term through ripgrep, returning its matching lines.
func runOne(parent context.Context, rg string, req Request, term string, limit int) ([]Match, bool, error) {
	args := []string{"--json", [...]string{"-i", "-S", "-s"}[req.Case]}
	if req.Literal {
		args = append(args, "-F")
	}
	if req.Scope != File {
		args = append(args, "-t", "markdown")
	}
	args = append(args, "--", term, req.Root)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}

	var ms []Match
	var truncated bool
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		m, ok := parseMatch(sc.Bytes())
		if !ok {
			continue
		}
		if len(ms) == limit {
			truncated = true
			cancel() // kill rg; we have enough
			break
		}
		ms = append(ms, m)
	}
	waitErr := cmd.Wait()

	if err := parent.Err(); err != nil {
		return nil, false, err // superseded by a newer search
	}
	// Exit 1 means no matches. Exit 2 means an error, but ripgrep keeps going
	// past unreadable files, so only report it when nothing was found.
	var exit *exec.ExitError
	if waitErr != nil && !truncated && len(ms) == 0 {
		if !errors.As(waitErr, &exit) || exit.ExitCode() != 1 {
			if msg := firstLines(stderr.String()); msg != "" {
				return nil, false, errors.New(msg)
			}
			return nil, false, waitErr
		}
	}
	return ms, truncated, nil
}

// finish sorts matches by path, then line, and counts the files.
func finish(ms []Match, truncated bool) Result {
	for i := range ms {
		s := ms[i].Spans
		sort.Slice(s, func(a, b int) bool {
			if s[a][0] != s[b][0] {
				return s[a][0] < s[b][0]
			}
			return s[a][1] < s[b][1]
		})
	}
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].Path != ms[j].Path {
			return ms[i].Path < ms[j].Path
		}
		return ms[i].Line < ms[j].Line
	})
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

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
