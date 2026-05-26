# GitHub tracking issue — Read→Edit ledger limit

Open this against `Musubi42/shhh` (label suggestion:
`upstream-blocked`, `known-limitation`). It is intentionally
written so that a future maintainer or Anthropic engineer
searching for `updatedOutput` / `markFileAsRead` / Read-ledger
finds it.

**Before pasting:** the body below uses absolute `Musubi42/shhh`
URLs because relative `../` paths render correctly *here* (inside
`docs/ready-to-publish/`) but would 404 once pasted into a
GitHub issue (which has no path context). The URLs assume `main`.

---

**Title**

`Read→Edit ledger limit (upstream Claude Code hook API)`

**Body**

> shhh rewrites `PreToolUse/Read.updatedInput.file_path` to a
> per-session redacted cache copy. Claude Code's internal
> Read-ledger records the rewritten path, not the original. The
> next `Edit` or `Write` on the original path fails the
> precondition check with `File has not been read yet`.
>
> **shipped resolution:** option D — documented limit, hook
> narrates a `Bash`-based workaround on every redacted Read. See
> [`docs/known-limitations.md`](https://github.com/Musubi42/shhh/blob/main/docs/known-limitations.md)
> and the permanent design record in
> [`docs/design/read-edit-tracking.md`](https://github.com/Musubi42/shhh/blob/main/docs/design/read-edit-tracking.md).
>
> **affected versions:** reproduced against Claude Code
> `2.1.150` (current at the time of filing); the limit applies to
> every Claude Code version that exposes the current hook API
> surface — no `PostToolUse.updatedOutput` for built-in tools, no
> `markFileAsRead` side-effect from `PreToolUse`.
>
> **strategies ruled out (do not redo):**
> 1. Replace tool result in `PostToolUse/Read`. Impossible:
>    `updatedMCPToolOutput` applies only to MCP tools.
> 2. Synthesize a tool result from `PreToolUse/Read`. Impossible:
>    `permissionDecision: "deny"` has no companion result-override
>    field.
> 3. Inject ledger state from any hook. Impossible: the Read
>    ledger is internal to Claude Code and not exposed.
>
> **closes when** Claude Code ships either of:
> - `PostToolUse.updatedOutput` (or symmetric to
>   `updatedMCPToolOutput`) for built-in tools, OR
> - `PreToolUse.markFileAsRead(path)` as a documented
>   side-effect.
>
> At that point flip ROADMAP items 1 and 4 back to OPEN and rip
> out the "use Bash instead" block in
> `cmd/shhh/cmdhook/read.go::narrateRedactions`.
>
> **repro:** any hook that rewrites `Read.file_path`, then tries
> to `Edit` the original path. Test 2 in
> `testdata/fixtures/hook-playground/README.md` is the minimal
> repro inside this repo.

---

After opening, paste the issue URL into
`docs/known-limitations.md` under "Affected versions" and into
`docs/design/read-edit-tracking.md` under "Feedback to Anthropic"
so the entry is bidirectionally linked.
