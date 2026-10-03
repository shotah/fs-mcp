package tools

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// ApplyUnified applies a one-file unified diff to content.
// A diff that is only hunks is given a synthetic header so the parser accepts it.
// Hunk @@ line counts are taken from the body, so a miscounted header still applies.
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
	return recountHunkHeaders(diff), nil
}

// recountHunkHeaders rewrites each @@ old,new count from the hunk body.
// A short count makes go-gitdiff stop on context and report "fragment contains no changes".
// A body line that is not part of a hunk leaves that header alone so the parser still rejects it.
func recountHunkHeaders(diff string) string {
	trailingNL := strings.HasSuffix(diff, "\n")
	lines := strings.Split(diff, "\n")
	if trailingNL {
		lines = lines[:len(lines)-1]
	}
	for i := 0; i < len(lines); i++ {
		oldStart, newStart, rest, ok := parseHunkHeader(lines[i])
		if !ok {
			continue
		}
		oldN, newN := 0, 0
		j := i + 1
		valid := true
		for ; j < len(lines); j++ {
			if _, _, _, header := parseHunkHeader(lines[j]); header {
				break
			}
			oldInc, newInc, body := hunkLineCounts(lines[j])
			if !body {
				valid = false
				break
			}
			oldN += oldInc
			newN += newInc
		}
		if !valid {
			continue
		}
		lines[i] = fmt.Sprintf("@@ -%d,%d +%d,%d @@%s", oldStart, oldN, newStart, newN, rest)
		i = j - 1
	}
	out := strings.Join(lines, "\n")
	if trailingNL {
		out += "\n"
	}
	return out
}

func parseHunkHeader(line string) (oldStart, newStart int64, rest string, ok bool) {
	const startMark = "@@ -"
	const endMark = " @@"
	if !strings.HasPrefix(line, startMark) {
		return 0, 0, "", false
	}
	end := strings.Index(line, endMark)
	if end < len(startMark) {
		return 0, 0, "", false
	}
	header := line[len(startMark):end]
	rest = line[end+len(endMark):]
	ranges := strings.Split(header, " +")
	if len(ranges) != 2 {
		return 0, 0, "", false
	}
	var err error
	if oldStart, err = rangeStart(ranges[0]); err != nil {
		return 0, 0, "", false
	}
	if newStart, err = rangeStart(ranges[1]); err != nil {
		return 0, 0, "", false
	}
	return oldStart, newStart, rest, true
}

func rangeStart(s string) (int64, error) {
	part, _, _ := strings.Cut(s, ",")
	return strconv.ParseInt(part, 10, 64)
}

// hunkLineCounts reports how many old and new lines a hunk body line contributes.
// ok is false when the line is not a unified-diff body line.
func hunkLineCounts(line string) (oldInc, newInc int, ok bool) {
	if line == "" {
		return 1, 1, true
	}
	switch line[0] {
	case ' ':
		return 1, 1, true
	case '-':
		return 1, 0, true
	case '+':
		return 0, 1, true
	case '\\':
		// "\ No newline at end of file" is not part of the @@ counts.
		if len(line) >= 12 && strings.HasPrefix(line, `\ `) {
			return 0, 0, true
		}
		return 0, 0, false
	default:
		return 0, 0, false
	}
}
