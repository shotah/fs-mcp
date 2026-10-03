package tools

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/shotah/fs-mcp/server"
)

func TestToolNamesLocked(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^[a-z]+_[a-z]+`)
	names, err := ToolNames(TierCore)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 5 {
		t.Fatalf("core len = %d, want 5: %v", len(names), names)
	}
	want := map[string]bool{
		ToolFileList: true, ToolFileGet: true, ToolFileSearch: true,
		ToolFileCreate: true, ToolFilePatch: true,
	}
	for _, name := range names {
		if !re.MatchString(name) {
			t.Errorf("tool %q does not match ^[a-z]+_[a-z]+", name)
		}
		if strings.HasPrefix(name, server.ServerName) {
			t.Errorf("tool %q starts with server id %s", name, server.ServerName)
		}
		if !want[name] {
			t.Errorf("unexpected tool %q", name)
		}
		delete(want, name)
	}
	for missing := range want {
		t.Errorf("missing tool %q", missing)
	}
}

func TestWriteTierAddsDeleteOnly(t *testing.T) {
	t.Parallel()
	names, err := ToolNames(TierWrite)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 6 || names[5] != ToolFileDelete {
		t.Fatalf("write tier = %v", names)
	}
	if _, err := ToolNames("complete"); err == nil {
		t.Fatal("expected unknown tier to fail")
	}
}

func TestRegisteredDescriptions(t *testing.T) {
	t.Parallel()
	s := server.New()
	if _, err := Register(s, t.TempDir(), TierCore); err != nil {
		t.Fatal(err)
	}
	got := listTools(t, s)
	want := map[string]string{
		ToolFileList:   descList,
		ToolFileGet:    descGet,
		ToolFileSearch: descSearch,
		ToolFileCreate: descCreate,
		ToolFilePatch:  descPatch,
	}
	if len(got) != len(want) {
		t.Fatalf("registered %d tools, want %d", len(got), len(want))
	}
	for name, prefix := range want {
		tool, ok := got[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if !strings.HasPrefix(tool, prefix) {
			t.Errorf("%s description %q does not start with %q", name, tool, prefix)
		}
	}
}

func TestCoreOmitsDelete(t *testing.T) {
	t.Parallel()
	s := server.New()
	root := t.TempDir()
	if _, err := Register(s, root, TierCore); err != nil {
		t.Fatal(err)
	}
	if _, ok := listTools(t, s)[ToolFileDelete]; ok {
		t.Fatal("core registered file_delete")
	}
}

func listTools(t *testing.T, s *mcpserver.MCPServer) map[string]string {
	t.Helper()
	resp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	result, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected JSONRPCResponse, got %T", resp)
	}
	listResult, ok := result.Result.(mcp.ListToolsResult)
	if !ok {
		t.Fatalf("expected ListToolsResult, got %T", result.Result)
	}
	names := make(map[string]string, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		names[tool.Name] = tool.Description
	}
	return names
}

func TestJail(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, "link", 0, 0); err == nil {
		t.Fatal("symlink out of the jail was readable")
	}
	if _, err := Read(root, filepath.Join("..", filepath.Base(outside), "secret"), 0, 0); err == nil {
		t.Fatal("relative escape was readable")
	}
	absOutside := filepath.Join(outside, "secret")
	if _, err := Read(root, absOutside, 0, 0); err == nil {
		t.Fatal("absolute escape was readable")
	}
	if err := os.WriteFile(filepath.Join(root, "c.txt"), []byte("see"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, "/c.txt", 0, 0)
	if err != nil || got != "see" {
		t.Fatalf("leading slash = %q %v", got, err)
	}
	got, err = Read(root, filepath.Join(root, "c.txt"), 0, 0)
	if err != nil || got != "see" {
		t.Fatalf("absolute inside root = %q %v", got, err)
	}
	if _, err := Read(root, "/../c.txt", 0, 0); err == nil {
		t.Fatal("leading slash with .. was readable")
	}
}

func TestReadListSearchCreatePatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "a.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "nested.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	listed, err := List(root, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Entries) != 2 || listed.Entries[0].Name != "a.txt" || listed.Entries[1].Name != "nested.go" {
		t.Fatalf("list = %+v", listed)
	}

	body, err := Read(root, "sub/a.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if body != "alpha\nbeta\n" {
		t.Fatalf("read = %q", body)
	}
	window, err := Read(root, "sub/a.txt", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(window, "range: 2-2 of 2\n") || !strings.Contains(window, "beta") {
		t.Fatalf("window = %q", window)
	}

	hits, err := Search(context.Background(), root, ".", "beta", "*.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits.Hits) != 1 || hits.Hits[0].Line != 2 || hits.Hits[0].Path != "sub/a.txt" {
		t.Fatalf("hits = %+v", hits)
	}

	msg, err := Create(root, "new/file.txt", "hello\n")
	if err != nil {
		t.Fatal(err)
	}
	if msg != "created: new/file.txt" {
		t.Fatalf("create = %q", msg)
	}
	if _, err := Create(root, "new/file.txt", "again"); err == nil {
		t.Fatal("create overwrote an existing file")
	}

	patched, err := Patch(root, "sub/a.txt", "@@ -2,1 +2,1 @@\n-beta\n+BETA\n")
	if err != nil {
		t.Fatal(err)
	}
	if patched != "patched: sub/a.txt\n1: alpha\n2: BETA" {
		t.Fatalf("patch = %q", patched)
	}
	after, err := os.ReadFile(filepath.Join(root, "sub", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "alpha\nBETA\n" {
		t.Fatalf("after patch = %q", after)
	}
	if _, err := Patch(root, "sub/a.txt", "@@ -2,1 +2,1 @@\n-beta\n+nope\n"); err == nil {
		t.Fatal("mismatched hunk wrote the file")
	}
	still, err := os.ReadFile(filepath.Join(root, "sub", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(still) != "alpha\nBETA\n" {
		t.Fatalf("mismatch changed the file to %q", still)
	}

	two := "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-a\n+b\ndiff --git a/b b/b\n--- a/b\n+++ b/b\n@@ -1 +1 @@\n-a\n+b\n"
	if _, err := Patch(root, "sub/a.txt", two); err == nil {
		t.Fatal("multi-file diff was accepted")
	}
}

func TestDeleteIsWriteTier(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gone.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg, err := Delete(root, "gone.txt")
	if err != nil {
		t.Fatal(err)
	}
	if msg != "deleted: gone.txt" {
		t.Fatalf("delete = %q", msg)
	}
	if _, err := Delete(root, "*.txt"); err == nil {
		t.Fatal("glob delete was accepted")
	}
}

func TestBinaryMarker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, "bin.dat", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "binary file: bin.dat" {
		t.Fatalf("binary = %q", got)
	}
}
