# shhh

> Stop leaking secrets to AI coding agents.

`shhh` is a thin local hook that drops into your coding agent (Claude
Code, Codex, Cursor) and redacts secrets — API keys, tokens,
connection strings — **before** they reach the LLM. The agent sees
typed placeholders and can keep reasoning. The raw secret never
leaves your machine.

```
$ shhh install claude-code
$ claude
claude> read .env
  # Claude sees: STRIPE_LIVE_KEY=[STRIPE_LIVE_KEY:sk_live_...:b4135099]
  # Not the raw key. Ever.
```

---

## Install

Pick one. All paths land the same `shhh` binary on your `$PATH`.

### Native install (recommended)

**macOS, Linux, WSL**

```sh
curl -fsSL https://musubi42.github.io/shhh/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://musubi42.github.io/shhh/install.ps1 | iex
```

The script detects your OS/arch, fetches the latest release from
GitHub, verifies the SHA-256 checksum, and installs to
`/usr/local/bin` (falls back to `~/.local/bin` if not writable) on
Unix, or `%LOCALAPPDATA%\Programs\shhh` on Windows. Override either
with `SHHH_INSTALL=/some/path`.

### Go

```sh
go install github.com/Musubi42/shhh/cmd/shhh@latest
```

### Manual

Grab the archive for your platform from the
[releases page](https://github.com/Musubi42/shhh/releases/latest),
verify the checksum, extract `shhh`, drop it on your `$PATH`.

> Homebrew tap coming in a future release.

---

## Wire it into your agent

```sh
shhh install claude-code     # writes a hook into ~/.claude/settings.json
shhh install                 # interactive picker (recommended on first run)
shhh uninstall claude-code   # clean removal of the hook entry
```

Codex and Cursor support are on the roadmap
([`docs/implementation-roadmap.md`](docs/implementation-roadmap.md)).

---

## Uninstall

To remove shhh entirely (binary + cache + Claude Code hook):

**macOS, Linux, WSL**

```sh
curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://musubi42.github.io/shhh/uninstall.ps1 | iex
```

The script detaches shhh from Claude Code, removes the binary, and
deletes `~/.shhh` (session cache + audit log). Pass `SHHH_KEEP_DATA=1`
to keep the cache and audit history.

Installed via `go install`? Also `rm $(go env GOPATH)/bin/shhh`.

---

## See what shhh is doing

```sh
shhh scan .                  # list every secret-shaped value in this directory
shhh audit                   # forensic audit of what Claude has seen so far
```

The hook also prints a one-line trailer after each tool call it
redacted, so you always see what was caught:

```
--- shhh (local secret-redaction tool) redacted 1 secret from this command's output. ---
  - STRIPE_LIVE_KEY at output line 23 (placeholder: [STRIPE_LIVE_KEY:sk_live_...:b4135099])
```

---

## Why this exists

Coding agents stream your files and shell output to an LLM provider.
That stream includes whatever happens to be in your `.env`, your
shell history, your `git diff`. Once a secret leaves the machine, it
has left the machine — rotating it is the only fix.

`shhh` runs entirely locally. No network calls. No daemon. No
external service. It's a single Go binary the agent shells out to
on each tool call.

For the full design rationale see [`PRD.md`](PRD.md) §§1, 2, 5, 6, 8.

---

## Development

```sh
make build    # binaries in ./bin
make test     # full test suite
make demo     # end-to-end hook smoke test
```

Repo guides:

- [`CLAUDE.md`](CLAUDE.md) — operating instructions for any AI agent (and humans) working in this repo.
- [`docs/implementation-roadmap.md`](docs/implementation-roadmap.md) — milestone list.
- [`ROADMAP.md`](ROADMAP.md) — current friction issues found during dogfooding.

---

## License

MIT. See [`LICENSE`](LICENSE).
