# Phase 4 — Multi-tool Coverage

**Duration:** 4 weeks
**Visibility:** Public
**Prerequisite phase:** Phase 3 shipped and stable

## Goal

Ship the proxy (with rehydration) and the MCP server (with compensatory tools) to extend coverage beyond Claude Code. After Phase 4, shhh works with Cursor, Windsurf, Aider, Codex, and any CLI tool that respects `ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL`.

## Why this phase exists

The product bet is multi-tool coverage — "shhh works no matter which agent the developer uses this week." That bet is only proven once shhh actually ships integrations for more than one tool. Phase 4 is where the multi-tool story becomes real, and where the session-map-shared-across-integrations claim is either validated or retracted.

This phase also ships the MCP compensatory tools that Phase 2 decided on. Those tools are a first-class product surface — they unblock eval tasks that pure redaction cannot — and they are what makes shhh more than "a worse version of Rehydra."

## Deliverables

### HTTP reverse proxy daemon with rehydration

- [ ] Go HTTP reverse proxy listening on `127.0.0.1:8787` by default.
- [ ] Upstream routing per PRD §7.6: `/v1/anthropic/*` → `https://api.anthropic.com/*`, similar for OpenAI, Google.
- [ ] Outbound: parse `messages[]` from the request body, redact secret values via the session map (shared with the Phase 3 daemon over the Unix socket), forward the modified request upstream.
- [ ] Inbound: parse `tool_use[]` blocks from the upstream response, rehydrate placeholder values in `tool_use.input` via the session map, forward the modified response to the agent.
- [ ] Session-token authentication: every proxy request includes a session token checked against the daemon to prevent cross-user access on shared hosts.
- [ ] Handles streaming responses (Server-Sent Events) without breaking the rehydration logic on partial JSON.
- [ ] `shhh proxy start`, `shhh proxy stop`, `shhh proxy restart`, `shhh proxy status`.
- [ ] PID file at `~/.config/shhh/proxy.pid`, logs at `~/.config/shhh/logs/proxy.log`.
- [ ] Graceful shutdown: drains in-flight requests before exiting.

### MCP server (stdio JSON-RPC)

- [ ] Go MCP server implementing the JSON-RPC protocol over stdio.
- [ ] File-reading tools: `read_sensitive_file`, `list_sensitive_files`.
- [ ] Compensatory tools (Phase 2 ADR selects which ship first; all of these are already stubbed from Phase 0):
  - [ ] `decode_jwt_safely`
  - [ ] `compare_secrets`
  - [ ] `grep_hardcoded`
  - [ ] `explain_secret`
- [ ] Rules file templates installed alongside MCP config: `.cursor/rules/shhh.md`, `.windsurfrules`, per PRD §7.7.
- [ ] `shhh mcp-serve` command launches the server in stdio mode (invoked by the IDE, not by the user directly).

### Per-tool install commands

Each of these is idempotent, backs up any existing config, and has a corresponding `shhh uninstall <tool>` that cleanly reverts.

- [ ] `shhh install proxy` — starts the proxy daemon and updates `~/.zshrc` / `~/.bashrc` / `~/.config/fish/config.fish` with `ANTHROPIC_BASE_URL` and `OPENAI_BASE_URL` pointing at the proxy. Detects shell automatically.
- [ ] `shhh install cursor` — writes `.cursor/mcp.json` with the `shhh` server and `.cursor/rules/shhh.md`.
- [ ] `shhh install windsurf` — writes the Windsurf MCP config and `.windsurfrules`.
- [ ] `shhh install aider` — configures Aider to use the proxy via `BASE_URL` env vars in its config file.
- [ ] `shhh install codex` — writes `AGENTS.md` instructions and configures the Codex hook if available.

### Updated commands from Phase 3

- [ ] `shhh status` — now reports: daemon state, proxy state, hook state per installed tool, MCP state per installed tool, interception counts per layer.
- [ ] `shhh doctor` — expanded with: proxy reachability, MCP server binary hash verification, `BASE_URL` env variables set in shell profile, per-tool hook/config validation.
- [ ] `shhh logs` — real-time tail of interception events across all three layers (hook, proxy, MCP).

