# Feature: NPX installer (anchor)

**Status:** refining. Most decisions locked; one Open question remains
(obfuscation level — see Q3).

## Resolved decisions (2026-04-14)

Locked in via conversation, will not be re-opened unless new evidence
appears:

- **Distribution: Option A** — NPM wrapper downloads a platform-specific
  Go binary from GitHub Releases. Go code stays as-is; Node wrapper is a
  pure `execFileSync` shim. Go compiles to static native binaries, no
  runtime dependency on the target machine, so the wrapper only has to
  pick the right binary for the current OS/arch.
- **Install scope: both `global` and `project`** — the installer prompts
  the user to pick. Each prompt displays the **resolved absolute path**
  in parentheses so the user sees exactly where the hook is about to
  land (e.g., `Everywhere on this machine (~/.claude/settings.json)`
  and `Only in the current project (/Users/alice/work/backend)`). No
  `.git`-based auto-detection — too noisy, and project protection
  isn't limited to code projects.
- **Config file: `~/.shhh/config.json`** — JSON, not TOML. Plain dot-dir
  under `$HOME`, not `$XDG_CONFIG_HOME`. Matches the "easy to find when
  I cd into my home" ergonomic the user cares about.
- **Prompt engine: native Go via `charmbracelet/huh`** — keeps the Node
  wrapper minimal and lets the same binary drive non-interactive installs
  via CLI flags for CI.
- **Post-install scan: opt-IN** — default answer is `no`. User must type
  `y` to trigger `shhh scan --home` after install. If they decline, the
  installer prints the exact command they'd run to do it later. We
  accept the marketing cost of a less automatic demo in exchange for
  never surprising the user with a multi-second scan they didn't ask
  for.
- **Auto-update: deferred to v0.3+** — `shhh update` and auto-version-
  bumping are not in v0.2 scope.

## Forcing function

```
$ npx shhh install
? Where should shhh protect you? › (Use arrows)
  ❯ Everywhere on this machine         (~/.claude/settings.json)
    Only in the current directory      (/Users/alice/work/backend)
? Which coding agents should shhh hook into? ›
  ❯ ◉ Claude Code (detected at ~/.claude)
    ◯ Codex        (not yet supported)
    ◯ Cursor       (not yet supported)
? Run a scan of your machine now to see what's at risk? (y/N) › n

✓ installed into ~/.claude/settings.json
✓ config written to ~/.shhh/config.json

Done. Start a new `claude` session and shhh will protect your secrets.
You can run a machine-wide audit any time with:

    npx shhh scan --home
```

The path in parentheses on the first prompt is resolved at runtime, so
the user sees exactly where the hook is about to land before confirming
— not an abstract "current project" label that could mean anything. If
the user happens to run the installer from `~/Documents` by accident,
they see `~/Documents` in the prompt and can back out.

The scan prompt defaults to `N`. A user who just hits Enter through the
installer does NOT trigger a multi-second scan of their whole home
directory. The trade-off — less automatic demo value — is a deliberate
choice to respect the user's time and machine.

A developer on a fresh machine with no Go toolchain types a single
command and ends up with a working, configured hook plus an immediate
report of what was at risk. That's the demo.

## Why this is the anchor feature

- **Distribution.** shhh is currently a Go binary. Without NPX, every
  user has to either install Go, run a one-off curl-pipe-bash, or wait
  for Homebrew. Each of those is an adoption killer. NPX is the only
  distribution channel that's already on every JS developer's machine.
- **Configuration surface.** The installer is where *all* shhh config
  decisions get made. Putting that surface anywhere else (environment
  variables, a separate `shhh config` subcommand, a dotfile the user
  edits by hand) creates a cognitive tax and a support-ticket surface.
  Making the interactive installer own it consolidates the UX.
- **Dependency ordering.** You can't scan if you haven't installed.
  You can't hook an agent if you haven't installed. You can't choose
  an obfuscation level if there's nothing to obfuscate. The installer
  is the load-bearing beam under every other feature.

## What already exists in the codebase

Most of the functionality the installer orchestrates is already built:

- `cmd/shhh/cmdinstall` — `shhh install claude-code` and
  `shhh uninstall claude-code`. Idempotent merge into
  `~/.claude/settings.json` with a before/after diff. The installer
  wraps and extends this.
- `cmd/shhh/cmdhook` — the PreToolUse/Read, PreToolUse/Bash, and
  SessionEnd dispatchers. The installer activates these by writing
  the hook entries; the code itself is unchanged.
