package tools

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	noteTrim = "matched after trimming trailing space"
	noteFold = "matched after trimming trailing space and folding quotes and dashes"
)

// locateOld finds old in content. Exact hits win: every hit when count is 0,
// or exactly count. A positive exact count that is not count is an error.
// Otherwise each later level must hit exactly once, or count when count is set.
func locateOld(rel, content, old string, count int) ([][2]int, string, error) {
	exact := indexSpans(content, old)
	if len(exact) > 0 {
		if count == 0 || len(exact) == count {
			return exact, "", nil
		}
		return nil, "", fmt.Errorf("found %d matches for old in %s, want %d; nothing written", len(exact), rel, count)
	}
	target := 1
	if count > 0 {
		target = count
	}
	for _, level := range []struct {
		fold bool
		note string
	}{
		{false, noteTrim},
		{true, noteFold},
	} {
		spans := normSpans(content, old, level.fold)
		if len(spans) == target {
			return spans, level.note, nil
		}
	}
	return nil, "", nearestMiss(rel, content, old)
}

func indexSpans(content, old string) [][2]int {
	if old == "" {
		return nil
	}
	var spans [][2]int
	from := 0
	for {
		i := strings.Index(content[from:], old)
		if i < 0 {
			return spans
		}
		i += from
		spans = append(spans, [2]int{i, i + len(old)})
		from = i + len(old)
	}
}

func normSpans(content, old string, fold bool) [][2]int {
	nc := normalize(content, fold)
	no := normalize(old, fold)
	if no.text == "" {
		return nil
	}
	var spans [][2]int
	from := 0
	for from <= len(nc.text)-len(no.text) {
		i := strings.Index(nc.text[from:], no.text)
		if i < 0 {
			return spans
		}
		i += from
		span := [2]int{nc.start[i], nc.end[i+len(no.text)-1]}
		if span[1] < span[0] {
			return nil
		}
		if len(spans) > 0 && span[0] < spans[len(spans)-1][1] {
			return nil
		}
		spans = append(spans, span)
		from = i + len(no.text)
	}
	return spans
}

type normText struct {
	text  string
	start []int
	end   []int
}

func normalize(s string, fold bool) normText {
	var b strings.Builder
	start := make([]int, 0, len(s))
	end := make([]int, 0, len(s))
	for i := 0; i < len(s); {
		nl := strings.IndexByte(s[i:], '\n')
		lineEnd := len(s)
		if nl >= 0 {
			lineEnd = i + nl
		}
		trimEnd := trimRightSpace(s[i:lineEnd])
		emitRunes(&b, &start, &end, s[i:i+trimEnd], i, fold)
		if nl >= 0 {
			b.WriteByte('\n')
			start = append(start, i+trimEnd)
			end = append(end, lineEnd+1)
			i = lineEnd + 1
			continue
		}
		i = lineEnd
	}
	return normText{text: b.String(), start: start, end: end}
}

func trimRightSpace(line string) int {
	trimEnd := len(line)
	for trimEnd > 0 {
		r, size := utf8.DecodeLastRuneInString(line[:trimEnd])
		if !unicode.IsSpace(r) {
			break
		}
		trimEnd -= size
	}
	return trimEnd
}

func emitRunes(b *strings.Builder, start, end *[]int, line string, base int, fold bool) {
	for j := 0; j < len(line); {
		r, size := utf8.DecodeRuneInString(line[j:])
		origStart := base + j
		origEnd := origStart + size
		out := string(r)
		if fold {
			out = foldRune(r)
		}
		for k := range len(out) {
			b.WriteByte(out[k])
			*start = append(*start, origStart)
			*end = append(*end, origEnd)
		}
		j += size
	}
}

func foldRune(r rune) string {
	switch r {
	case '\u2018', '\u2019':
		return "'"
	case '\u201c', '\u201d':
		return `"`
	case '\u2013', '\u2014':
		return "-"
	case '\u00a0':
		return " "
	default:
		return string(r)
	}
}

func applyByteSpans(content, newText string, spans [][2]int) (string, [][2]int) {
	var b strings.Builder
	lineSpans := make([][2]int, 0, len(spans))
	line := 0
	prev := 0
	for _, sp := range spans {
		if sp[0] < prev || sp[1] < sp[0] || sp[1] > len(content) {
			continue
		}
		chunk := content[prev:sp[0]]
		b.WriteString(chunk)
		line += strings.Count(chunk, "\n")
		start := line
		nl := strings.Count(newText, "\n")
		end := start
		if newText != "" {
			end = start + nl
			if !strings.HasSuffix(newText, "\n") {
				end++
			}
		}
		lineSpans = append(lineSpans, [2]int{start, end})
		b.WriteString(newText)
		line = start + nl
		prev = sp[1]
	}
	b.WriteString(content[prev:])
	return b.String(), lineSpans
}

