package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Tool names — service_verb, no server-id prefix.
// Host mcp.toml name is fs → fs__file_get, fs__file_patch, …
const (
	ToolFileList   = "file_list"
	ToolFileGet    = "file_get"
	ToolFileSearch = "file_search"
	ToolFileCreate = "file_create"
	ToolFilePatch  = "file_patch"
	ToolFileDelete = "file_delete"

	TierCore  = "core"
	TierWrite = "write"
)

// Descriptions are the first sentence the model sees. Keep these stable.
const (
	descList   = "List entries in one directory."
	descGet    = "Read one text file by path."
	descSearch = "Search file contents (grep) under a path."
	descCreate = "Create one new file."
	descPatch  = "Edit one existing file."

	pathExample  = `{"path":"a.go"}`
	queryExample = `{"query":"needle"}`
	patchExample = `{"path":"a.go","old":"Hello","new":"Greet"} or {"path":"a.go","diff":"@@ -1 +1 @@\n-old\n+new\n"}`
	descDelete   = "Delete one file by path."
)

// ToolNames is the catalog for tier. core is five tools so a tied file hint lists all of them.
func ToolNames(tier string) ([]string, error) {
	switch tier {
	case "", TierCore:
		return []string{ToolFileList, ToolFileGet, ToolFileSearch, ToolFileCreate, ToolFilePatch}, nil
	case TierWrite:
		return []string{ToolFileList, ToolFileGet, ToolFileSearch, ToolFileCreate, ToolFilePatch, ToolFileDelete}, nil
	default:
		return nil, fmt.Errorf("invalid --tool-tier %q (want core|write)", tier)
	}
}

// Register publishes the tier's tools. root is an already-cleaned jail.
func Register(s *mcpserver.MCPServer, root, tier string) (int, error) {
	names, err := ToolNames(tier)
	if err != nil {
		return 0, err
	}
	for _, name := range names {
		switch name {
		case ToolFileList:
			s.AddTool(mcp.NewTool(name,
				mcp.WithDescription(descList+" path is relative to the workspace root. One level, not recursive."),
				mcp.WithString("path", mcp.Description("Directory to list. Empty lists the workspace root.")),
				mcp.WithReadOnlyHintAnnotation(true),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return list(root, req)
			})
		case ToolFileGet:
			s.AddTool(mcp.NewTool(name,
				mcp.WithDescription(descGet+" offset is a 1-based line number. A binary file returns a one-line marker."),
				mcp.WithString("path", mcp.Required(), mcp.Description("File to read, relative to the workspace root.")),
				mcp.WithNumber("offset", mcp.Description("First line to return, 1-based. Default 1.")),
				mcp.WithNumber("limit", mcp.Description("Maximum lines. Default 200, max 2000.")),
				mcp.WithReadOnlyHintAnnotation(true),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return get(root, req)
			})
		case ToolFileSearch:
			s.AddTool(mcp.NewTool(name,
				mcp.WithDescription(descSearch+" Returns path, line, and text for each hit."),
				mcp.WithString("query", mcp.Required(), mcp.Description("Literal text to find. Not a regular expression.")),
				mcp.WithString("path", mcp.Description("Directory or file to search. Empty searches the workspace root.")),
				mcp.WithString("glob", mcp.Description("Optional base-name pattern, such as *.go.")),
				mcp.WithNumber("limit", mcp.Description("Maximum hits. Default 40.")),
				mcp.WithReadOnlyHintAnnotation(true),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return search(ctx, root, req)
			})
		case ToolFileCreate:
			s.AddTool(mcp.NewTool(name,
				mcp.WithDescription(descCreate+" Refuses when the path already exists."),
				mcp.WithString("path", mcp.Required(), mcp.Description("New file path, relative to the workspace root.")),
				mcp.WithString("contents", mcp.Description("File body. Empty creates an empty file.")),
				mcp.WithDestructiveHintAnnotation(true),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return create(root, req)
			})
		case ToolFilePatch:
			s.AddTool(mcp.NewTool(name,
				mcp.WithDescription(descPatch+" Send old and new to replace exact text everywhere it appears, or diff for a unified diff. Exactly one mode. Omit count to replace every match; set count to require that many. A miss writes nothing."),
				mcp.WithString("path", mcp.Required(), mcp.Description("Existing file to patch.")),
				mcp.WithString("diff", mcp.Description("Unified diff for this one file. Omit when sending old and new.")),
				mcp.WithString("old", mcp.Description("Exact text to replace, including a name that appears more than once. Omit when sending diff.")),
				mcp.WithString("new", mcp.Description("Replacement for old. Empty deletes the matched text.")),
				mcp.WithNumber("count", mcp.Description("Exact number of matches required. Omit to replace all. Any other number writes nothing.")),
				mcp.WithDestructiveHintAnnotation(true),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return patch(root, req)
			})
		case ToolFileDelete:
			s.AddTool(mcp.NewTool(name,
				mcp.WithDescription(descDelete+" One path, no globs, not a directory."),
				mcp.WithString("path", mcp.Required(), mcp.Description("File to delete.")),
				mcp.WithDestructiveHintAnnotation(true),
			), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return deleteFile(root, req)
			})
		default:
			return 0, fmt.Errorf("unknown tool %q", name)
		}
	}
	return len(names), nil
}

