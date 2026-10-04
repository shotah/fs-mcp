// Package tools is the workspace file catalog. The host publishes these as fs__file_get.
package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	defaultGetLines  = 200
	maxGetLines      = 2000
	defaultMaxChars  = 6000
	maxFileBytes     = 2 << 20
	maxSearchHits    = 40
	maxSearchFiles   = 4000
	maxSearchFileLen = 512 << 10
	maxHitRunes      = 200
)

// Entry is one directory listing row.
type Entry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Size int64  `json:"size,omitempty"`
}

// ListResult is the JSON body of file_list.
type ListResult struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

// Hit is one file_search match.
type Hit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// SearchResult is the JSON body of file_search.
type SearchResult struct {
	Hits      []Hit `json:"hits"`
	Truncated bool  `json:"truncated"`
}

// List returns one directory level.
func List(root, path string) (ListResult, error) {
	abs, err := Resolve(root, path, false)
	if err != nil {
		return ListResult{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return ListResult{}, err
	}
	if !info.IsDir() {
		return ListResult{}, errors.New("not a directory")
	}
	dirents, err := os.ReadDir(abs)
	if err != nil {
		return ListResult{}, err
	}
	rel, err := Rel(root, abs)
	if err != nil {
		return ListResult{}, err
	}
	out := ListResult{Path: rel, Entries: make([]Entry, 0, len(dirents))}
	for _, d := range dirents {
		kind := "other"
		var size int64
		switch {
		case d.Type()&os.ModeSymlink != 0:
			kind = "symlink"
		case d.IsDir():
			kind = "dir"
		case d.Type().IsRegular():
			kind = "file"
			if info, err := d.Info(); err == nil {
				size = info.Size()
			}
		}
		out.Entries = append(out.Entries, Entry{Name: d.Name(), Kind: kind, Size: size})
	}
	sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
	return out, nil
}

// Read returns file text. Every line is "N: text" and the result opens with
// a range header. A page is at most limit lines and maxChars bytes, cut at a
// line. maxChars 0 uses the default. A binary file is a one-line marker.
// offset is 1-based. limit 0 uses the default line window.
func Read(root, path string, offset, limit, maxChars int) (string, error) {
	abs, err := Resolve(root, path, false)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a file")
	}
	if info.Size() > maxFileBytes {
		return "", fmt.Errorf("file is too large (%d bytes)", info.Size())
	}
	f, err := os.Open(abs) //nolint:gosec // G304: path resolved inside --root
	if err != nil {
		return "", err
	}
	defer f.Close()
	rel, err := Rel(root, abs)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 8192)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	if bytes.Contains(buf[:n], []byte{0}) {
		return "binary file: " + rel, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	content, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return "", err
	}
	if offset < 1 {
		offset = 1
	}
	if limit < 1 {
		limit = defaultGetLines
	}
	if limit > maxGetLines {
		limit = maxGetLines
	}
	if maxChars < 1 {
		maxChars = defaultMaxChars
	}
	return formatPage(splitLines(content), offset, limit, maxChars)
}

func formatPage(lines []string, offset, limit, maxChars int) (string, error) {
	total := len(lines)
	if total == 0 {
		if offset > 1 {
			return "", fmt.Errorf("offset %d is past end (last line 0)", offset)
		}
		return "range: 0-0 of 0; end", nil
	}
	if offset > total {
		return "", fmt.Errorf("offset %d is past end (last line %d)", offset, total)
	}
	start := offset - 1
	end := min(start+limit, total)
	for end > start {
		page := renderPage(lines, start, end, total)
		if len(page) <= maxChars {
			return page, nil
		}
		if end == start+1 {
			return clipPageLine(lines[start], start, total, maxChars), nil
		}
		end--
	}
	return "", errors.New("empty page")
}

func rangeHeader(start, end, total int) string {
	if end < total {
		return fmt.Sprintf("range: %d-%d of %d; next offset %d", start+1, end, total, end+1)
	}
	return fmt.Sprintf("range: %d-%d of %d; end", start+1, end, total)
}

func renderPage(lines []string, start, end, total int) string {
	var b strings.Builder
	b.WriteString(rangeHeader(start, end, total))
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "\n%d: %s", i+1, lines[i])
	}
	return b.String()
}

func clipPageLine(line string, index, total, maxChars int) string {
	header := rangeHeader(index, index+1, total)
	prefix := fmt.Sprintf("\n%d: ", index+1)
	budget := maxChars - len(header) - len(prefix)
	if budget <= 0 {
		return header
	}
	return header + prefix + clipToBytes(line, budget)
}

func clipToBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := 0
	for i < len(s) {
		_, size := utf8.DecodeRuneInString(s[i:])
		if i+size > n {
			break
		}
		i += size
	}
	return s[:i]
}

