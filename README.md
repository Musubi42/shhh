# shhh

> Stop leaking secrets to AI coding agents.

When Claude Code, Codex, or Cursor runs `Read .env`, the file
content is sent verbatim to the model provider. Secrets included.

`shhh` is a local hook that swaps each secret for a typed
placeholder **before** the tool output reaches the model. The
agent keeps reasoning. The raw value never leaves your machine.

```
$ shhh install claude-code
$ claude
claude> read .env
  # Claude sees: STRIPE_LIVE_KEY=[STRIPE_LIVE_KEY:sk_live_...:b4135099]
  # Not the raw key. Ever.
```

> Live demo (real Claude Code transcript, shhh off vs on):
> [**musubi42.github.io/shhh**](https://musubi42.github.io/shhh/)
> · Hero recording: `assets/hero.tape` (run `vhs assets/hero.tape`
> to generate `assets/hero.gif`).

---

## Wait — Claude has probably already seen some

```sh
shhh audit
```

`shhh audit` reads `~/.claude/projects/` — Claude Code's own
session transcripts on your disk — and reports every secret that
has already left your machine. Most users find between 5 and 50.
None of them is fun to rotate.

Run it once before you install the hook. The number is what
convinces you to keep the hook on.

---

## Install

Pick one. All paths land the same `shhh` binary on your `$PATH`.

**macOS, Linux, WSL**

```sh
curl -fsSL https://musubi42.github.io/shhh/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://musubi42.github.io/shhh/install.ps1 | iex
```

**Go**

```sh
go install github.com/Musubi42/shhh/cmd/shhh@latest
```

**Manual**

Grab the archive for your platform from the
[releases page](https://github.com/Musubi42/shhh/releases/latest),
verify the checksum, drop `shhh` on your `$PATH`.

The script detects OS/arch, fetches the latest release, verifies
SHA-256, and installs to `/usr/local/bin` (falls back to
`~/.local/bin`) on Unix, or `%LOCALAPPDATA%\Programs\shhh` on
Windows. Override with `SHHH_INSTALL=/some/path`.

> Homebrew tap is intentionally deferred until the launch post
> brings real user demand.

---

## Wire it into your agent

```sh
shhh install claude-code     # writes a hook into ~/.claude/settings.json
shhh install                 # interactive picker (recommended on first run)
shhh uninstall claude-code   # clean removal
```

shhh ships with two detection engines:

- **gitleaks** (default) — maintained third-party MIT engine,
  ~222 provider rules, curated path allowlist (lockfiles, vendor,
  binaries skipped).
- **shhh-native** — first-party additive layer. Adds env
  cross-reference ("value defined in `.env` found copy-pasted in
  code") and structural URL handling
  (`[POSTGRES_CONNSTRING:user@host/db:d22dea72]` keeps host/db
  visible, redacts creds).

```sh
shhh install claude-code --engines gitleaks,shhh-native
```

**Codex CLI** support also ships:

```sh
shhh install codex     # writes ~/.codex/hooks.json
```

Coverage today is **Bash only** — `cat .env`, `rg`, `sed -i`, and
any shell command Codex runs are redacted. Codex's internal
`apply_patch` / `read_file` tools do not yet fire `PreToolUse`
upstream (track
[openai/codex#18491](https://github.com/openai/codex/issues/18491)).
Until that lands, in-place edits via `apply_patch` can hand the
model a raw secret. Full repro:
[`docs/known-limitations.md`](docs/known-limitations.md) §2.

**Cursor** support is scoped in
[`docs/ready-to-publish/05-cursor-support.md`](docs/ready-to-publish/05-cursor-support.md).

---

## See what shhh is doing

```sh
shhh scan .              # every secret-shaped value in this directory
shhh audit               # forensic audit of Claude history (the hook above)
shhh bench .             # compare detection engines on this content
shhh ignore list         # active .shhhignore cascade + gitleaks defaults
shhh ignore check <path> # explain which layer decides a given path
shhh licenses            # shhh + third-party MIT notices
```

The hook prints a one-line trailer after each redacted tool call,
so you always see what was caught:

```
--- shhh redacted 1 secret from this command's output. ---
  - STRIPE_LIVE_KEY at output line 23
    placeholder: [STRIPE_LIVE_KEY:sk_live_...:b4135099]
```

---

## Trust

`shhh` runs entirely **locally**. No network calls. No daemon.
No telemetry. No external service. One Go binary the agent
shells out to on each tool call. MIT-licensed; `shhh licenses`
prints every third-party notice (gitleaks v8.30.1 MIT included).

Reproduce the redaction proof end-to-end on your own disk:

```sh
git clone https://github.com/Musubi42/shhh
cd shhh/demo && ./run.sh
```

`run.sh` drives two real Claude Code sessions and greps the
real `.jsonl` transcripts — the bytes the model actually
received. No screenshots to argue with.

---

## Known limitations

After shhh redacts a file, Claude Code's `Edit` and `Write`
tools fail on that file for the rest of the session with `File
has not been read yet`. This is a limitation of the Claude Code
hook API, not a bug in shhh — see
[`docs/design/read-edit-tracking.md`](docs/design/read-edit-tracking.md)
for the three hook-API strategies that were evaluated and ruled
out.

**Workaround:** the hook tells Claude to use the `Bash` tool
(`sed -i`, `tee`, `printf >>`, `python -c`, …) on any file shhh
just redacted. In practice Claude reaches for `Bash` directly
and you never see the `Edit` failure. Bash output is also
redacted, so this stays safe. Full repro:
[`docs/known-limitations.md`](docs/known-limitations.md).

---

## Uninstall

**macOS, Linux, WSL**

```sh
curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://musubi42.github.io/shhh/uninstall.ps1 | iex
```

Detaches shhh from Claude Code, removes the binary, and deletes
`~/.shhh` (session cache + audit log).

To keep the cache and audit history, pass `SHHH_KEEP_DATA=1`
*after* the pipe (so it lands in the shell that runs the script,
not in `curl`'s environment):

```sh
curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | SHHH_KEEP_DATA=1 sh
```

PowerShell: `$env:SHHH_KEEP_DATA = '1'` before running `irm`.

Installed via `go install`? Also `rm $(go env GOPATH)/bin/shhh`.

---

## Development

```sh
make build    # binaries in ./bin
make test     # full test suite
make demo     # end-to-end hook smoke test
```

Guides:

- [`CLAUDE.md`](CLAUDE.md) — operating instructions for any AI
  agent (and humans) working in this repo.
- [`PRD.md`](PRD.md) §§1, 2, 5, 6, 8 — the product vision.
- [`docs/engine-architecture.md`](docs/engine-architecture.md) —
  the gitleaks + shhh-native + `.shhhignore` design.
- [`ROADMAP.md`](ROADMAP.md) — current friction items from
  dogfooding.

---

## License

MIT. See [`LICENSE`](LICENSE).
