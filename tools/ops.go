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

// Read returns file text. A binary file is a one-line marker, not an error.
// offset is 1-based. limit 0 uses the default line window.
func Read(root, path string, offset, limit int) (string, error) {
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
	if offset < 1 {
		offset = 1
	}
	if limit < 1 {
		limit = defaultGetLines
	}
	if limit > maxGetLines {
		limit = maxGetLines
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var (
		lines []string
		total int
	)
	for sc.Scan() {
		total++
		if total < offset {
			continue
		}
		if len(lines) >= limit {
			continue
		}
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if total == 0 {
		return "", nil
	}
	if offset > total {
		return "", fmt.Errorf("offset %d is past end (%d lines)", offset, total)
	}
	end := offset + len(lines) - 1
	body := strings.Join(lines, "\n")
	if offset == 1 && end == total {
		if endsWithNewline(f) {
			body += "\n"
		}
		return body, nil
	}
	return fmt.Sprintf("range: %d-%d of %d\n%s\n", offset, end, total, body), nil
}

func endsWithNewline(f *os.File) bool {
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return false
	}
	if _, err := f.Seek(-1, io.SeekEnd); err != nil {
		return false
	}
	var b [1]byte
	if _, err := f.Read(b[:]); err != nil {
		return false
	}
	return b[0] == '\n'
}

// Search walks path for a literal query. glob matches the base name.
func Search(ctx context.Context, root, path, query, glob string, limit int) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, errors.New("query is required")
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
		hits = append(hits, Hit{Path: rel, Line: lineNo, Text: clipRunes(text, maxHitRunes)})
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
	content, err := os.ReadFile(abs) //nolint:gosec // G304: path resolved inside --root
	if err != nil {
		return "", err
	}
	if bytes.Contains(content, []byte{0}) {
		return "", errors.New("binary file")
	}
	rel, err := Rel(root, abs)
	if err != nil {
		return "", err
	}
	next, err := ApplyUnified(rel, diff, content)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(abs, next, info.Mode().Perm()); err != nil {
		return "", err
	}
	return "patched: " + rel, nil
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

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}

// JSON encodes v for a tool result.
func JSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
