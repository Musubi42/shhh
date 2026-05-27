# Release dry-run — manual end-to-end test

This document is **for you, the maintainer**, to run before tagging
the first public release. The release pipeline is mechanical;
*adoption* depends on whether a fresh user can actually install,
configure, and trust shhh in 10 minutes without you holding their
hand. The only way to find that out is to play the role yourself.

Run this from a clean state. Take notes in the "Results" grid at the
bottom as you go. Be ruthless — every "I'll explain it in a tweet"
hand-wave is a documentation bug, every retry is a UX bug.

---

## 0. Pre-flight

You currently have shhh installed and wired into Claude Code. Tear
that down so you arrive at the install page as a stranger would.

- [ ] `shhh uninstall claude-code` — verify the hook entries
      disappear from `~/.claude/settings.json` (diff before/after).
- [ ] `which shhh` — note the current path; we'll remove it.
- [ ] Remove the dev binary from `$PATH`: either `rm $(which shhh)`
      or rename the `bin/` directory so the locally-built binary is
      gone.
- [ ] Restart your shell (`exec $SHELL`). `which shhh` should now
      return nothing.
- [ ] Open a fresh `claude` session in a scratch directory and
      `read .env` (create one with a fake `sk_live_...` first).
      The raw key should appear in the transcript — confirming
      that shhh is genuinely uninstalled.

If any of those steps surprise you, that's bug #0: uninstall is not
clean.

---

## 1. Tag and release (one-shot, can dry-run locally first)

- [ ] **Push everything to main first.** `install.sh` lives at
      `web/install.sh` and is served by GitHub Pages from
      `musubi42.github.io/shhh/install.sh`. The pages.yml workflow
      deploys on push to `main` when `web/**` changes. Wait for the
      Pages deploy to go green before testing the curl-pipe — the
      raw fallback (`raw.githubusercontent.com/.../install.sh`)
      works immediately if you don't want to wait.
- [ ] **Pre-release local dry-run:** `goreleaser release --snapshot --clean`
      from the repo root. Build artifacts land in `dist/`. Spot-check:
      6 archives (linux/darwin/windows × amd64/arm64) + `checksums.txt`.
- [ ] Inspect one tarball: `tar -tzf dist/shhh_*_macos_arm64.tar.gz`
      — should contain `shhh`, `LICENSE`, `README.md`.
- [ ] **Real release:** `git tag v0.1.0 && git push origin v0.1.0`.
      No extra secrets needed (Homebrew tap deferred).
- [ ] Watch the `Release` workflow run. Confirm GitHub Release
      `v0.1.0` exists with all 6 archives + `checksums.txt`.

If anything else needs hand-holding, that's bug #1: the pipeline
isn't actually one-shot.

---

## 2. Discover-as-a-stranger

Open `https://github.com/Musubi42/shhh` in an incognito window. Read
the README cold. Stopwatch yourself.

- [ ] Within 30 seconds, can you tell what shhh does?
- [ ] Within 60 seconds, do you know how to install it?
- [ ] Is the example output (the `sk_live_...` placeholder block)
      clear without explanation?
- [ ] Does the install section give you a copy-pasteable command
      for *your* OS and package manager without scrolling?

Note anything that made you pause. That's onboarding friction.

---

## 3. Install — try each path independently

Between each install attempt, **uninstall fully** (`rm $(which shhh)`,
restart shell, confirm `which shhh` is empty). Otherwise you're
testing a partial state.

### 3a. `curl | sh` — **the headline path, test this one first**

- [ ] `curl -fsSL https://musubi42.github.io/shhh/install.sh | sh`
- [ ] Script prints version, OS/arch, install dir, checksum verify.
- [ ] `shhh version` works after a shell restart.
- [ ] If the install dir wasn't on `$PATH`, the script told you so
      with the exact line to add.
- [ ] Sanity: re-run the same command — installer should overwrite
      cleanly with no error.

### 3b. `go install`

- [ ] `go install github.com/Musubi42/shhh/cmd/shhh@latest`
      *(only works because we renamed the module path to
      `Musubi42/shhh`. If this 404s, the rename didn't fully
      propagate.)*
- [ ] `shhh version` works.

### 3c. Manual

- [ ] Download `shhh_0.1.0_macos_arm64.tar.gz` and `checksums.txt`
      from the releases page.
- [ ] `shasum -a 256 -c checksums.txt --ignore-missing` passes.
- [ ] Extract, drop on `$PATH`, `shhh version` works.

### 3d. PowerShell (if you have a Windows box / VM handy — otherwise skip and flag)

- [ ] `irm https://musubi42.github.io/shhh/install.ps1 | iex`
- [ ] `shhh version` works in a new PowerShell window.

---

## 4. Wire into Claude Code

Pick one of the install paths above; uninstall the others first.

- [ ] `shhh install claude-code` — output shows the diff it made
      and where (`~/.claude/settings.json`).
- [ ] Diff `~/.claude/settings.json` manually: hook entries point at
      the *installed* binary path (not the dev `bin/shhh`).
