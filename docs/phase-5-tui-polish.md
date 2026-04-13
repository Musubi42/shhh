# Phase 5 — TUI Installer & Polish

**Duration:** 3 weeks
**Visibility:** Public
**Prerequisite phase:** Phase 4 shipped, multi-tool story working

## Goal

Ship `shhh init` — the interactive TUI that auto-detects installed tools, lets the user choose integrations, installs everything, runs a first scan, and completes in under 90 seconds.

## Why this phase exists

By Phase 4, shhh has every moving part: scanner, hook, proxy, MCP server, compensatory tools, daemon. But installing all of them requires knowing which tools you have, running the right `shhh install <tool>` commands in the right order, and verifying the result manually. That is too much friction for the early-adopter persona who runs `npx shhh scan .`, sees the scary output, and wants protection *right now*.

The TUI exists for that 30-second window. It's the "buy" button after the scan's "wow" moment.

This phase also handles the customization surface (`shhh add-rule`, `shhh ignore`, `shhh allow`) and the trust-on-first-use model for project-level `.shhh.toml` configs, which is a security boundary the tool needs before team adoption can happen in Phase 6.

## Deliverables

### Auto-detection

- [ ] Detector for Claude Code: `claude` binary in `PATH`, or `~/.claude/` directory exists.
- [ ] Detector for Cursor: `cursor` binary, or `~/.cursor/` directory, or `.cursor/` in the current project.
- [ ] Detector for Codex: `codex` binary in `PATH`.
- [ ] Detector for Windsurf: `windsurf` binary or config directory.
- [ ] Detector for Aider: `aider` binary in `PATH`.
- [ ] Detector returns a structured list with tool name, version (if detectable), and available integration type (hook / MCP / proxy).

### Interactive TUI (Bubble Tea)

- [ ] `shhh init` launches a full-screen TUI.
- [ ] First screen: header with logo, "scanning your system..." spinner, then the list of detected tools with checkboxes (per PRD §6.2).
- [ ] Each detected tool shows the integration type that will be used ("PreToolUse hook," "MCP + rules file," "Proxy via BASE_URL").
- [ ] Non-detected tools are shown as `⬜ Not found` for awareness.
- [ ] Arrow keys to navigate, space to toggle, enter to confirm.
- [ ] Confirmation screen shows the full list of files that will be written/modified, with a diff preview for existing files.
- [ ] Install progress screen with a checkmark per completed step.
- [ ] Post-install first scan runs automatically, showing the interception count.
- [ ] Final screen: "next steps" with a link to `shhh status`, `shhh logs`, and the docs site.
- [ ] Escape key at any point cancels cleanly — nothing is written if the user quits.

### Clean uninstall

- [ ] `shhh uninstall <tool>` — per-tool uninstall, symmetric with install. Already in Phase 3 for Claude Code; Phase 5 ensures every tool added in Phase 4 has a matching uninstall.
- [ ] `shhh uninstall --all` — removes every integration, stops the daemon, cleans up `/tmp/shhh-*`, restores shell profiles from backup.
- [ ] Uninstall is tested in CI as rigorously as install: a test that installs every integration, uninstalls every integration, and verifies the system is bit-for-bit back to its pre-shhh state (with the exception of `~/.config/shhh/backups/` which intentionally survives).
- [ ] If uninstall fails partway, it reports exactly which files are in an inconsistent state and provides a manual recovery command.

### Project-level `.shhh.toml` with trust-on-first-use

- [ ] Support for a `.shhh.toml` file at the project root that can add ignore entries, extra detection rules, and per-project configuration.
- [ ] **Trust-on-first-use:** the first time shhh encounters a `.shhh.toml` in a project it hasn't seen before, it prompts (in the terminal, not silently) with a diff of what the config would change. The user approves or rejects.
- [ ] Trust is recorded in `~/.config/shhh/trust.db` keyed by project path + content hash. If the content changes, trust is re-prompted.
- [ ] Untrusted `.shhh.toml` files are loaded in **read-only lint mode** — their settings are reported by `shhh doctor` but not applied. This means a malicious config in a cloned repo cannot disable protection silently.
- [ ] Project-level configs cannot disable protection entirely — they can only add ignores within a capped range (max 10 ignore entries, max 5 custom patterns).

### Customization commands