// Search walks path for a literal query. glob matches the base name.
func Search(ctx context.Context, root, path, query, glob string, limit int) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, fmt.Errorf("query is required, e.g. %s", queryExample)
	}
	abs, err := Resolve(root, path, false)
	if err != nil {
		return SearchResult{}, err
	}
	if glob != "" {
		if _, err := filepath.Match(glob, "x"); err != nil {
			return SearchResult{}, fmt.Errorf("glob: %w", err)
		}
	}
	if limit < 1 || limit > maxSearchHits {
		limit = maxSearchHits
	}
	var result SearchResult
	visited := 0
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		if glob != "" {
			ok, err := filepath.Match(glob, d.Name())
			if err != nil || !ok {
				return nil
			}
		}
		visited++
		if visited > maxSearchFiles {
			result.Truncated = true
			return fs.SkipAll
		}
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil
		}
		if err := inside(root, resolved); err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSearchFileLen {
			return nil
		}
		hits, err := searchFile(resolved, root, query, limit-len(result.Hits))
		if err != nil {
			return nil
		}
		result.Hits = append(result.Hits, hits...)
		if len(result.Hits) >= limit {
			result.Truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return SearchResult{}, err
	}
	if result.Hits == nil {
		result.Hits = []Hit{}
	}
	return result, nil
}

func searchFile(abs, root, query string, room int) ([]Hit, error) {
	if room < 1 {
		return nil, nil
	}
	f, err := os.Open(abs) //nolint:gosec // G304: path resolved inside --root
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, err
	}
	buf := make([]byte, 8192)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if bytes.Contains(buf[:n], []byte{0}) {
		return nil, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	rel, err := Rel(root, abs)
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var hits []Hit
	lineNo := 0
	for sc.Scan() {
		lineNo++
		text := sc.Text()
		if !strings.Contains(text, query) {
			continue
		}
		hits = append(hits, Hit{Path: rel, Line: lineNo, Text: clipRunes(text)})
		if len(hits) >= room {
			break
		}
	}
	return hits, sc.Err()
}

// Create writes a new file. An existing path is an error.
func Create(root, path, contents string) (string, error) {
	abs, err := Resolve(root, path, true)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(abs); err == nil {
		return "", errors.New("path already exists")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644) //nolint:gosec // G304: path resolved inside --root
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(contents); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	rel, err := Rel(root, abs)
	if err != nil {
		return "", err
	}
	return "created: " + rel, nil
}

// Patch applies diff to one existing file. A mismatch writes nothing.
func Patch(root, path, diff string) (string, error) {
	abs, rel, info, content, err := loadTextFile(root, path)
	if err != nil {
		return "", err
	}
	next, spans, err := applyUnified(rel, diff, content)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(abs, next, info.Mode().Perm()); err != nil {
		return "", err
	}
	return formatPatchResult(rel, next, spans, ""), nil
}

// Replace writes newText over old in one file.
// count 0 replaces every exact match, or one forgiving match when nothing is exact.
// A positive count must equal the number of matches at the level that hits.
// Zero matches or a count miss writes nothing.
func Replace(root, path, old, newText string, count int) (string, error) {
	if old == "" {
		return "", errors.New("old is empty")
	}
	if count < 0 {
		return "", errors.New("count must be at least 1")
	}
	abs, rel, info, content, err := loadTextFile(root, path)
	if err != nil {
		return "", err
	}
	text := string(content)
	spans, note, err := locateOld(rel, text, old, count)
	if err != nil {
		return "", err
	}
	nextText, lineSpans := applyByteSpans(text, newText, spans)
	if nextText == text {
		return "", fmt.Errorf("old and new are the same in %s; nothing written", rel)
	}
	next := []byte(nextText)
	if err := writeAtomic(abs, next, info.Mode().Perm()); err != nil {
		return "", err
	}
	return formatPatchResult(rel, next, lineSpans, note), nil
}

// Insert writes newText after a 1-based line. Line 0 inserts at the top.
func Insert(root, path string, after int, newText string) (string, error) {
	abs, rel, info, content, err := loadTextFile(root, path)
	if err != nil {
		return "", err
	}
	nextText, spans, err := insertAfter(string(content), after, newText)
	if err != nil {
		return "", err
	}
	next := []byte(nextText)
	if err := writeAtomic(abs, next, info.Mode().Perm()); err != nil {
		return "", err
	}
	return formatPatchResult(rel, next, spans, ""), nil
}

func loadTextFile(root, path string) (abs, rel string, info os.FileInfo, content []byte, err error) {
	abs, err = Resolve(root, path, false)
	if err != nil {
		return "", "", nil, nil, err
	}
	info, err = os.Stat(abs)
	if err != nil {
		return "", "", nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return "", "", nil, nil, errors.New("not a file")
	}
	if info.Size() > maxFileBytes {
		return "", "", nil, nil, fmt.Errorf("file is too large (%d bytes)", info.Size())
	}
	content, err = os.ReadFile(abs) //nolint:gosec // G304: path resolved inside --root
	if err != nil {
		return "", "", nil, nil, err
	}
	if bytes.Contains(content, []byte{0}) {
		return "", "", nil, nil, errors.New("binary file")
	}
	rel, err = Rel(root, abs)
	if err != nil {
		return "", "", nil, nil, err
	}
	return abs, rel, info, content, nil
}

// Delete removes one file. Directories and globs are refused.
func Delete(root, path string) (string, error) {
	if strings.ContainsAny(path, "*?[") {
		return "", errors.New("one path, no globs")
	}
	abs, err := Resolve(root, path, false)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("not a file")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", err
		}
		if err := inside(root, resolved); err != nil {
			return "", err
		}
	}
	if err := os.Remove(abs); err != nil {
		return "", err
	}
	rel, err := Rel(root, abs)
	if err != nil {
		return "", err
	}
	return "deleted: " + rel, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fs-mcp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func clipRunes(s string) string {
	if utf8.RuneCountInString(s) <= maxHitRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxHitRunes])
}

// JSON encodes v for a tool result.
func JSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
