# Testing playbook

How to test shhh changes end-to-end without losing an hour to
avoidable mistakes. Written 2026-05-26 after a dryrun session
where ~30 minutes evaporated into stale binaries, silently-aliased
`cp`, and a buffered `head` pipe.

This doc is for the agent (or human) doing the next test pass.
It assumes you already know what shhh does — see `CLAUDE.md` for
that. **Read this once before you `make build` for testing
purposes.** Skip the section index and read top-to-bottom the
first time; it's < 5 minutes.

---

## 0. The one rule

**`go test ./...` is necessary, not sufficient.** It exercises Go
functions, not the real `flag.Parse` path the user takes. Two
CRITICAL bugs shipped in 2026-05-26 — `--no-serve` silently ignored
after a positional, install's `--scope` token getting turned into a
stray `./--scope/.claude/` directory — both passed every unit test.
They were caught only by invoking the binary in a real shell.

For any change touching `cmdaudit/`, `cmdinstall/`, `cmdscan/`,
`cmdredact/`, or `main.go`:

1. `make build`
2. **`install -m 0755 bin/shhh ~/.local/bin/shhh`** (or wherever
   the user's `which shhh` resolves)
3. Run the affected command in a real shell, **with a flag placed
   *after* a positional arg** (e.g., `shhh audit . --no-serve`).
   That ordering is the regression that hides argv parsing bugs.
4. `shhh doctor` to sanity-check state didn't drift.

Only then claim done.

---

## 1. Pre-flight (60 seconds)

```sh
which shhh                 # know which binary the shell will run
shasum -a 1 bin/shhh $(which shhh)   # they should match after install
shhh doctor                # config + hooks + binary alignment
```

If `shasum` shows different hashes, the user is running an old
binary — your behavior changes are invisible until you reinstall.
This is the single most common time-sink.

`shhh doctor` will tell you:
- The running binary path
- Whether `~/.shhh/config.json` is sane
- Whether each `installed_paths` entry actually has the shhh hook in
  its settings.json (catches the F3 desync class)

---

## 2. Invocation patterns

### `make build` does nothing — what gives?

`make build` is timestamp-based. If your last edit touched a `.go`
file but its mtime is older than `bin/shhh` (rare, but happens with
copy/restore operations or git checkouts), `make` will skip the
rebuild. Force it:

```sh
rm bin/shhh && make build
```

### Bash tool truncates output — what gives?

Long-running shhh commands write to stdout/stderr progressively.
If you pipe them to `head -N`, the pipe buffers and you see nothing
until N lines accumulate. **Redirect to a file instead** and
inspect with `wc -l` + `tail`:

```sh
shhh audit . --no-select --no-serve > /tmp/audit.txt 2>&1 &
# then poll: wc -l /tmp/audit.txt
```

For runs you expect to take more than ~30s, prefer
`Bash(run_in_background: true)` with a separate poll command.

### Aliases bite

Default zsh on the maintainer's machine aliases `cp` → `cp -i` and
`rm` → `rm -i`. Both prompt on overwrite/delete and silently
default to "no" in non-interactive contexts. **Bypass with the
absolute path or use `install`:**

```sh
install -m 0755 bin/shhh ~/.local/bin/shhh    # for binaries
install -m 0600 src dst                       # for config files
/bin/rm -f path                               # for forced delete
```

Never use `cp -f` from inside the Bash tool — the `-i` alias takes
precedence and you'll lose time chasing a no-op.

---

## 3. Pitfalls catalog

### The hook only activates on session restart

`shhh install claude-code` writes to `settings.json`, but Claude Code
only reads that file at session boot. **If you're an agent
running inside Claude Code, your own tool calls will NOT start going
through the hook mid-session.** This means:

- The forcing-function scenario (install → read .env → see
  placeholder) **cannot be self-verified by an agent**. Hand off to
  a human to spawn a fresh session and read a fixture.
- For per-project installs, the user's existing claude sessions
  also won't pick up the hook until restart. The install command's
  footer already says this; `shhh doctor` echoes it.

### Argv parser silently drops flags after positionals

Go's `flag.Parse` stops at the first non-flag arg. This is fixed
in `cmdaudit` (`splitFlagsAndPositionals`) and `cmdinstall`
(`splitArgsByFlags`). But if you add a NEW string flag to either,
the trivial split breaks. **If cmdaudit grows a string/int flag,
swap its splitter for the value-aware one from cmdinstall.**

Always test new flags with both orderings:

```sh
shhh <cmd> --flag value positional
shhh <cmd> positional --flag value
```

### Side-effects of broken argv parsing

A bug in argv parsing can turn flag tokens into "paths" and the
install code will happily `MkdirAll(./--scope/.claude/)`. After
ANY install/uninstall test in the repo dir, check for stray
`.claude/` directories:

```sh
find . -type d -name '.claude' -not -path './.claude' 2>/dev/null
```

The `not -path './.claude'` clause excludes the legitimate
per-project install at the repo root (created during normal
dogfooding).

### Background servers leave zombies

`shhh audit` (no `--no-serve`) starts an HTTP server on a random
port and blocks on Ctrl-C. If you send the wrong signal or kill
the wrong PID, the server keeps running and ports leak.

After any audit run that wasn't `--no-serve`:

```sh
ps aux | grep "shhh audit" | grep -v grep
# kill stray PIDs explicitly
```

### `shhh redact --session <uuid>` is a normal hook firing

Don't confuse a live `shhh redact --session ...` process with a
zombie. It's the SessionEnd hook running on transcript close —
short-lived and benign. Only worry about long-running `shhh audit`
or `shhh hook` processes.

---

## 4. Dogfood pattern

The most efficient integration test is to **be the user**. If
you're an agent inside a Claude Code session AND shhh's hook is
installed globally, every tool call you make exercises shhh:

- Your `Read` calls go through the PreToolUse(Read) hook → tests
  the redaction path on real files.
- Your `Bash` calls go through PreToolUse(Bash) → tests the bash
  command rewrite path.
- Session close fires SessionEnd → tests the final flush.

But — see "hook only activates on session restart" — this only
works for the session you started in. If you mid-session
reinstall, you don't suddenly start firing hooks. Plan
accordingly.

When dogfooding catches a bug (as it did for F1 in 2026-05-26),
**log it AS the agent** — your `Bash` tool calls are the most
authentic integration test the project has.

---

## 5. Cleanup recipes

### After a dryrun: reset to a known-good state

```sh
# 1. See what shhh thinks is installed
shhh doctor

# 2. If there are stale entries, heal them
shhh doctor --fix

# 3. Check for stray .claude/ dirs in the repo
find . -type d -name '.claude' -not -path './.claude'

# 4. Kill any audit servers still running
pkill -f "shhh audit" 2>/dev/null

# 5. Sanity-check the hook is still wired into your global config
grep -q "shhh hook" ~/.claude/settings.json && echo "hook OK" || echo "REINSTALL NEEDED"
```

### Reinstall global hook from scratch

```sh
shhh uninstall claude-code      # global
shhh install claude-code        # global, fresh
shhh doctor                     # confirm green
```

### Drop a stray per-project install

```sh
shhh uninstall claude-code /path/to/repo
# verify
shhh doctor
# .claude/ directory left in place by design — see CLAUDE.md
rm -rf /path/to/repo/.claude   # only if you want to fully forget
```

---

## 6. Test order for a typical change

For a typical "edit cmdaudit, run tests, ship" cycle:

1. `go test ./cmd/shhh/cmdaudit/`  (fastest feedback)
2. `go test ./...`                 (regression sweep)
3. `make build`                    (compile-check the full binary)
4. `install -m 0755 bin/shhh ~/.local/bin/shhh`
5. **At least one real-shell invocation** of the affected command,
   with a flag *after* a positional. Capture stdout+stderr to a
   file, read it.
6. `shhh doctor` to verify state didn't drift.

Skip step 5 only for changes that genuinely cannot touch CLI
parsing (internal package refactors, doc-only changes,
test-only changes).

---

## 7. When something feels weird

- **First reflex:** `shasum bin/shhh $(which shhh)`. If they
  differ, reinstall and re-test.
- **Second reflex:** `shhh doctor`. It catches config drift, stale
  install entries, and binary mismatch in one shot.
- **Third reflex:** run the failing command with `2>&1 > /tmp/x`
  and inspect the file. Not the pipe.
- **If still stuck:** add a debug print to the suspected code
  path, rebuild, reinstall. Don't try to reason your way to the
  bug — the time you save guessing is gone the moment you have to
  rebuild anyway.

The 2026-05-26 dryrun lost time precisely because of skipping
these reflexes ("the binary must be fresh, I just rebuilt"). It
wasn't. `shasum` would have told me in 1 second.

---

## Reading order for a fresh test pass

1. This file.
2. `CLAUDE.md` hard rules (especially #1 on what "progress" means).
3. The doc that motivates the change you're testing (likely under
   `docs/`).
4. The previous dryrun log if there is one (`docs/dryrun-YYYY-MM-DD.md`)
   — it'll tell you what already broke.

That's it. Test smart, not long.
