package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/shotah/fs-mcp/server"
)

func TestCleanRoot(t *testing.T) {
	t.Parallel()
	if _, err := CleanRoot(""); err == nil {
		t.Fatal("empty root accepted")
	}
	if _, err := CleanRoot(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing root accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CleanRoot(file); err == nil {
		t.Fatal("file root accepted")
	}
	got, err := CleanRoot(t.TempDir())
	if err != nil || got == "" {
		t.Fatalf("CleanRoot: %v %q", err, got)
	}
}

func TestListAndReadEdges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("note.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	listed, err := List(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, e := range listed.Entries {
		kinds[e.Name] = e.Kind
	}
	if kinds["note.txt"] != "file" || kinds["link"] != "symlink" || kinds["pipe"] != "other" {
		t.Fatalf("kinds = %v", kinds)
	}
	if _, err := List(root, "note.txt"); err == nil {
		t.Fatal("listed a file")
	}
	body, err := Read(root, "note.txt", 0, 0, 0)
	if err != nil || body != "range: 1-1 of 1; end\n1: alpha" {
		t.Fatalf("read = %q %v", body, err)
	}
	empty, err := Read(root, "empty.txt", 0, 0, 0)
	if err != nil || empty != "range: 0-0 of 0; end" {
		t.Fatalf("empty = %q %v", empty, err)
	}
	if _, err := Read(root, ".", 0, 0, 0); err == nil {
		t.Fatal("read a directory")
	}
	if _, err := Read(root, "note.txt", 9, 1, 0); err == nil || !strings.Contains(err.Error(), "last line 1") {
		t.Fatal("offset past end was accepted")
	}
	if _, err := Read(root, "a\x00b", 0, 0, 0); err == nil {
		t.Fatal("nul path was accepted")
	}
	if _, err := ToolNames(""); err != nil {
		t.Fatal(err)
	}
}

func TestSearchAndPatchEdges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "hidden.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", maxHitRunes+20) + "needle"
	if err := os.WriteFile(filepath.Join(root, "one.txt"), []byte(long+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "two.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{'n', 'e', 0, 'd'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Search(context.Background(), root, ".", "  ", "", 0); err == nil {
		t.Fatal("blank query accepted")
	}
	if _, err := Search(context.Background(), root, ".", "needle", "[", 0); err == nil {
		t.Fatal("bad glob accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(ctx, root, ".", "needle", "", 0); err == nil {
		t.Fatal("canceled search succeeded")
	}
	hits, err := Search(context.Background(), root, ".", "needle", "*.txt", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits.Hits) != 1 || !hits.Truncated {
		t.Fatalf("hits = %+v", hits)
	}
	if len([]rune(hits.Hits[0].Text)) > maxHitRunes {
		t.Fatalf("hit text was not clipped: %d", len(hits.Hits[0].Text))
	}
	if _, err := Patch(root, "bin.dat", "@@ -1 +1 @@\n-a\n+b\n"); err == nil {
		t.Fatal("patched a binary file")
	}
	if _, err := ApplyUnified("f", "", []byte("a")); err == nil {
		t.Fatal("empty diff accepted")
	}
	if _, err := ApplyUnified("f", "--- a/f\n+++ b/f\n", []byte("a")); err == nil {
		t.Fatal("diff without hunks accepted")
	}
	if _, err := ApplyUnified("f", "--- /dev/null\n+++ b/f\n@@ -0,0 +1 @@\n+hi\n", nil); err == nil {
		t.Fatal("create diff accepted")
	}
	if _, err := ApplyUnified("f", "--- a/f\n+++ /dev/null\n@@ -1 +0,0 @@\n-a\n", []byte("a\n")); err == nil {
		t.Fatal("delete diff accepted")
	}
	next, err := ApplyUnified("f", "@@ -1 +1 @@\r\n-a\r\n+b\r\n", []byte("a\n"))
	if err != nil || string(next) != "b\n" {
		t.Fatalf("crlf apply = %q %v", next, err)
	}
}

func TestDeleteEdges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(root, "dir"); err == nil {
		t.Fatal("deleted a directory")
	}
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	msg, err := Delete(root, "link")
	if err != nil || msg != "deleted: real.txt" {
		t.Fatalf("delete link = %q %v", msg, err)
	}
	if _, err := os.Stat(filepath.Join(root, "real.txt")); !os.IsNotExist(err) {
		t.Fatal("symlink path did not remove the target inside the jail")
	}
}

