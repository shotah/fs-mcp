# fs-mcp

- [x] Five core tools, `--root`, `--tool-tier core|write`
- [x] Name tests: `^[a-z]+_[a-z]+`, first token is not `fs`
- [x] Jail, including symlink escape
- [x] `file_patch` is one file; a mismatch writes nothing
- [ ] GitHub release archive for `george tools-fetch`

- [x] **2a. Hunk header counts (owned by `fs-mcp`).** With the real `fs`, the model's diffs are rejected most runs: `gitdiff: line 6: fragment contains no changes`. It writes `@@ -1,2 +1,2 @@` over a hunk with three old lines, re-reads, and sends the same header again until the loop guard lands it. A 30B model can't count hunk lines. The fix belongs in `fs-mcp`, not here: recount hunk headers from the body before applying, or offer an exact-string replace. Check: `parallel_patches` 5/5 on the next `fs-mcp` release with no change in this repo.