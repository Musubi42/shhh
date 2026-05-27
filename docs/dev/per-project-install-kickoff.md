# Per-project install — kickoff doc

Captures everything we worked out about per-project shhh installs
during the 2026-05-25 dryrun session. To be implemented in a later
session by someone (or some agent) starting fresh.

**Status:** MVP shipped 2026-05-25. `shhh install claude-code
--scope project [--cwd <path>]` and the symmetric uninstall both
work end-to-end. `.claude/` is auto-created when missing. Config
`scope` is now re-derived from `installed_paths` on every load
(any path matching the user's global settings.json wins as
"global"). Audit picks up per-project installs correctly via
`coversPath`. See "What's still pending" at the bottom for the
follow-ups that didn't make this batch.

---

## What "per-project install" means

shhh's hook can be wired into Claude Code at two scopes:

| Scope | settings.json location | Coverage |
|---|---|---|
| Global (current default) | `~/.claude/settings.json` | every Claude Code session, present and future, on this machine |
| Per-project (this doc) | `<project>/.claude/settings.json` | only sessions whose `cwd` is under `<project>` |

Per-project is wanted for two reasons:
1. **Team adoption.** A `.claude/settings.json` checked into the
   project repo means every contributor gets shhh automatically
   without each having to install globally.
2. **Surgical control.** A user who doesn't want shhh active
   everywhere (perf, distrust, conflict with another tool) can
   enable it only where it matters.

Today `shhh install claude-code` is hard-wired to scope=global. The
interactive picker (`shhh install` no arg) also hard-codes scope to
global — see `cmd/shhh/cmdinstall/interactive.go:43` and the comment
"v0.2: always global from the interactive flow".

## What's already built

- `cmdinstall.Plan.Scope` accepts `ScopeGlobal` or `ScopeProject`
- `cmdinstall.AgentSettingsPath(agent, scope, cwd)` returns the
  correct path for either scope
- `cmdinstall.Install(path, binary)` is scope-agnostic — give it any
  settings.json path and it merges hook entries
- `internal/audit/status.go::coversPath()` already understands
  per-project install: a settings.json under `<project>/.claude/`
  marks `<project>` and any descendants as HOOKED
- The user's persistent config (`~/.shhh/config.json`) has fields
  for both `scope` and `installed_paths` (a list, so multiple
  per-project installs can coexist)

## What's missing

### 1. CLI surface

Add a `--scope` flag to `shhh install`:

```sh
shhh install claude-code                    # default: global (unchanged)
shhh install claude-code --scope global     # explicit
shhh install claude-code --scope project    # writes <cwd>/.claude/settings.json
```

Symmetric for uninstall:

```sh
shhh uninstall claude-code --scope project  # removes from <cwd>/.claude/settings.json
```

If `--scope project` is passed without `cwd`, default to the current
working directory.

### 2. The `.claude/` directory creation

Important gotcha caught during the design discussion: **the project's
`.claude/` directory often doesn't exist** when the user runs
`shhh install --scope project`. Claude Code creates `.claude/` lazily
on first session in that dir, but a user might want to pre-install
shhh in a fresh repo before ever running Claude there.

Options:
- **Create `.claude/` if missing** (recommended). One `os.MkdirAll`
  call. The directory is harmless — Claude Code will use it.
- Fail with "no `.claude/` here, run `claude` once first". Hostile.
- Prompt "create `.claude/`? [y/N]". Friction.

→ Implement option 1, mention in the install output that `.claude/`
was created if it didn't exist (so the user understands the new
directory).

### 3. Multiple per-project installs

`config.json::installed_paths` is already a list, so this works:

```json
{
  "scope": "project",
  "installed_paths": [
    "/Users/alice/work/billing/.claude/settings.json",
    "/Users/alice/work/api/.claude/settings.json"
  ]
}
```

But the `scope` field is a single value — what does it mean if the
user has BOTH a global install AND a per-project install? Today
`coversPath` handles both correctly (any matching settings.json
hooks the project) but `scope` becomes ambiguous.

→ Change `scope` to a derived/computed field, not a fixed one. The
config simply lists `installed_paths`; whether each is "global" or
"project" is inferred from the path (is it `~/.claude/settings.json`
or `<somewhere>/.claude/settings.json`?). Audit logic stays the same.

Alternative: keep `scope` for back-compat but treat it as a hint
only, with the real source of truth being the `installed_paths` list.
Less clean but no migration needed.

### 4. Audit integration: per-project install CTA in HTML

The HTML audit report (per task 25 in the 2026-05-25 batch) shows a
top-level "Install shhh globally" CTA. Each per-project card on an
unhooked project should also display a per-project install snippet:

```
This project is not hooked yet.
Install shhh just for this project (recommended for team repos):

  cd /Users/musubi42/Documents/Musubi42/Eurio && shhh install claude-code --scope project

Or hook everything at once: shhh install claude-code
```

