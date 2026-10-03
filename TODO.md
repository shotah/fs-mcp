# fs-mcp

- [x] Five core tools, `--root`, `--tool-tier core|write`
- [x] Name tests: `^[a-z]+_[a-z]+`, first token is not `fs`
- [x] Jail, including symlink escape
- [x] `file_patch` is one file; a mismatch writes nothing
- [ ] GitHub release archive for `george tools-fetch`

- [x] **2a. Hunk header counts (owned by `fs-mcp`).** With the real `fs`, the model's diffs are rejected most runs: `gitdiff: line 6: fragment contains no changes`. It writes `@@ -1,2 +1,2 @@` over a hunk with three old lines, re-reads, and sends the same header again until the loop guard lands it. A 30B model can't count hunk lines. The fix belongs in `fs-mcp`, not here: recount hunk headers from the body before applying, or offer an exact-string replace. Check: `parallel_patches` 5/5 on the next `fs-mcp` release with no change in this repo.

- [x] **2b. Replace-all in `fs-mcp` (owned by `fs-mcp`).** With the headers fixed, the remaining `parallel_patches` miss is the rename itself. The model renames `func TestHello` but leaves `if Hello("bob")` as a context line. Contract lines (every line with the old name is a `-`/`+` pair) fixed it in 2 of 3 runs once, then not; a concrete `Load → Open` diff example made it worse (0/3) and was reverted. A tool that replaces every occurrence of an exact string in one file (old, new, optional count) makes a rename one call per file with nothing to track. Check: `parallel_patches` 5/5 with that tool and no contract change. Shipped as `file_patch` `old`/`new` in `fs-mcp` 0.0.3; `qwen3.6:35b-a3b-coding` uses it for the rename, 3/3 at `-eval.n=3` and 4/5 at `-eval.n=5`; the one miss was a call without `new`, fixed a round later.