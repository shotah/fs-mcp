package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/fs-mcp/server"
)

func TestRecountHunkHeaders(t *testing.T) {
	t.Parallel()
	in := "@@ -1,2 +1,2 @@ gamma\n alpha\n beta\n-gamma\n+GAMMA\n@@ -8,1 +8,1 @@\n-old\n+new\n"
	got := recountHunkHeaders(in)
	want := "@@ -1,3 +1,3 @@ gamma\n alpha\n beta\n-gamma\n+GAMMA\n@@ -8,1 +8,1 @@\n-old\n+new\n"
	if got != want {
		t.Fatalf("recount = %q", got)
	}
	// A non-body line means the header is left for the parser to reject.
	dirty := "@@ -1,5 +1,5 @@\n-a\n+b\nnot a hunk line\n"
	if recountHunkHeaders(dirty) != dirty {
		t.Fatalf("dirty hunk was rewritten: %q", recountHunkHeaders(dirty))
	}
	noEOL := "@@ -1,9 +1,9 @@\n alpha\n-beta\n\\ No newline at end of file\n+BETA\n\\ No newline at end of file\n"
	wantEOL := "@@ -1,2 +1,2 @@\n alpha\n-beta\n\\ No newline at end of file\n+BETA\n\\ No newline at end of file\n"
	if got := recountHunkHeaders(noEOL); got != wantEOL {
		t.Fatalf("no eol recount = %q", got)
	}
}

func TestApplyMiscountedHunk(t *testing.T) {
	t.Parallel()
	content := []byte("alpha\nbeta\ngamma\n")
	// Header says two old lines; the body has three. That is the
	// "fragment contains no changes" failure from a short @@ count.
	next, err := ApplyUnified("f.txt", "@@ -1,2 +1,2 @@\n alpha\n beta\n-gamma\n+GAMMA\n", content)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "alpha\nbeta\nGAMMA\n" {
		t.Fatalf("undercount = %q", next)
	}

	next, err = ApplyUnified("f.txt", "@@ -1,8 +1,8 @@\n alpha\n-beta\n+BETA\n gamma\n", content)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "alpha\nBETA\ngamma\n" {
		t.Fatalf("overcount = %q", next)
	}

	two := []byte("a\nb\nc\nd\n")
	next, err = ApplyUnified("f.txt", "@@ -1,3 +1,3 @@\n-a\n+A\n@@ -4,2 +4,2 @@\n-d\n+D\n", two)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "A\nb\nc\nD\n" {
		t.Fatalf("two hunks = %q", next)
	}

	noEOL := []byte("alpha\nbeta")
	next, err = ApplyUnified("f.txt", "@@ -1,4 +1,4 @@\n alpha\n-beta\n\\ No newline at end of file\n+BETA\n\\ No newline at end of file\n", noEOL)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "alpha\nBETA" {
		t.Fatalf("no eol = %q", next)
	}

	if _, err := ApplyUnified("f.txt", "@@ -1,1 +1,1 @@\n alpha\n beta\n", content); err == nil {
		t.Fatal("context-only hunk accepted")
	}
	if _, err := ApplyUnified("f.txt", "@@ -1,2 +1,2 @@\n nope\n beta\n-gamma\n+GAMMA\n", content); err == nil {
		t.Fatal("mismatched context accepted")
	}
	if _, err := ApplyUnified("f.txt", "@@ -1,2 +1,2 @@\n-alpha\n+ALPHA\nnot a hunk line\n", []byte("alpha\n")); err == nil {
		t.Fatal("dirty hunk applied")
	}
}

func TestPatchResultCap(t *testing.T) {
	t.Parallel()
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = "x"
	}
	body, truncated := formatChangedLines(lines, [][2]int{{10, 70}})
	if !truncated {
		t.Fatal("expected truncation")
	}
	if got := strings.Count(body, "\n") + 1; got != patchMaxLines {
		t.Fatalf("showed %d lines", got)
	}
}

func TestPatchResultAndMismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	body := "one\ntwo\nthree\nfour\nfive\nsix\nseven\n"
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Patch(root, "a.txt", "@@ -4,1 +4,1 @@\n-four\n+FOUR\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "patched: a.txt\n2: two\n3: three\n4: FOUR\n5: five\n6: six"
	if got != want {
		t.Fatalf("result = %q", got)
	}

	if _, err := Patch(root, "a.txt", "@@ -1,1 +1,1 @@\n-seven\n+nope\n"); err == nil {
		t.Fatal("mismatch was written")
	} else if !strings.Contains(err.Error(), "1: one") || !strings.Contains(err.Error(), `"seven" is on line 7`) {
		t.Fatalf("mismatch = %q", err.Error())
	}
	still, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(still) != "one\ntwo\nthree\nFOUR\nfive\nsix\nseven\n" {
		t.Fatalf("mismatch changed the file to %q", still)
	}
}

func TestReplaceMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := "func Hello() {}\n\nfunc TestHello(t *testing.T) {\n\tif Hello(\"bob\") {}\n}\n"
	if err := os.WriteFile(filepath.Join(root, "hello_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Replace(root, "hello_test.go", "Hello", "Greet", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func Greet()") || !strings.Contains(got, "Greet(\"bob\")") {
		t.Fatalf("replace = %q", got)
	}
	after, err := os.ReadFile(filepath.Join(root, "hello_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "Hello") {
		t.Fatalf("old text remains: %q", after)
	}

	if err := os.WriteFile(filepath.Join(root, "hello_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Replace(root, "hello_test.go", "Hello", "Greet", 1); err == nil {
		t.Fatal("count miss wrote the file")
	}
	again, err := os.ReadFile(filepath.Join(root, "hello_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != src {
		t.Fatalf("count miss changed the file to %q", again)
	}
	if _, err := Replace(root, "hello_test.go", "missing", "x", 0); err == nil {
		t.Fatal("zero matches wrote the file")
	}

	s := server.New()
	if _, err := Register(s, root, TierCore); err != nil {
		t.Fatal(err)
	}
	replaced, isErr := callTool(t, s, ToolFilePatch, map[string]any{
		"path": "hello_test.go", "old": "Hello", "new": "Greet",
	})
	if isErr || !strings.Contains(replaced, "Greet(\"bob\")") {
		t.Fatalf("tool replace = %q err=%v", replaced, isErr)
	}
	both, isErr := callTool(t, s, ToolFilePatch, map[string]any{
		"path": "hello_test.go", "old": "Greet", "new": "Hello", "diff": "@@ -1 +1 @@\n-a\n+b\n",
	})
	if !isErr || !strings.Contains(both, "exactly one") {
		t.Fatalf("both modes = %q err=%v", both, isErr)
	}
	empty, isErr := callTool(t, s, ToolFilePatch, map[string]any{})
	if !isErr || !strings.Contains(empty, `{"path":"a.go","old":"Hello","new":"Greet"}`) || !strings.Contains(empty, `"diff"`) {
		t.Fatalf("empty patch = %q err=%v", empty, isErr)
	}
	noPath, isErr := callTool(t, s, ToolFileGet, map[string]any{})
	if !isErr || noPath != `path is required, e.g. {"path":"a.go"}` {
		t.Fatalf("get = %q err=%v", noPath, isErr)
	}
	noQuery, isErr := callTool(t, s, ToolFileSearch, map[string]any{})
	if !isErr || noQuery != `query is required, e.g. {"query":"needle"}` {
		t.Fatalf("search = %q err=%v", noQuery, isErr)
	}
}