- [ ] `make demo` from the repo root passes (this hits the locally
      built binary; if you've deleted `bin/shhh` rebuild first).

---

## 5. Non-naive tests against `demo/leaktest/`

These are the **known weak spots** (`ROADMAP.md` items 2–5). The goal
is not to pass — it's to know exactly which still bite.

### 5a. End-to-end leak test (the headline number)

- [ ] `cd demo && ./run.sh`
- [ ] Expect `RESULT: PASS` (0 raw secrets in the shhh-ON transcript).
- [ ] **Look at the precision line.** A `decoys wrongly redacted` count
      > 0 is the false-positive rate showing up — exactly the
      friction `ROADMAP.md` item 2 calls out. Record the number.

### 5b. False positives on docs / tests / fixtures (ROADMAP item 2)

The 2026-04-15 dogfooding session found 5 FPs out of 6 Reads on
shhh's own source tree. Reproduce that:

- [ ] Open a `claude` session at the shhh repo root.
- [ ] Ask Claude to `Read README.md`. Did shhh redact anything in it?
      A redaction here is an FP — README has no real secrets.
- [ ] Same with `docs/postmortem-eval-overbuild.md`,
      `PRD.md`, and a couple of test files under `internal/*/`.
- [ ] Count FP-redactions. Record the rate.

### 5c. Allowlist / bypass gap (ROADMAP item 3)

- [ ] Try to tell shhh "stop redacting `testdata/**`". There is no
      affordance for this today — confirm the gap is real. A fresh
      user will hit this within an hour on a real project.

### 5d. Narration size (ROADMAP item 4)

- [ ] In a real Claude session with shhh ON, read a `.env` and a
      couple of source files. Look at the cumulative size of the
      shhh narration blocks injected into the transcript.
- [ ] Is the "IMPORTANT — how to modify this file" block still
      present? Per ROADMAP item 4 it should be deletable now that
      the Read→Edit ledger bug is largely resolved. If it's still
      there, that's stale text.

### 5e. Read→Edit cascade (ROADMAP item 1, supposedly fixed)

- [ ] Open `testdata/fixtures/hook-playground/README.md`, run its
      "Test 2" scenario, and see whether Edit works on the first
      try after a redacted Read. If you see `File has not been read
      yet`, the fix isn't as complete as advertised.

### 5f. Cache growth on a long session (ROADMAP item 5)

- [ ] Run a 30-minute Claude session on a non-trivial repo with
      shhh on. Then `du -sh ~/.shhh/sessions/`. Record the size.
      Useful baseline for the eventual cache-eviction work.

---

## 6. Uninstall path — **the symmetric curl|sh test**

- [ ] `shhh uninstall claude-code` — settings.json diff is clean
      (no leftover entries, no extra whitespace). The uninstall
      script also calls this for you, but you want to verify it
      works as a standalone command too.
- [ ] `curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | sh`
      — should remove the binary, delete `~/.shhh`, detach from
      Claude Code in one shot.
- [ ] After: `which shhh` returns nothing, `~/.shhh` is gone,
      `~/.claude/settings.json` has no shhh hook entries.
- [ ] Re-run the original `curl | sh` install command to confirm
      the round-trip (install → use → uninstall → install again)
      works cleanly. This is what an evaluating user will do.

---

## 7. Results grid

Copy-paste this table into a fresh file (`docs/release-dryrun-runs/v0.1.0.md`)
and fill it in as you go. One row per test. Then we triage from this.

| Test | Result | Notes / surprises | Severity (block / warn / nit) |
|---|---|---|---|
| 0. Pre-flight uninstall is clean |   |   |   |
| 1. Pages deploy of install.sh green |   |   |   |
| 1. `goreleaser release --snapshot` |   |   |   |
| 1. Tag → release workflow green |   |   |   |
| 2. README readable in <60s |   |   |   |
| 3a. curl\|sh install (headline) |   |   |   |
| 3b. go install |   |   |   |
| 3c. manual install |   |   |   |
| 3d. PowerShell install |   |   |   |
| 4. shhh install claude-code |   |   |   |
| 5a. leaktest RESULT |   |   |   |
| 5a. leaktest precision (decoys redacted) |   |   |   |
| 5b. FP rate on shhh's own repo |   |   |   |
| 5c. allowlist gap confirmed |   |   |   |
| 5d. narration size / stale text |   |   |   |
| 5e. Read→Edit cascade reproduces? |   |   |   |
| 5f. cache size after 30min |   |   |   |
| 6. curl\|sh uninstall removes everything |   |   |   |
| 6. round-trip (install → uninstall → install) clean |   |   |   |

**After the grid is filled:** every "block" becomes a fix before the
public announcement. Every "warn" gets a follow-up issue. Every "nit"
goes into ROADMAP.md.

---

## Honesty check

When you're done, write one sentence at the bottom of the run file
answering CLAUDE.md's forcing-function question:

> Is the demo scenario closer to working for **a stranger on the
> internet** today than it was before this dry-run?

If the answer is "no, but the pipeline ships," the release is a
mechanical milestone, not a product one. That's still progress — but
the answer goes in the postmortem queue, not in the launch tweet.