### Cross-tool eval re-run

- [ ] `shhh-eval` harness extended with Cursor-in-Docker and Aider-in-Docker runners.
- [ ] Full 10-task eval re-run across three integrations: Claude Code (hook), Cursor (MCP), Aider (proxy).
- [ ] Results published in `docs/eval-results-multi-tool.md` with per-integration breakdowns.
- [ ] **Cross-tool consistency test:** a session where the agent starts in Claude Code, and a second agent in Cursor reads the same `.env`, verifies that `compare_secrets(placeholder_from_claude_code, placeholder_from_cursor)` returns true. This is the test that validates the shared session map claim. If it fails, ADR on the Phase 2 session-map-scope decision is revisited.

### Compatibility matrix published

- [ ] `docs/compatibility.md` — for each supported tool, list: integration method (hook / proxy / MCP), which eval tasks pass, which fail, and any known limitations.
- [ ] Link this matrix prominently from the README.

## Ship criterion

Three things must work end-to-end on a fresh machine:

1. **Proxy smoke test with Aider.** Install shhh, run `shhh install aider`, run Aider in a repo with a `.env` containing a real secret, verify the secret never appears in outbound API requests (captured via a local capture script) and the agent can still execute commands that use the secret via rehydration.

2. **MCP smoke test with Cursor.** Install shhh, run `shhh install cursor`, restart Cursor, ask the agent to "summarize the .env file" via the MCP tool, verify the agent sees placeholders and can still use `compare_secrets` and `decode_jwt_safely`.

3. **Cross-tool consistency test.** The test from the deliverables list — same session, two agents, shared session map.

If any of the three fails, Phase 4 is not shipped.

## Explicitly out of scope for Phase 4

- `shhh init` TUI installer. Phase 5.
- Auto-detection of installed tools. Phase 5.
- `shhh add-rule`, `shhh ignore`, `shhh allow` customization commands. Phase 5.
- Team configuration features. Phase 6.
- VS Code extension. Phase 6.
- CI mode (`shhh scan --ci`). Phase 6.
- Any integration with secret managers (1Password, Vault). Phase 6 at earliest.
- Support for tools that route exclusively through vendor servers (Cursor agent mode direct-to-Cursor-servers). These remain in the "hook only" column of the compatibility matrix.

## Phase-specific risks

| Risk | Severity | Response |
|------|----------|----------|
| Streaming responses with partial JSON break the rehydration parser | High | Use a streaming-aware JSON parser. Write specific tests for partial responses. A crash here kills the entire agent session. |
| Cursor updates its MCP protocol in an incompatible way during Phase 4 | Medium | Pin to the current MCP spec. Test against the Cursor beta channel. |
| Shell profile modification breaks users' existing setup | High | Always back up the profile. Use a well-delimited `# --- shhh begin ---` / `# --- shhh end ---` block that's easy to remove. Never touch anything outside that block. |
| `BASE_URL` override silently doesn't apply on some tool versions | Medium | Verify in `shhh doctor` by making a test request and checking the proxy logs. If no request arrives at the proxy, warn loudly. |
| Rehydration in streaming responses causes tokens to leak in the non-rehydrated portion of a response | High | Rehydration happens after the full response is assembled, before delivery to the agent. Never attempt to rehydrate on partial chunks. |
| Cross-tool session-map claim fails in practice because the two tools don't share a process context | Medium | The test exists specifically to reveal this. If it fails, revise the session-map scope (per-tool instead of shared) and document the limitation. Do not ship a false claim. |
| The MCP server binary hash in `.cursor/mcp.json` gets stale after an update | Medium | `shhh doctor` re-verifies and re-writes the hash after every `shhh upgrade`. |

## Dependencies

- Phase 3 shipped, daemon and session map stable.
- Phase 2 ADR on rehydration has committed to rehydration.
- At least one month of Phase 3 in production to surface real-world issues before broadening the surface area.
- `shhh-eval` harness extended to support multi-agent Docker runners.
