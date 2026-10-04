package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/fs-mcp/server"
)

func TestForgivingOld(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "t.md")
	file := "Use `make build` — it’s fast.\n"
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Replace(root, "t.md", "Use `make build` - it's fast.", "Use `make build` - it's quick.", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, noteFold) {
		t.Fatalf("fold result = %q", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "Use `make build` - it's quick.\n" {
		t.Fatalf("folded write = %q", after)
	}

	spaced := "hello \nworld\n"
	if err := os.WriteFile(path, []byte(spaced), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = Replace(root, "t.md", "hello\n", "hi\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, noteTrim) || strings.Contains(got, "folding") {
		t.Fatalf("trim result = %q", got)
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "hi\nworld\n" {
		t.Fatalf("trim write = %q", after)
	}

	twice := "hello \nhello \n"
	if err := os.WriteFile(path, []byte(twice), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Replace(root, "t.md", "hello\n", "hi\n", 0); err == nil {
		t.Fatal("ambiguous trim wrote the file")
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != twice {
		t.Fatalf("ambiguous trim changed the file to %q", again)
	}
	if _, err := Replace(root, "t.md", "hello\n", "hi\n", 2); err != nil {
		t.Fatal(err)
	}
	counted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(counted) != "hi\nhi\n" {
		t.Fatalf("count 2 = %q", counted)
	}

	if err := os.WriteFile(path, []byte("Hello\nHello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = Replace(root, "t.md", "Hello", "Greet", 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "matched after") {
		t.Fatalf("exact replace noted a fuzzy level: %q", got)
	}
	exact, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(exact) != "Greet\nGreet\n" {
		t.Fatalf("exact replace-all = %q", exact)
	}
}

func TestNearestLine(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	body := "intro\nUse `make build` — it's slow.\noutro\n"
	if err := os.WriteFile(filepath.Join(root, "t.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Replace(root, "t.md", "Use `make build` - it's fast.", "nope", 0)
	if err == nil {
		t.Fatal("nearest miss wrote the file")
	}
	msg := err.Error()
	if !strings.Contains(msg, "0 matches") || !strings.Contains(msg, "nothing written") {
		t.Fatalf("miss = %q", msg)
	}
	if !strings.Contains(msg, `Nearest: line 2:`) || !strings.Contains(msg, `differs at char 18: old has "-", file has "—"`) {
		t.Fatalf("nearest = %q", msg)
	}
	still, err := os.ReadFile(filepath.Join(root, "t.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(still) != body {
		t.Fatalf("nearest changed the file to %q", still)
	}

	if err := os.WriteFile(filepath.Join(root, "t.md"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Replace(root, "t.md", "completely different text", "x", 0)
	if err == nil || !strings.Contains(err.Error(), "No line shares half of the first line of old") {
		t.Fatalf("far miss = %v", err)
	}
}

func TestInsertAfterLine(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Insert(root, "a.txt", 1, "world"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello\nworld" {
		t.Fatalf("append no nl = %q", got)
	}

	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Insert(root, "a.txt", 1, "world"); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello\nworld\n" {
		t.Fatalf("append with nl = %q", got)
	}

	if err := os.WriteFile(path, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg, err := Insert(root, "a.txt", 1, "X")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "2: X") {
		t.Fatalf("insert result = %q", msg)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nX\nb\n" {
		t.Fatalf("middle = %q", got)
	}

	if _, err := Insert(root, "a.txt", 0, "top"); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "top\na\nX\nb\n" {
		t.Fatalf("top = %q", got)
	}

	if _, err := Insert(root, "a.txt", 9, "nope"); err == nil || !strings.Contains(err.Error(), "past end (last line 4)") {
		t.Fatalf("past end = %v", err)
	}

	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Insert(root, "a.txt", 0, "world"); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Fatalf("empty file = %q", got)
	}
	if _, err := Insert(root, "a.txt", 1, "more"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Insert(root, "a.txt", 1, "nope"); err == nil || !strings.Contains(err.Error(), "last line 0") {
		t.Fatalf("empty past end = %v", err)
	}

	s := server.New()
	if _, err := Register(s, root, TierCore); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, isErr := callTool(t, s, ToolFilePatch, map[string]any{
		"path": "a.txt", "after_line": 1, "new": "section",
	})
	if isErr || !strings.Contains(text, "2: section") {
		t.Fatalf("tool insert = %q err=%v", text, isErr)
	}
	both, isErr := callTool(t, s, ToolFilePatch, map[string]any{
		"path": "a.txt", "after_line": 1, "old": "a", "new": "b",
	})
	if !isErr || !strings.Contains(both, "exactly one") {
		t.Fatalf("two modes = %q err=%v", both, isErr)
	}
}

func TestReadPaging(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var b strings.Builder
	for i := range 10 {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("aaaa")
	}
	b.WriteByte('\n')
	if err := os.WriteFile(filepath.Join(root, "p.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := Read(root, "p.txt", 1, 10, 56)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) > 56 {
		t.Fatalf("page is %d bytes: %q", len(page), page)
	}
	if page != "range: 1-3 of 10; next offset 4\n1: aaaa\n2: aaaa\n3: aaaa" {
		t.Fatalf("page = %q", page)
	}
	last, err := Read(root, "p.txt", 10, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if last != "range: 10-10 of 10; end\n10: aaaa" {
		t.Fatalf("last = %q", last)
	}

	long := strings.Repeat("x", 7000)
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	clipped, err := Read(root, "long.txt", 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(clipped) > defaultMaxChars {
		t.Fatalf("clipped len %d", len(clipped))
	}
	if !strings.HasPrefix(clipped, "range: 1-1 of 1; end\n1: ") {
		t.Fatalf("clipped = %q", clipped[:40])
	}

	if _, err := Read(root, "p.txt", 200, 1, 0); err == nil || !strings.Contains(err.Error(), "last line 10") {
		t.Fatalf("past end = %v", err)
	}
}

func TestCaseHint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	secret := "secret body"
	if err := os.WriteFile(filepath.Join(root, "readme.md"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(root, "README.md", 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "no such file README.md; did you mean readme.md?") || strings.Contains(err.Error(), secret) {
		t.Fatalf("hint = %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "Readme.md"), []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Read(root, "README.md", 0, 0, 0)
	if err == nil || err.Error() != "no such file README.md" || strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("ambiguous = %v", err)
	}

	msg, err := Create(root, "README.md", "new")
	if err != nil {
		t.Fatal(err)
	}
	if msg != "created: README.md" {
		t.Fatalf("create = %q", msg)
	}
}
