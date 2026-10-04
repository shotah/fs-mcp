package tools

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

const (
	patchContext  = 2
	patchMaxLines = 40
)

// ApplyUnified applies a one-file unified diff to content.
// A diff that is only hunks is given a synthetic header so the parser accepts it.
// Hunk @@ line counts are taken from the body, so a miscounted header still applies.
// A conflict returns an error and leaves the caller to keep the original bytes.
func ApplyUnified(relPath, diff string, content []byte) ([]byte, error) {
	next, _, err := applyUnified(relPath, diff, content)
	return next, err
}

// applyUnified returns the patched bytes and the changed line spans in the
// result. Spans are half-open indexes into splitLines(next).
func applyUnified(relPath, diff string, content []byte) ([]byte, [][2]int, error) {
	normalized, err := normalizeDiff(relPath, diff)
	if err != nil {
		return nil, nil, err
	}
	files, _, err := gitdiff.Parse(strings.NewReader(normalized))
	if err != nil {
		return nil, nil, err
	}
	if len(files) != 1 {
		return nil, nil, errors.New("one file per call")
	}
	file := files[0]
	switch {
	case file.IsNew:
		return nil, nil, errors.New("diff creates a file; use file_create")
	case file.IsDelete:
		return nil, nil, errors.New("diff deletes a file; use file_delete")
	case file.IsBinary || file.BinaryFragment != nil:
		return nil, nil, errors.New("binary diff")
	case len(file.TextFragments) == 0:
		return nil, nil, errors.New("diff has no hunks")
	}
	var buf bytes.Buffer
	if err := gitdiff.Apply(&buf, bytes.NewReader(content), file); err != nil {
		return nil, nil, mismatchError(content, file, err)
	}
	return buf.Bytes(), fragmentSpans(file.TextFragments), nil
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

func fragmentSpans(frags []*gitdiff.TextFragment) [][2]int {
	spans := make([][2]int, 0, len(frags))
	for _, frag := range frags {
		start := max(int(frag.NewPosition)-1, 0)
		n := max(int(frag.NewLines), 0)
		spans = append(spans, [2]int{start, start + n})
	}
	return spans
}

func formatPatchResult(rel string, after []byte, spans [][2]int, note string) string {
	body, truncated := formatChangedLines(splitLines(after), spans)
	var b strings.Builder
	b.WriteString("patched: ")
	b.WriteString(rel)
	if note != "" {
		b.WriteString(" (")
		b.WriteString(note)
		b.WriteByte(')')
	}
	if body != "" {
		b.WriteByte('\n')
		b.WriteString(body)
	}
	if truncated {
		b.WriteString("\n(truncated)")
	}
	return b.String()
}

func formatChangedLines(lines []string, spans [][2]int) (string, bool) {
	if len(lines) == 0 || len(spans) == 0 {
		return "", false
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	type seg struct{ start, end int }
	var segs []seg
	for _, sp := range spans {
		s, e := expandSpan(sp, len(lines))
		if s >= e {
			continue
		}
		if len(segs) > 0 && s <= segs[len(segs)-1].end {
			if e > segs[len(segs)-1].end {
				segs[len(segs)-1].end = e
			}
			continue
		}
		segs = append(segs, seg{s, e})
	}
	var b strings.Builder
	n := 0
	truncated := false
	for _, seg := range segs {
		for i := seg.start; i < seg.end; i++ {
			if n == patchMaxLines {
				truncated = true
				break
			}
			if n > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%d: %s", i+1, clipRunes(lines[i]))
			n++
		}
		if truncated {
			break
		}
	}
	return b.String(), truncated
}

func expandSpan(sp [2]int, n int) (int, int) {
	s := sp[0] - patchContext
	e := sp[1] + patchContext
	if s < 0 {
		s = 0
	}
	if e > n {
		e = n
	}
	if s > e {
		s = e
	}
	return s, e
}

func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	text := string(content)
	lines := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func mismatchError(content []byte, file *gitdiff.File, err error) error {
	msg := "hunk does not match: " + err.Error()
	frag, ok := failingFragment(file, err)
	if !ok {
		return errors.New(msg)
	}
	var b strings.Builder
	b.WriteString(msg)
	appendHunkLines(&b, splitLines(content), frag)
	if hint := elsewhereHint(splitLines(content), frag); hint != "" {
		b.WriteByte('\n')
		b.WriteString(hint)
	}
	return errors.New(b.String())
}

func failingFragment(file *gitdiff.File, err error) (*gitdiff.TextFragment, bool) {
	var applyErr *gitdiff.ApplyError
	if !errors.As(err, &applyErr) || applyErr.Fragment < 1 {
		return nil, false
	}
	frags := make([]*gitdiff.TextFragment, len(file.TextFragments))
	copy(frags, file.TextFragments)
	sort.Slice(frags, func(i, j int) bool {
		return frags[i].OldPosition < frags[j].OldPosition
	})
	idx := applyErr.Fragment - 1
	if idx >= len(frags) {
		return nil, false
	}
	return frags[idx], true
}

func appendHunkLines(b *strings.Builder, lines []string, frag *gitdiff.TextFragment) {
	start, end, pastEnd := hunkRange(frag, len(lines))
	if pastEnd {
		fmt.Fprintf(b, "\nfile has %d lines", len(lines))
		return
	}
	n := 0
	for i := start; i <= end; i++ {
		if n == patchMaxLines {
			b.WriteString("\n(truncated)")
			return
		}
		fmt.Fprintf(b, "\n%d: %s", i, clipRunes(lines[i-1]))
		n++
	}
}

// hunkRange is the inclusive 1-based line window to show for a failed hunk.
// An insert (no old lines) shows the lines around the insertion point.
func hunkRange(frag *gitdiff.TextFragment, nLines int) (start, end int, pastEnd bool) {
	start = max(int(frag.OldPosition), 1)
	end = start + int(frag.OldLines) - 1
	if frag.OldLines < 1 {
		start = max(start-patchContext, 1)
		end = max(int(frag.OldPosition)+patchContext, start)
	}
	if start > nLines {
		return start, start, true
	}
	end = min(end, nLines)
	return start, end, false
}

func elsewhereHint(lines []string, frag *gitdiff.TextFragment) string {
	text, ok := firstSoughtLine(frag)
	if !ok {
		return ""
	}
	start := max(int(frag.OldPosition), 1)
	end := start + int(frag.OldLines) - 1
	if frag.OldLines < 1 {
		end = start
	}
	best := 0
	bestDist := int(^uint(0) >> 1)
	for i, line := range lines {
		n := i + 1
		if n >= start && n <= end {
			continue
		}
		if line != text {
			continue
		}
		dist := n - start
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			best = n
		}
	}
	if best == 0 {
		return ""
	}
	return fmt.Sprintf("%q is on line %d", clipRunes(text), best)
}

func firstSoughtLine(frag *gitdiff.TextFragment) (string, bool) {
	var deleted string
	haveDeleted := false
	for _, line := range frag.Lines {
		if line.Op == gitdiff.OpContext {
			text := strings.TrimRight(line.Line, "\n")
			if text == "" {
				continue
			}
			return text, true
		}
		if line.Op == gitdiff.OpDelete && !haveDeleted {
			deleted = strings.TrimRight(line.Line, "\n")
			haveDeleted = deleted != ""
		}
	}
	if !haveDeleted {
		return "", false
	}
	return deleted, true
}
