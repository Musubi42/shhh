# Installer TUI — wireframes

The installer TUI runs in a terminal via `charmbracelet/huh`. It is
not a web surface, so there are no HTML mockups — this document is
the source of truth for the prompt flow, copy, and layout.

## Entry point

```
$ npx shhh install          # interactive (default)
$ shhh install              # same thing, from a locally-built binary
$ shhh install claude-code  # non-interactive, scripted path (existing)
```

Non-TTY stdin → the installer refuses with a clear error and points at
the scripted path. Never hangs.

## Flow shape

The installer is **agent-first**. Once one or more agents are picked,
each selected agent gets its own configuration group. For v0.2 the
only supported agent is Claude Code, so only one config group runs;
the code paths are structured for N agents so v0.3 can plug in Codex
and Cursor without a rewrite.

```
┌──────────────────────────────────────────────────────────────┐
│  GROUP 1 — Agent selection                                   │
├──────────────────────────────────────────────────────────────┤
│  For each detected agent:                                    │
│    GROUP N — <agent> configuration                           │
│      - project multi-select (agent-specific)                 │
├──────────────────────────────────────────────────────────────┤
│  GROUP FINAL — Confirm and run                               │
│    - scan opt-in (default NO)                                │
└──────────────────────────────────────────────────────────────┘
```

## Screen 1 — welcome banner

Printed once at startup, before the first prompt. No input.

```
    ███████╗██╗  ██╗██╗  ██╗██╗  ██╗
    ██╔════╝██║  ██║██║  ██║██║  ██║
    ███████╗███████║███████║███████║
    ╚════██║██╔══██║██╔══██║██╔══██║
    ███████║██║  ██║██║  ██║██║  ██║
    ╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝

    stop leaking secrets to AI agents · v0.2.0

    This wizard installs shhh into each coding agent you use.
    Everything runs locally — nothing leaves your machine.
```

## Group 1 — pick agents

```
  ┌─ Which coding agents do you use? ────────────────────────────┐
  │                                                              │
  │  Pick the agents shhh should protect. Unsupported agents are │
  │  greyed out and will ship in later versions.                 │
  │                                                              │
  │    [x] Claude Code                  (detected at ~/.claude)  │
  │    [ ] Codex                        (support coming soon)    │
  │    [ ] Cursor                       (support coming soon)    │
  │                                                              │
  │  space to toggle · enter to continue                         │
  └──────────────────────────────────────────────────────────────┘
```

**Detection rule (v0.2):** an agent is "detected" iff its config
directory exists (`~/.claude/` for Claude Code). Unsupported agents
appear in the list but cannot be selected; removing them entirely
would hide the roadmap, which we don't want.

**Validation:** at least one agent must be checked or the form
refuses to advance.

## Group 2 — Claude Code configuration

Runs once per selected agent. Agent-specific prompts live here.

### 2a — which projects to audit

```
  ┌─ Claude Code · projects to audit ────────────────────────────┐
  │                                                              │
  │  shhh found 13 projects in ~/.claude/projects/. For each     │
  │  one, it can read the local session transcripts to find      │
  │  secrets that have already been sent to Claude.              │
  │                                                              │
  │  Projects you uncheck here will be SKIPPED during audits.    │
  │                                                              │
  │    [x] ~/Documents/Musubi42/betterShortForge                 │
  │    [x] ~/Documents/Musubi42/betterShortForge/apps/api        │
  │    [x] ~/Documents/Musubi42/betterShortForge/services/ccp... │
  │    [x] ~/Documents/Musubi42/codeBackup                       │
  │    [x] ~/Documents/Musubi42/Eurio                            │
  │    [x] ~/Documents/Musubi42/musuCopilot                      │
  │    [x] ~/Documents/Musubi42/shhh                             │
  │    [x] /tmp/test-fb-com          (folder gone · transcripts) │
  │    [x] /tmp/test-musuMangaDown   (folder gone · transcripts) │
  │    [x] /tmp/test-sc2-hud-lowko   (folder gone · transcripts) │
  │      ... 3 more                                              │
  │                                                              │
  │  a to toggle all · space to toggle one · enter to continue   │
  └──────────────────────────────────────────────────────────────┘
```

Notes:
- Projects whose on-disk folder no longer exists are marked
  `(folder gone · transcripts)`. The user can still include them;
  their transcripts are in Claude's history regardless of whether
  the code folder still exists.
- The list is scrollable with arrow keys. `huh`'s MultiSelect
  handles this natively.
- Defaults: everything checked. The user's action is to *exclude*
  projects, not to opt-in.
- Long paths are truncated with `...`. The full path is shown on
  focus (huh renders the focused item with its full text).

### 2b — (REMOVED FROM v0.2)

