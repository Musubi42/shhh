# Cursor — shhh integration

**Status: ships, with a known protocol gap.** Cursor v1.7+ native
hooks are wired. Coverage of the "read .env, see placeholders"
forcing-function scenario is currently unverified at the protocol
level — Cursor's PreToolUse payload shape for file reads has
diverged from Claude Code's in ways we have not fully mapped.

For the current state of the integration code, see
`cmd/shhh/cmdhook/cursor.go` and
`cmd/shhh/cmdinstall/cursor_settings.go`.

## Commands available

| Command                    | What it does |
|----------------------------|--------------|
| `shhh install cursor`      | Writes hook entries into `~/.cursor/hooks.json`. |
| `shhh uninstall cursor`    | Removes them. |

## Interception flows

The hooks installed are equivalent in name to Claude Code's
(`PreToolUse`, `PostToolUse`, `UserPromptSubmit`), but the
payload Cursor sends and the response Cursor honours differ in
ways that are still being mapped. Until that mapping is
finalized, treat Cursor coverage as **partial** and verify with
a real redaction test before trusting it.

## Further reading

- Background research: `docs/dev/cursor-research-2026-05-27.md`
- Upstream docs: `developers.cursor.com/docs/hooks`
- Known gaps: `docs/dev/known-limitations.md`