- `cmd/shhh/cmdscan` — the scanner CLI. Used by the installer's
  optional post-install scan step.
- `internal/session.PlaceholderFor` — currently hard-codes the
  "typed" placeholder format. Strict mode is ~20 lines here plus a
  config-read step at redactor init.
- `internal/detector`, `internal/redactor` — untouched. The
  installer is pure orchestration.

## What's new

The installer adds, at most:

1. **A Node wrapper** (`bin/shhh.js` or similar) that:
   - Resolves the correct platform-specific binary from a GitHub
     release (darwin-arm64, darwin-x64, linux-x64, linux-arm64,
     win-x64).
   - Caches it under `~/.shhh/bin/shhh-<version>-<platform>`.
   - Forwards argv to the binary via `execFileSync`.
   - No other logic. The Node wrapper is a thin shim, not a
     reimplementation.
2. **A new Go subcommand `shhh install`** (no target arg) that enters
   interactive mode. Different from today's `shhh install claude-code`:
   - Detects installed agents by looking for their config dirs.
   - Prompts for scope, agents, obfuscation level, scan-on-install.
   - Calls the existing `install claude-code` / `install codex` /
     `install cursor` subcommands as subroutines for each selected
     agent.
   - Writes `~/.shhh/config.toml` with the user's choices.
   - Optionally runs `shhh scan --home` at the end.
3. **A config file read-path** for `internal/session` so the
   obfuscation level is honored by every hook invocation. Tiny:
   load `~/.shhh/config.toml` once at startup, pass level to
   `session.New()`.
4. **GitHub Actions release workflow** that builds and publishes
   the 5 platform binaries plus the npm package on tag push.

Explicitly *not* new: the hook behavior, the detector rules, the
settings.json merge logic. All of that is already shipping.

## Configuration surface owned by the installer

The installer is the single place the user can answer these
questions. Every config option has a default so `npx shhh install -y`
works without any prompts.

| Option | Choices | Default | Notes |
|---|---|---|---|
| **Install scope** | `global` / `project` | `global` | `global` writes `~/.claude/settings.json`. `project` writes `.claude/settings.json` in the current working directory. Prompt displays the resolved absolute path in parentheses. |
| **Agents** | multi-select: `claude-code`, `codex`, `cursor` | auto-detected | Only agents whose config dir exists are offered. Unsupported agents show "(not yet supported)" and are not selectable. |
| **Obfuscation level** | see Q3 below | TBD | Unresolved — user's own reasoning arc suggested one level may be enough for v0.2. |
| **Post-install scan** | `yes` / `no` | **`no`** | Opt-in. User must type `y` to trigger `shhh scan --home`. Respects "don't surprise the user" over "always demo value." |
| **Update channel** | `stable` / `prerelease` | `stable` | For the Node wrapper's binary-download step. Only matters once releases exist. |

Stored in `~/.shhh/config.json` after the installer finishes. The user
can re-run `npx shhh install` to reconfigure; it detects the existing
config and pre-fills the prompts.

## Distribution model — decision needed

Three options, same three I outlined in conversation. Writing them
here so the choice is on paper:

### Option A: Node wrapper downloads platform binary (recommended)

**How it works:**
- `@shhh/cli` on npm contains `bin/shhh.js`, `package.json`, and
  nothing else.
- On first run, `shhh.js` checks `~/.shhh/bin/` for a binary
  matching the current version + platform. If missing, downloads
  it from `https://github.com/<owner>/shhh/releases/download/vX.Y.Z/shhh-<os>-<arch>`.
- After download, `execFileSync(binary, process.argv.slice(2))`.

**Pros:**
- Go code is unchanged. All the library work we've done ships
  directly.
- Fast: the binary is native code, not Node.
- Standard pattern (`esbuild`, `biome`, `turbo`, `swc` all do
  this).

**Cons:**
- First-run latency: ~2–5 MB download before the binary runs.
- Release pipeline: must build 5 platform binaries in CI and
  attach them to a GitHub release on tag push. Not hard, but it's
  new infrastructure.
- Corporate firewalls that block direct GitHub release downloads
  will break install. Mitigation: fall back to a mirror or
  document the manual download.

### Option B: Full Node/TS port

**How it works:** reimplement `internal/detector`, `internal/session`,
`internal/redactor`, and the hook dispatchers in TypeScript. Ship
as a pure npm package.

**Pros:**
- Install is a single `npm install` — no download step, no
  platform matrix.