The earlier draft included a per-subdirectory opt-in (transcripts /
paste-cache / history / file-history). **We cut this during
refinement.** In v0.2 shhh always reads all four sources. Fewer
prompts, simpler mental model, and the disclosure in the final
confirm step lists them explicitly.

## Group final — confirm and optionally scan

```
  ┌─ Ready to install ───────────────────────────────────────────┐
  │                                                              │
  │  shhh will install the hook into:                            │
  │    ~/.claude/settings.json                                   │
  │                                                              │
  │  The hook intercepts file reads and bash command output in   │
  │  Claude Code sessions and redacts any secrets BEFORE the     │
  │  content reaches the model.                                  │
  │                                                              │
  │  Configuration will be saved to:                             │
  │    ~/.shhh/config.json                                       │
  │                                                              │
  │  ─────────────────────────────────────────────────────────   │
  │                                                              │
  │  Optional: run a forensic audit after install?               │
  │                                                              │
  │  If you say yes, shhh will read these local files to find    │
  │  secrets that have already been sent to Claude in past       │
  │  sessions (so you know which ones to rotate):                │
  │                                                              │
  │    • ~/.claude/projects/**/*.jsonl   session transcripts     │
  │    • ~/.claude/paste-cache/*.txt     content you've pasted   │
  │    • ~/.claude/history.jsonl         prompt history          │
  │    • ~/.claude/file-history/**       file edit history       │
  │                                                              │
  │  Nothing is sent over the network. Everything stays on       │
  │  this machine. Raw secrets never appear in the output.       │
  │                                                              │
  │    ( ) Skip  — I'll run `shhh audit` later                   │
  │    (•) Run the audit now                                     │
  │                                                              │
  │  enter to confirm · esc to cancel                            │
  └──────────────────────────────────────────────────────────────┘
```

Default selection: **Skip**. The user hits Enter and the installer
completes without running a multi-second scan. This is
opt-in-by-design, per resolved Q6 in the NPX installer spec.

## Screen — completion

Printed after the install commits to disk and, if the user opted in,
after the audit finishes.

```
  ✓ Hook installed          ~/.claude/settings.json
  ✓ Configuration saved     ~/.shhh/config.json
  ✓ Projects registered     13 (2 will be skipped)

  ─────────────────────────────────────────────────────────────
  Coming soon: Codex, Cursor. Track progress in docs/mvp/.
  ─────────────────────────────────────────────────────────────

  Start a new Claude Code session for the hook to take effect.

  To audit your existing sessions any time:
      shhh audit                      (terminal + HTML report)
      shhh audit --no-serve           (terminal only, for scripts)
```

If the audit ran, the completion screen is followed immediately by
the audit CLI output (see [`cli-output.md`](./cli-output.md)) and
the report server URL.

## Cancel / error states

- **`Ctrl+C` during prompts** → `form.Run()` returns an error, the
  installer prints `installer cancelled (no changes made)` and
  exits 1. Nothing is written to disk.
- **No agents detected** (no `~/.claude/`, no `~/.codex/`, no
  `~/.cursor/`) → short-circuit before the form, print
  `no supported agents detected on this machine (looked for ~/.claude).
  install Claude Code first, then re-run shhh install`, exit 1.
- **`~/.claude/projects/` empty** → the project multi-select shows
  a single informational line: *"no past sessions found — shhh will
  still protect future sessions"*, the multi-select is empty, and
  the form proceeds.
- **Write failure on settings.json or config.json** → rollback any
  partial writes (tmp+rename means we either committed both or
  neither), print the underlying error, exit 1.

## Dimensions and style

- **Terminal width assumption:** 80 columns minimum, 120 preferred.
  Text wraps cleanly at 76 chars (the prompt body width). Project
  paths longer than 54 chars truncate with `…` in the middle (huh's
  default behavior for long options).
- **Colors:** huh's default theme, overridden so:
  - accent = magenta (Claude-like; picked because it's distinctive
    from the terminal red/green we use for audit output)
  - confirm button = green background
  - destructive states (cancelled, error) = red
- **No emoji on prompt labels.** Emoji live in the audit output,
  not in the installer. The installer stays restrained.

## What the TUI does NOT ask

Explicitly cut from the flow, for decision hygiene:

- ~~global vs project install scope~~ — always global for v0.2
  (resolved: the user called it a distraction)
- ~~obfuscation level picker~~ — always `typed` for v0.2 (resolved
  α in the NPX installer spec)
- ~~per-subdir audit opt-in~~ — cut, always read all 4 sources
- ~~auto-update channel~~ — deferred to v0.3+
- ~~which .claude/ subdirectories to scan~~ — hard-coded

Each of these was discussed and explicitly dropped. Do not
reintroduce without a documented user need.
