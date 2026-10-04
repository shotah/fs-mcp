# fs-mcp

Requests come from george's live gate and sessions. The full write-ups,
with the george fixture each one is checked against, are in
[george docs/mcp_todo.md](https://github.com/shotah/george/blob/main/docs/mcp_todo.md).
A check runs from the george checkout:
`make integration-test EVAL_ARGS='-eval.n=5 -eval.only=<fixture>'` after
`rm -rf /tmp/george-eval-mcp` so it fetches the new release.

## Done

- [x] Five core tools, `--root`, `--tool-tier core|write`
- [x] Name tests: `^[a-z]+_[a-z]+`, first token is not `fs`
- [x] Jail, including symlink escape
- [x] `file_patch` is one file; a mismatch writes nothing
- [x] GitHub release archive for `george tools-fetch`
- [x] Hunk headers recounted from the body (0.0.2); `hunk does not match` shows the real lines and where the context appears elsewhere
- [x] `file_patch` `old`/`new` with optional `count`, the rename fix (0.0.3); george's `parallel_patches` 5/5
- [x] Required-argument errors show a valid call: `path is required, e.g. {"path":"a.go"}`
- [x] A patch result echoes the changed lines with numbers
- [x] Diff shape errors name the problem (wrong file, repeated file, bad `@@`, missing final newline)
- [x] A leading `/` means the root (0.0.5)
- [x] **Forgiving `old`.** Match in levels and stop at the first with exactly one hit (or `count`): exact; each line with trailing whitespace trimmed on both sides; that plus a fold of `’ ‘` → `'`, `“ ”` → `"`, `— –` → `-`, NBSP → space, on both sides. Write `new` as sent; characters outside `old` are untouched. Say which level matched in the result. No distance matching. Check: george `long_file_edit` 5/5 with no `0 matches`.
- [x] **Nearest line on a miss.** Return the line with the longest common prefix to `old`'s first line (after the fold), with its number and the first differing position: `0 matches. Nearest: line 57: "…" (differs at char 18: old has "-", file has "—")`. Under half a line in common: say there is no near line. Check: after a `0 matches`, the next call is a corrected patch, not a read.
- [x] **Insert by line.** `file_patch` `after_line` (number) with `new`. `0` is the top; the last line appends. Exactly one of `diff`, `old`, `after_line`. Keep the file's final newline. Result shows the inserted lines with numbers. Core stays at five names. Check: george `append_section` 5/5, one patch per run.
- [x] **Page by characters as well as lines.** `file_get` takes `max_chars` (default 6000); a page is at most `limit` lines and `max_chars`, cut at a line. Header: `range: 1-59 of 184; next offset 60`, and on the last page `range: 60-184 of 184; end`. Check: george's own cut never fires on a `file_get` in a full gate.
- [x] **Line numbers on every read.** Every `file_get` line is `N: text` and every read opens with the `range:` header. `file_search` stays. Check: `after_line` values in the gate match a line from a read that turn.
- [x] **Case hint on a missing file.** When exactly one entry in that directory matches case-insensitively: `no such file README.md; did you mean readme.md?`. Do not open it. Check: no george `pick_an_option` run ends on "no such file".