func list(root string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	res, err := List(root, req.GetString("path", ""))
	if err != nil {
		return toolErr(err), nil
	}
	text, err := JSON(res)
	if err != nil {
		return toolErr(err), nil
	}
	return mcp.NewToolResultText(text), nil
}

func get(root string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requirePath(req, pathExample)
	if err != nil {
		return toolErr(err), nil
	}
	text, err := Read(root, path, req.GetInt("offset", 0), req.GetInt("limit", 0))
	if err != nil {
		return toolErr(err), nil
	}
	return mcp.NewToolResultText(text), nil
}

func search(ctx context.Context, root string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil || strings.TrimSpace(query) == "" {
		return toolErr(fmt.Errorf("query is required, e.g. %s", queryExample)), nil
	}
	res, err := Search(ctx, root, req.GetString("path", ""), query, req.GetString("glob", ""), req.GetInt("limit", 0))
	if err != nil {
		return toolErr(err), nil
	}
	text, err := JSON(res)
	if err != nil {
		return toolErr(err), nil
	}
	return mcp.NewToolResultText(text), nil
}

func create(root string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requirePath(req, pathExample)
	if err != nil {
		return toolErr(err), nil
	}
	contents := ""
	if _, ok := req.GetArguments()["contents"]; ok {
		contents = req.GetString("contents", "")
	}
	text, err := Create(root, path, contents)
	if err != nil {
		return toolErr(err), nil
	}
	return mcp.NewToolResultText(text), nil
}

func patch(root string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requirePath(req, patchExample)
	if err != nil {
		return toolErr(err), nil
	}
	text, err := editFile(root, path, req)
	if err != nil {
		return toolErr(err), nil
	}
	return mcp.NewToolResultText(text), nil
}

func editFile(root, path string, req mcp.CallToolRequest) (string, error) {
	args := req.GetArguments()
	diff := req.GetString("diff", "")
	_, hasOld := args["old"]
	_, hasNew := args["new"]
	_, hasCount := args["count"]
	hasDiff := strings.TrimSpace(diff) != ""
	hasReplace := hasOld || hasNew
	if hasDiff && hasReplace {
		return "", fmt.Errorf("send exactly one of diff or old and new, e.g. %s", patchExample)
	}
	if hasCount && !hasOld {
		return "", fmt.Errorf("count is only used with old and new, e.g. %s", patchExample)
	}
	if hasDiff {
		return Patch(root, path, diff)
	}
	if !hasOld || req.GetString("old", "") == "" {
		if hasNew {
			return "", fmt.Errorf("old is required, e.g. %s", patchExample)
		}
		return "", fmt.Errorf("diff or old and new is required, e.g. %s", patchExample)
	}
	if !hasNew {
		return "", fmt.Errorf("new is required, e.g. %s", patchExample)
	}
	count := 0
	if hasCount {
		var err error
		count, err = req.RequireInt("count")
		if err != nil || count < 1 {
			return "", errors.New("count must be at least 1")
		}
	}
	return Replace(root, path, req.GetString("old", ""), req.GetString("new", ""), count)
}

func deleteFile(root string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requirePath(req, pathExample)
	if err != nil {
		return toolErr(err), nil
	}
	text, err := Delete(root, path)
	if err != nil {
		return toolErr(err), nil
	}
	return mcp.NewToolResultText(text), nil
}

func requirePath(req mcp.CallToolRequest, example string) (string, error) {
	path, err := req.RequireString("path")
	if err != nil || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required, e.g. %s", example)
	}
	return path, nil
}

func toolErr(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(err.Error())
}
