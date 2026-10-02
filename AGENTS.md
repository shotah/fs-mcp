# Agent notes

Naming contract: [george docs/mcp-naming.md](https://github.com/shotah/george/blob/main/docs/mcp-naming.md).
This binary's checklist is [docs/coding-mcp.md](https://github.com/shotah/george/blob/main/docs/coding-mcp.md) in the george checkout.

| Layer | Value |
| --- | --- |
| Server id | `fs` |
| Core tools | `file_list`, `file_get`, `file_search`, `file_create`, `file_patch` |
| Write tier | `file_delete` |
| Host-facing | `fs__file_get`, … |

Do not put `fs` on a tool name. Do not register a second name for the same call. Core stays at five `file_*` tools.

Do not `git init` here.