func TestToolHandlers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := server.New()
	if _, err := Register(s, root, TierWrite); err != nil {
		t.Fatal(err)
	}
	listed, isErr := callTool(t, s, ToolFileList, map[string]any{"path": "."})
	if isErr || !strings.Contains(listed, "a.txt") {
		t.Fatalf("list = %q err=%v", listed, isErr)
	}
	got, isErr := callTool(t, s, ToolFileGet, map[string]any{"path": "a.txt", "offset": 1, "limit": 1})
	if isErr || !strings.Contains(got, "alpha") {
		t.Fatalf("get = %q err=%v", got, isErr)
	}
	missing, isErr := callTool(t, s, ToolFileGet, map[string]any{})
	if !isErr || !strings.Contains(missing, "path") {
		t.Fatalf("get missing = %q err=%v", missing, isErr)
	}
	found, isErr := callTool(t, s, ToolFileSearch, map[string]any{"query": "alpha", "glob": "*.txt"})
	if isErr || !strings.Contains(found, "a.txt") {
		t.Fatalf("search = %q err=%v", found, isErr)
	}
	blank, isErr := callTool(t, s, ToolFileSearch, map[string]any{"query": "  "})
	if !isErr || !strings.Contains(blank, "query") {
		t.Fatalf("search blank = %q err=%v", blank, isErr)
	}
	created, isErr := callTool(t, s, ToolFileCreate, map[string]any{"path": "b.txt"})
	if isErr || !strings.Contains(created, "created: b.txt") {
		t.Fatalf("create = %q err=%v", created, isErr)
	}
	_, isErr = callTool(t, s, ToolFileCreate, map[string]any{})
	if !isErr {
		t.Fatal("create without path succeeded")
	}
	patched, isErr := callTool(t, s, ToolFilePatch, map[string]any{
		"path": "a.txt",
		"diff": "@@ -1 +1 @@\n-alpha\n+ALPHA\n",
	})
	if isErr || !strings.Contains(patched, "patched: a.txt") {
		t.Fatalf("patch = %q err=%v", patched, isErr)
	}
	noDiff, isErr := callTool(t, s, ToolFilePatch, map[string]any{"path": "a.txt"})
	if !isErr || !strings.Contains(noDiff, "diff") {
		t.Fatalf("patch missing = %q err=%v", noDiff, isErr)
	}
	deleted, isErr := callTool(t, s, ToolFileDelete, map[string]any{"path": "b.txt"})
	if isErr || !strings.Contains(deleted, "deleted: b.txt") {
		t.Fatalf("delete = %q err=%v", deleted, isErr)
	}
	_, isErr = callTool(t, s, ToolFileDelete, map[string]any{})
	if !isErr {
		t.Fatal("delete without path succeeded")
	}
}

func callTool(t *testing.T, s *mcpserver.MCPServer, name string, args map[string]any) (string, bool) {
	t.Helper()
	params := map[string]any{"name": name, "arguments": args}
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	resp := s.HandleMessage(context.Background(), raw)
	switch r := resp.(type) {
	case mcp.JSONRPCResponse:
		result, ok := r.Result.(*mcp.CallToolResult)
		if !ok {
			t.Fatalf("result %T", r.Result)
		}
		if len(result.Content) == 0 {
			return "", result.IsError
		}
		text, ok := result.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatalf("content %T", result.Content[0])
		}
		return text.Text, result.IsError
	case mcp.JSONRPCError:
		t.Fatalf("protocol error %d: %s", r.Error.Code, r.Error.Message)
	default:
		t.Fatalf("response %T", resp)
	}
	return "", true
}
