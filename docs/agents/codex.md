# Codex — shhh integration

**Status: ships, Bash-only coverage.** Codex CLI v0.117+ exposes a
hook system that is near-1:1 with Claude Code's. shhh's hook is
wired in and intercepts shell commands the agent runs. Coverage of
Codex's *internal* `apply_patch` and `read_file` tools is blocked
upstream: those tools do not yet fire `PreToolUse`
(track [openai/codex#18491](https://github.com/openai/codex/issues/18491)).

Until that lands, an in-place edit through `apply_patch` can hand
the model a raw secret — the `cat .env` path is covered, the
`apply_patch path/to/.env` path is not.

For the integration code, see `cmd/shhh/cmdhook/codex.go`.

## Commands available

| Command                  | What it does |
|--------------------------|--------------|
| `shhh install codex`     | Writes hook entries into `~/.codex/hooks.json`. |
| `shhh uninstall codex`   | Removes them. |

## Interception flows

### Shell command (covered)

- **Hook:** `PreToolUse` on Bash-equivalent tool.
- **Capability:** modify the command before execution, same as
  Claude Code's Bash flow.

### Internal file read / edit (not covered)

- **Hook:** would be `PreToolUse` on Codex's internal tools.
- **Status:** Codex does not currently emit these events. Out of
  shhh's control until the upstream issue resolves.

## Further reading

- Background research: `docs/dev/codex-research-2026-05-26.md`
- Upstream tracking issue: openai/codex#18491
- Known gaps: `docs/dev/known-limitations.md` §2
