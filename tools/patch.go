package tools

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// ApplyUnified applies a one-file unified diff to content.
// A diff that is only hunks is given a synthetic header so the parser accepts it.
// A conflict returns an error and leaves the caller to keep the original bytes.
func ApplyUnified(relPath, diff string, content []byte) ([]byte, error) {
	normalized, err := normalizeDiff(relPath, diff)
	if err != nil {
		return nil, err
	}
	files, _, err := gitdiff.Parse(strings.NewReader(normalized))
	if err != nil {
		return nil, err
	}
	if len(files) != 1 {
		return nil, errors.New("one file per call")
	}
	file := files[0]
	switch {
	case file.IsNew:
		return nil, errors.New("diff creates a file; use file_create")
	case file.IsDelete:
		return nil, errors.New("diff deletes a file; use file_delete")
	case file.IsBinary || file.BinaryFragment != nil:
		return nil, errors.New("binary diff")
	case len(file.TextFragments) == 0:
		return nil, errors.New("diff has no hunks")
	}
	var buf bytes.Buffer
	if err := gitdiff.Apply(&buf, bytes.NewReader(content), file); err != nil {
		return nil, fmt.Errorf("hunk does not match: %w", err)
	}
	return buf.Bytes(), nil
}

func normalizeDiff(relPath, diff string) (string, error) {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return "", errors.New("diff is required")
	}
	gitFiles := 0
	for line := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			gitFiles++
		}
	}
	if gitFiles > 1 {
		return "", errors.New("one file per call")
	}
	if !strings.HasPrefix(diff, "@@") && !strings.Contains(diff, "\n@@") {
		return "", errors.New("diff has no hunks")
	}
	if !strings.Contains(diff, "--- ") {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" {
			relPath = "file"
		}
		diff = "--- a/" + relPath + "\n+++ b/" + relPath + "\n" + diff + "\n"
	}
	if !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	return diff, nil
}
