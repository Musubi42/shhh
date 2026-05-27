# 02 — Cut v0.3.0 release + Homebrew tap

## Context

Distribution is partially wired but stale:

- `.goreleaser.yaml` exists and builds the cross-platform matrix
  (darwin/linux/windows × amd64/arm64). `dist/` already has
  pre-built archives from a prior dry-run (`shhh_0.2.1-dev+a96eeb8_*.tar.gz`).
- `.github/workflows/release.yml` triggers on `v*` tags, runs
  tests, and invokes `goreleaser release --clean`.
- Existing releases on GitHub: `v0.1.0` (2026-05-25 15:00 UTC),
  `v0.2.0` (2026-05-25 15:16 UTC). Both predate the engine
  refactor.
- `web/install.sh` and `web/install.ps1` consume releases via
  `https://github.com/Musubi42/shhh/releases/latest`, so they
  automatically pick up new tags.
- README points users at `https://musubi42.github.io/shhh/install.sh`
  for the `curl | sh` flow — that URL must resolve and serve a
  working script before any launch post lands.
- `.github/workflows/pages.yml` exists and publishes `web/` to
  GitHub Pages. Confirm the deployment is live.
- The release workflow's leading comment notes Homebrew is
  intentionally NOT wired yet: "A Homebrew tap step can be added
  later by re-enabling the `brews:` block in .goreleaser.yaml and
  adding a HOMEBREW_TAP_TOKEN secret."

We have **5 commits since v0.2.0** (the engine refactor):
`c2b0509 88484e2 60ed106 e118bb6 4f7b1d8`. None are in a release.

## Goal

A first-time visitor on macOS or Linux runs the single-line
installer from the README, gets the latest engine-refactor binary
in `<5 seconds`, runs `shhh install claude-code`, sees the new
attribution footer (gitleaks v8.30.1 + the versioned ignore link).
Optional but high-impact: `brew install musubi42/shhh/shhh` works.

## Step-by-step

### 1. Verify the release pipeline locally

Don't trust the workflow's last green run from `v0.2.0` — the
engine refactor touched go.mod (new `sabhiram/go-gitignore` dep),
added an embedded gitleaks LICENSE, and renamed enough JSON tags
that a stale artifact would mislead.

```sh
go mod tidy && git diff go.mod go.sum   # nothing should appear
go test ./... -count=1                  # all green
goreleaser release --clean --snapshot --skip=publish
ls dist/                                # archives present, checksums.txt valid
```

If `goreleaser` is not installed:
`brew install goreleaser/tap/goreleaser`.

Open one of the dist archives and confirm:
- `shhh licenses` prints both notices (the embedded
  `gitleaks-LICENSE.txt` MUST be in the binary).
- `shhh version` outputs the snapshot tag (or v0.3.0-dev).
- `shhh ignore list` resolves the gitleaks layer cleanly.

### 2. Bump the version constant

`cmd/shhh/main.go` has `const version = "0.1.0-dev"`. Move it to
`"0.3.0"` (or whatever you pick — see versioning note below)
**before** tagging so `shhh version` matches the tag.

### 3. Tag and push

```sh
git tag -a v0.3.0 -m "v0.3.0 — gitleaks default + .shhhignore"
git push origin v0.3.0
```

The release workflow fires. Watch:
- `gh run watch` for the goreleaser job.
- After it finishes, `gh release view v0.3.0` should list 6
  archives (3 OS × 2 arches) and `checksums.txt`.

### 4. Smoke-test the public install path

On the same machine, **uninstall any existing shhh first**:

```sh
curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | sh
which shhh   # must print nothing
```

Then exercise the public install flow:

```sh
curl -fsSL https://musubi42.github.io/shhh/install.sh | sh
shhh version   # must print the new tag
shhh licenses  # must print both MIT notices
```

If the install script downloads from
`https://github.com/Musubi42/shhh/releases/latest`, the new tag
becomes "latest" automatically. Confirm by reading the script's
first 5 lines if you want to be extra sure.

### 5. (Optional but high-impact) Homebrew tap

The release.yml comment lays out the path. Two ways:

**Path A — quick, single-repo tap.**
1. Create a new GitHub repo `Musubi42/homebrew-shhh` (the prefix
   `homebrew-` is mandatory).
2. In `.goreleaser.yaml`, uncomment / add a `brews:` block
   pointing at that tap repo. The minimum shape is roughly:
   ```yaml
   brews:
     - repository:
         owner: Musubi42
         name: homebrew-shhh
       homepage: https://github.com/Musubi42/shhh
       description: Stop leaking secrets to AI coding agents.
       license: MIT
       install: |
         bin.install "shhh"
   ```
3. Create a fine-grained GitHub PAT scoped to the tap repo
   (contents:write). Add it as `HOMEBREW_TAP_TOKEN` secret in the
   main `Musubi42/shhh` repo.
4. Update `.github/workflows/release.yml` env block to forward
   `HOMEBREW_TAP_TOKEN`.
5. Cut **v0.3.1** as a no-op release to test (or re-tag — but
   re-tagging is messy, prefer a no-op bump).
6. Verify `brew tap Musubi42/shhh && brew install shhh` works.

**Path B — defer.** Keep the README's "Homebrew tap coming in a
future release" line. The `curl | sh` path is enough for v1 launch.

Recommendation: **path B** for the launch. Homebrew adds visible
polish but is not blocking. After the post lands and traffic
arrives, ship the tap if comments ask for it.

### 6. Update README install section

After v0.3.0 is live and the curl flow is verified, sweep the
README to:
- Remove the "future release" Homebrew line if path A landed.
- Add a sentence under "Install" pointing at the engine attribution
  the new install summary now prints (transparency cue for users
  who care about supply chain).

Cross-references:
- README install section currently lives at lines ~20–60 of
  `README.md`. See [`03-viral-readme.md`](03-viral-readme.md) — the
  README rewrite includes install-section adjustments; coordinate
  to avoid double-editing.

## Versioning note

`v0.2.0` shipped the per-project install + audit polish. `v0.3.0`
ships the engine refactor. The jump from minor (.2 → .3) signals
"meaningful behaviour change, breaking config schema". A `v0.2.1`
would underplay it; a `v1.0.0` would lie about stability. Stay at
0.x until the launch post brings real users and one of the open
roadmap items (Codex/Cursor support, hook bypass `.shhhtrust`)
forces a bigger version commitment.

## Acceptance

```sh
# Fresh test machine (or a VM, or a sandboxed user account):
curl -fsSL https://musubi42.github.io/shhh/install.sh | sh
shhh version                              # → 0.3.0
shhh install claude-code --engines gitleaks
# Output includes:
#   Engines active: gitleaks
#   • gitleaks v8.30.1 — MIT, https://github.com/gitleaks/gitleaks
#     Default ignore rules: https://github.com/gitleaks/gitleaks/blob/v8.30.1/config/gitleaks.toml
shhh licenses                             # both MIT notices visible
```

Three machines is the baseline (one Mac arm64, one Mac amd64 or
Linux amd64 in a VM, one VPS). If you only have one, ask a
friend to do the third.

## Files to touch

- `cmd/shhh/main.go` (`version` const)
- `.goreleaser.yaml` (only if path A on Homebrew)
- `.github/workflows/release.yml` (only if path A)
- `README.md` (install section update post-release)
- Possibly `web/install.sh` if a defect is found during smoke
  test.

## Risks to watch

- **`web/install.sh` host.** README says
  `https://musubi42.github.io/shhh/install.sh`. Confirm GitHub
  Pages is publishing `web/` and serving `install.sh` with the
  correct mime type (`application/x-sh` or `text/plain`). A
  redirect or 404 here is invisible until users hit it.
- **Embedded gitleaks LICENSE.** `cmd/shhh/cmdlicenses/gitleaks-LICENSE.txt`
  was copied from the module cache. If `go.mod` ever upgrades
  the gitleaks version, that file must be refreshed manually.
  Add a `make update-gitleaks-license` target or note this in
  CLAUDE.md so a future agent doesn't ship a stale MIT notice.
- **`dist/` already exists in the repo from prior snapshots.**
  goreleaser may complain about a non-clean tree; use `--clean`
  (already in the workflow) and confirm `.gitignore` covers
  `dist/`.