- [ ] `shhh add-rule --name <name> --pattern <regex>` — adds a custom detection pattern to `~/.config/shhh/config.toml`.
- [ ] `shhh ignore <file-or-path>` — adds a file or glob to the ignore list (with a prompt for reason, logged to the config).
- [ ] `shhh ignore --secret <key-name>` — ignores a specific variable name globally.
- [ ] `shhh allow <sensitive-file>` — allows Edit/Write operations on a specific sensitive file. Requires explicit confirmation. Logged.
- [ ] `shhh trust .` — explicitly trusts the current project's `.shhh.toml` (alternative to the interactive prompt).

### Comprehensive `shhh doctor`

- [ ] Daemon reachable, socket responsive.
- [ ] Proxy daemon running and bound to the expected port.
- [ ] Hook config present and valid JSON for every installed tool.
- [ ] Hook binary executable, hash matches expected.
- [ ] MCP server binary executable, hash matches expected, listed in each IDE's config correctly.
- [ ] `BASE_URL` env variables set correctly in the user's shell profile, and current shell has them.
- [ ] Temp directory `/tmp/shhh-*` writable.
- [ ] Detection rules loaded: total count and version.
- [ ] Custom rules loaded from `~/.config/shhh/rules/`.
- [ ] Trust database accessible.
- [ ] Output is colorized, grouped by subsystem, and ends with an overall status (green / yellow / red) and an actionable summary.

### Distribution polish

- [ ] Homebrew formula updated with all Phase 4/5 features.
- [ ] Curl installer script updated.
- [ ] Debian `.deb` package (stretch goal — only if there's demand).
- [ ] README v2 with full documentation reflecting Phase 5 capabilities.
- [ ] Docs site at `shhh.dev` (or decided alternative) — static site generated from the `docs/` directory.

## Ship criterion

A fresh machine with Claude Code, Cursor, and Aider installed:

```bash
npx shhh init
```

...completes the full install, runs the first scan, and shows the user a protection status — all in under 90 seconds, with no errors and no manual follow-up steps.

A second test:

```bash
npx shhh uninstall --all
```

...fully reverts the system to its pre-shhh state. `shhh status` afterward shows nothing installed. The backup files remain in `~/.config/shhh/backups/` for manual recovery if needed.

Both tests must pass end-to-end on macOS and Linux. Windows can be a Phase 6 stretch goal.

## Explicitly out of scope for Phase 5

- Team configuration sharing beyond the per-project `.shhh.toml`. Phase 6.
- VS Code extension. Phase 6.
- CI mode (`shhh scan --ci` with exit codes for pipelines). Phase 6.
- Integration with external secret managers. Phase 6.
- LLM-local enrichment of detection (the original "v2" idea from the PRD — shelved indefinitely until an eval task demonstrates pattern-based labeling is insufficient).
- Monetization or paid features. Not even contemplated yet.
- Windows TUI polish beyond "it runs." Focus is macOS and Linux.

## Phase-specific risks

| Risk | Severity | Response |
|------|----------|----------|
| TUI works on the developer machine but fails on exotic terminal emulators | Medium | Test on iTerm, Terminal.app, Alacritty, Kitty, gnome-terminal, tmux, screen. Have a `--no-tui` fallback that runs the same flow as a sequence of prompts. |
| Uninstall fails partway and leaves a half-removed system | High | Uninstall is transactional where possible: write a uninstall plan, execute it, roll back on failure. Backups are the safety net. |
| Trust-on-first-use prompt gets ignored / auto-yes'd by users, defeating the security purpose | Medium | Make the prompt clear about what changes and include a "why does this matter" explainer in the TUI. Document the design choice. Accept that a fraction of users will approve blindly — that's a UX limit, not a bug. |
| `shhh add-rule` regex lets users write patterns that cause false-positive storms | Medium | Validate patterns against the public-example corpus before accepting them. Warn if the new rule matches >X safe strings. |
| Auto-detection misidentifies a similarly-named binary as one of the supported tools | Low | Use version-check as a secondary signal. Fall back to "not detected" on ambiguity. |

## Dependencies

- Phase 4 shipped with all per-tool install commands working non-interactively.
- The `ARCHITECTURE_DECISIONS.md` from Phase 2 includes an ADR on the trust-on-first-use model (required for the security-boundary design).
- Community feedback from 3+ months of Phase 1–4 public usage has surfaced the rough edges that Phase 5 must smooth over.