The snippet uses the project's absolute path so the user can copy
and paste verbatim.

### 5. Override edge case

A user with global install might intentionally disable shhh on one
specific project — e.g. by editing `<project>/.claude/settings.json`
to remove the inherited hook entry. Today the audit's "is shhh
covering this project" check would still return true (global covers
everything).

To handle this honestly, the audit should:
1. Recognize a per-project settings.json that exists and has no
   shhh hook
2. Mark the project as `[NOT HOOKED]` even if global install
   is active

This is an edge case — probably defer until a real user asks for it.
Document in CLAUDE.md or here that this scenario isn't handled yet.

### 6. Interactive picker

The `shhh install` (no args) interactive flow at
`cmd/shhh/cmdinstall/interactive.go` currently hard-codes
`Scope: ScopeGlobal`. Add a step that asks:

```
Where to install shhh?
  > Global — every Claude Code session on this machine (recommended)
    This project only — repo-scoped, can be checked into git
```

The "this project only" option appears only if the picker has a
project context (it's invoked from inside a directory that looks
like a project — has a `.git/`, a `package.json`, etc.).

## Test plan for the implementer

When this feature lands, verify the following manually:

- [ ] `shhh install claude-code --scope project` from inside a fresh
      repo (no `.claude/` dir) creates `.claude/settings.json` with
      shhh hook entries
- [ ] Same command from inside a repo that already has
      `.claude/settings.json` (with other tools' entries) merges
      cleanly without clobbering
- [ ] `shhh uninstall claude-code --scope project` removes only the
      shhh entries, leaves other tools alone, and deletes the `.claude/`
      directory only if it's now empty (or maybe never deletes — TBD)
- [ ] `shhh audit` after a per-project install shows that project as
      `[HOOKED ✓]` and other projects as their actual status
- [ ] Per-project install of shhh on a fresh repo is checkable into
      git: `git add .claude/settings.json && git commit` works; a
      teammate cloning the repo gets shhh active automatically on
      their next `claude` session (assuming they have the shhh
      binary installed)

## What NOT to do

- Don't change the default scope. `shhh install claude-code` stays
  global. Adding `--scope project` is the explicit opt-in.
- Don't auto-detect "this looks like a project repo, install
  per-project". Implicit magic is surprising.
- Don't ship a TUI for picking install locations. The `--scope`
  flag is enough; the interactive picker can ask one question.
- Don't try to handle the "global install + project override
  disabling shhh" case unless a real user asks. It's an edge case
  with a clear workaround (uninstall global, install per-project
  everywhere needed).

## Reading order for the implementer

1. `CLAUDE.md` hard rules
2. This document
3. `cmd/shhh/cmdinstall/plan.go` — Plan/Scope/Execute (already
   parameterized over scope, just unused at the CLI surface)
4. `cmd/shhh/cmdinstall/cmdinstall.go` — `installClaudeCode()` and
   `uninstallClaudeCode()` — wire `--scope` here
5. `cmd/shhh/cmdinstall/interactive.go` — the picker, add the
   "global vs project" question
6. `internal/audit/status.go` — `coversPath` for the audit-side
   recognition (already correct)
7. `cmd/shhh/cmdaudit/render_html.go` — add per-project install
   snippet to unhooked project cards (the v0.2 batch left a hook
   for this in the rendering pipeline; if not, add a new field to
   `projectRowVM`)

---

## What's still pending after 2026-05-25 MVP

These items from the original plan above are NOT done. Pick them
up in a future session when there's a user need.

- **Override edge case**: a user with global install who manually edits a per-project `settings.json` to remove the hook is still reported as HOOKED. Detect "per-project settings.json with no shhh hook" and demote that project to NOT HOOKED.

## Decisions captured

- **Interactive picker (done 2026-05-26)**: `shhh install` (no args) now asks "Where to install shhh?" between agent selection and audit-scope selection. Project scope sources its candidate list from `~/.claude/projects/` (OnDisk only) — multi-select, hook lands in each chosen project's `.claude/settings.json`. A `✍ Type a custom path...` option supports repos absent from Claude history. CLI shape generalized to positional: `shhh install claude-code [paths...]` infers project scope automatically.
- **Per-project HTML mini-CTAs (done 2026-05-26)**: each NOT HOOKED card on the audit overview now embeds a copy-paste-ready `shhh install claude-code <abs-path>` line styled with a light accent block. Footer "protect at-risk projects" block uses the same positional shape — no more `cd <path> && shhh install`.
- **`.claude/` retention on uninstall (decided 2026-05-26)**: we never delete the `.claude/` directory on uninstall, even when empty. Claude Code itself may use the directory for unrelated state; an empty dir is harmless; and partial cleanup (delete settings.json but keep dir, or only delete-if-empty) adds complexity for no user-visible benefit. Documented in `cmdinstall.go::uninstallClaudeCode`.