**Cons:**
- Throws away ~2000 lines of Go that work and have tests, plus
  the calibration lessons baked into the detector. Almost
  guaranteed to reintroduce bugs the Go version already fixed.
- The `internal/eval` tests would have to be reimplemented too,
  or lost.
- Slower runtime: Node startup is ~40 ms vs ~5 ms for a static
  Go binary. Multiplied by every tool call the hook fires on,
  that's perceptible.
- Two codebases to maintain if we ever want both.

**Recommendation: no.** This is the option that looks easy but
actually destroys work we've already done.

### Option C: No NPX, use `go install` or Homebrew

**How it works:** ship only a Go binary. Users run `go install
github.com/musubi-sasu/shhh/cmd/shhh@latest` or `brew install shhh`.

**Pros:** zero new infrastructure.

**Cons:** closes the door on every JS developer who doesn't have
Go. The PRD §1 positioning ("installs in 30 seconds") does not
survive "step 1: install Go."

**Recommendation:** keep as a *secondary* distribution channel
alongside NPX, not a replacement.

**My vote:** Option A + Option C as a fallback mention in the README.

## Open questions

### Q3. Obfuscation levels — do we need more than one?

**Context.** The original proposal was two levels:
- `typed` — `[STRIPE_LIVE_KEY:sk_live_...:a1b2]` (current default)
- `strict` — `[SECRET:a1b2]` (drop everything but the hash)

During refinement, the user's own reasoning converged on: *"you
actually want the agent to see `sk_live_` because it tells the
agent whether it's a prod or dev key — which is useful for
reasoning."* If that's the conclusion, the two levels collapse
into one and the installer doesn't need this prompt at all.

**Two coherent paths forward:**

### Option α — one level for v0.2 (default / recommended)

Drop the obfuscation-level prompt from the installer. Ship only
the current `typed` format. Add a second level the first time a
real user brings a use-case that needs it. Respects the
postmortem rule against speculative optionality.

**Cost:** zero. Current code already does this.

### Option β — two levels along a different axis

Not "show vs hide `sk_live_`" but *"preserve vs mask structural
metadata in connection strings."*

- `typed` (default, current) — Postgres URL becomes
  `[POSTGRES_CONNSTRING:admin@prod-db.example.com:5432/myapp:hash]`.
  The agent sees the host, DB name, and username, which is
  useful for *"is this pointing at prod or staging?"* reasoning.
- `strict` — Postgres URL becomes
  `[POSTGRES_CONNSTRING:hash]` with the structural body
  stripped. The agent loses the ability to distinguish prod vs
  staging from the placeholder alone but gains real information
  hiding. Threat model: user doesn't want Anthropic/OpenAI to
  learn that a project has a prod database at
  `prod-db.internal:5432`.

**Key difference from the original framing:** in both levels
the `STRIPE_LIVE_KEY` label AND the `sk_live_` public prefix
are shown. Only the *structural body* of connection-string-
shaped values is affected. That's the metadata the user was
implicitly worrying about when they said *"Anthropic knows our
infrastructure."*

**Cost:** ~20 lines in `session.PlaceholderFor` plus a config
read-path. Code is trivial; the decision is whether the
distinction is worth the extra prompt.

**Assistant's vote: α.** Ship one level in v0.2. Add β (or
something else) if a real user asks for it. The postmortem's
explicit warning is about speculative optionality, and the
user's own reasoning arc during refinement surfaced the
speculation.

**Decision needed before implementation starts.**

## Out of scope for this feature

- The **home audit** itself (`shhh scan --home`). The installer
  *calls* it as a post-install step, but the feature belongs in
  [`feature-home-audit.md`](./feature-home-audit.md). The
  installer should not ship if the scan is broken, but the scan
  is its own silo and its own spec.
- **Codex and Cursor hooks.** The installer surfaces them as
  options *only if* those features exist. Until then, the
  installer shows them as "coming soon" or omits them entirely.
  The installer does not block on those features landing.
- **Auto-update.** `shhh update` or "installer notices a new
  version and upgrades itself" is a v0.3+ concern. v0.2 installs
  a specific version and stays there.
- **Team/org config distribution** (CI-friendly shared config
  via a remote config URL). Speculative; wait for the first user
  who asks.

## What I need from you to start implementing

Just Q3. Everything else is locked (see "Resolved decisions"
at the top of this file). Once Q3 is answered — even if the
answer is "drop it, we're at α" — this spec becomes a work
list and I can start writing code. Until Q3 is answered, the
installer's prompt count is undetermined and I don't know
whether `internal/session` needs a config read-path or not.