func nearestMiss(rel, content, old string) error {
	first, _, _ := strings.Cut(old, "\n")
	want := strings.TrimRightFunc(first, unicode.IsSpace)
	wantRunes := utf8.RuneCountInString(want)
	wantFold := foldLine(want)
	lines := splitLines([]byte(content))
	bestIdx := -1
	bestPrefix := 0
	for i, line := range lines {
		n := commonPrefixRunes(wantFold, foldLine(strings.TrimRightFunc(line, unicode.IsSpace)))
		if n > bestPrefix {
			bestPrefix = n
			bestIdx = i
		}
	}
	base := fmt.Sprintf("0 matches for old in %s; nothing written", rel)
	if wantRunes == 0 || bestIdx < 0 || bestPrefix*2 < wantRunes {
		return fmt.Errorf("%s. No line shares half of the first line of old", base)
	}
	fileLine := strings.TrimRightFunc(lines[bestIdx], unicode.IsSpace)
	pos, oldEnded, fileEnded, oldR, fileR := firstRuneDiff(want, fileLine)
	shown := clipRunes(lines[bestIdx])
	if oldEnded && fileEnded {
		return fmt.Errorf("%s. Nearest: line %d: %q (first line matches)", base, bestIdx+1, shown)
	}
	return fmt.Errorf("%s. Nearest: line %d: %q (differs at char %d: old has %s, file has %s)",
		base, bestIdx+1, shown, pos, runeLabel(oldR, oldEnded), runeLabel(fileR, fileEnded))
}

func foldLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteString(foldRune(r))
	}
	return b.String()
}

func commonPrefixRunes(a, b string) int {
	n := 0
	for a != "" && b != "" {
		ra, sa := utf8.DecodeRuneInString(a)
		rb, sb := utf8.DecodeRuneInString(b)
		if ra != rb {
			return n
		}
		n++
		a = a[sa:]
		b = b[sb:]
	}
	return n
}

func firstRuneDiff(a, b string) (pos int, aEnded, bEnded bool, ar, br rune) {
	pos = 1
	for a != "" && b != "" {
		ra, sa := utf8.DecodeRuneInString(a)
		rb, sb := utf8.DecodeRuneInString(b)
		if ra != rb {
			return pos, false, false, ra, rb
		}
		pos++
		a = a[sa:]
		b = b[sb:]
	}
	if a != "" {
		ra, _ := utf8.DecodeRuneInString(a)
		return pos, false, true, ra, 0
	}
	if b != "" {
		rb, _ := utf8.DecodeRuneInString(b)
		return pos, true, false, 0, rb
	}
	return pos, true, true, 0, 0
}

func runeLabel(r rune, ended bool) string {
	if ended {
		return "end"
	}
	return fmt.Sprintf("%q", string(r))
}

func lineCount(content string) int {
	if content == "" {
		return 0
	}
	n := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		n++
	}
	return n
}

func lineOffset(content string, n int) int {
	off := 0
	for range n {
		j := strings.IndexByte(content[off:], '\n')
		if j < 0 {
			return len(content)
		}
		off += j + 1
	}
	return off
}

func insertAfter(content string, after int, newText string) (string, [][2]int, error) {
	last := lineCount(content)
	if after < 0 || after > last {
		return "", nil, fmt.Errorf("after_line %d is past end (last line %d)", after, last)
	}
	at := lineOffset(content, after)
	head, tail := content[:at], content[at:]
	prefixed := head != "" && !strings.HasSuffix(head, "\n")
	block := newText
	if prefixed {
		block = "\n" + block
	}
	if !strings.HasSuffix(block, "\n") && (tail != "" || (content != "" && strings.HasSuffix(content, "\n"))) {
		block += "\n"
	}
	next := head + block + tail
	return next, [][2]int{insertedSpan(after, block, prefixed)}, nil
}

func insertedSpan(after int, block string, prefixed bool) [2]int {
	payload := block
	if prefixed {
		payload = strings.TrimPrefix(block, "\n")
	}
	if payload == "" {
		return [2]int{after, after}
	}
	nl := strings.Count(payload, "\n")
	if strings.HasSuffix(payload, "\n") {
		return [2]int{after, after + nl}
	}
	return [2]int{after, after + nl + 1}
}
