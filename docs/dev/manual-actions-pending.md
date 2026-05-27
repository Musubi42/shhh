# Manual actions pending

Everything in this list is a step the assistant cannot or should not
do for you. Each entry has the **exact command or copy-paste body**
to use, and what to do with the result. Order is rough priority —
do them in any order, but the ones tagged **[blocks launch]** must
happen before brief 06.

Last updated: 2026-05-26 (after brief 03 shipment).

---

## 1. Open the GitHub tracking issue for the Read→Edit ledger limit

**Why:** brief 01 (option D) shipped honest documentation of the
limit. A GitHub issue is the surface a future Anthropic engineer or
contributor finds when searching for `updatedOutput` /
`markFileAsRead` / Read-ledger.

**Where:** https://github.com/Musubi42/shhh/issues/new

**Title:**

```
Read→Edit ledger limit (upstream Claude Code hook API)
```

**Body:** copy the body block from
[`docs/ready-to-publish/01-tracking-issue-draft.md`](ready-to-publish/01-tracking-issue-draft.md)
(the "Body" section, starting with `> shhh rewrites…`). The URLs
in there are already absolute `Musubi42/shhh/blob/main/…` so they
will render correctly inside the issue.

**Labels (suggested):** `upstream-blocked`, `known-limitation`.

**After opening:** paste the issue URL into TWO files so the entry
becomes bidirectionally linked:

- `docs/known-limitations.md` → "Affected versions" section
- `docs/design/read-edit-tracking.md` → "Feedback to Anthropic"
  section

Commit those edits as `docs: link the upstream tracking issue`.

---

## 2. Send `/feedback` to Anthropic from Claude Code

**Why:** the only path that flips the ledger bug back to OPEN is
Anthropic shipping either `PostToolUse.updatedOutput` for built-in
tools or `PreToolUse.markFileAsRead`. Sending feedback raises the
chance they prioritize it.

**Where:** type `/feedback` inside any Claude Code session.

**Body:** copy the block from
[`docs/design/read-edit-tracking.md`](design/read-edit-tracking.md)
under the "Feedback to Anthropic" section (lines ~155-169, the
blockquote starting with `The hooks API lacks a way to replace…`).

Paste verbatim. The blockquote already names the use case, the
two API additions that would unblock us, and the minimal repro.

---

## 3. Generate the hero GIF and commit it **[blocks launch]**

**Why:** README's first impression. Brief 03 left a `.tape` script;
running it produces `assets/hero.gif` which becomes the hero of
the README.

**One-time setup:**

```sh
brew install charmbracelet/tap/vhs
```

(Linux/Windows: grab a binary from
https://github.com/charmbracelet/vhs/releases).

**Stage required before recording (see assets/hero.tape header for
the canonical list):**

- `$HOME/demo/` exists with a `.env` containing a Stripe-shaped
  key. Copy from `testdata/fixtures/leaky-project/.env` if needed.
- `shhh` is on `$PATH` (already true — installed via the public
  `curl | sh` earlier today).
- `claude` is on `$PATH` and authenticated.

**Generate:**

```sh
vhs assets/hero.tape       # writes assets/hero.gif
vhs assets/audit.tape      # writes assets/audit.gif (run on a
                           # machine where `shhh audit` finds >0)
```

**Verify:** each GIF should be ≤ 800 KB (hero) / ≤ 600 KB (audit)
and loop. If they blow the size, lower `Set FontSize` or trim a
`Sleep` line.

**Then in `README.md`:** add an image embed *above* the live-demo
callout near the top:

```md
![shhh in action — Claude reads .env and sees the placeholder, not the key](assets/hero.gif)
```

Optionally embed `assets/audit.gif` inside the "Wait — Claude has
probably already seen some" section.

Commit as `docs(readme): embed hero + audit recordings`.

---

## 4. Smoke-test `curl | sh` on 2 more machines **[blocks launch]**

**Why:** brief 02 acceptance asks for 3 platforms to validate the
public install path. So far: 1 ✓ (macOS arm64 on this machine).
Pending: 1 Linux + 1 second platform (Mac amd64 or a VPS).

**Test script (same on every machine):**

```sh
# Clean any prior install first, so this is a fresh-machine test:
curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | sh
which shhh        # → nothing

# Public install:
curl -fsSL https://musubi42.github.io/shhh/install.sh | sh
shhh version      # → 0.3.0
shhh licenses     # → both MIT notices (shhh + gitleaks v8.30.1)

# Wire into Claude Code if Claude is installed on that box:
shhh install claude-code
```

**Report:** if any step fails on a given platform, open a GitHub
issue with the platform string (`uname -sm` + distro) and the exact
error. The install scripts live in `web/install.sh` /
`web/install.ps1` if you want to patch directly.

---

## 5. Clean up CI cosmetic warnings (optional, before brief 06)

**Why:** ROADMAP item 6 logs two warnings the v0.3.0 release run
surfaced. The release is green, so they are cosmetic — but the
launch post will bring third-party eyes on the Actions tab.

**Two bumps to do in one chore commit:**

In `.github/workflows/release.yml`:

- Pin `goreleaser-action` to a specific major (replace `latest`
  with `~> v2` to silence the auto-lock warning):
  ```yaml
  with:
    version: '~> v2'
  ```
- Bump the action versions (or set
  `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true` at the workflow level)
  to avoid the Node 20 deprecation flag in June 2026.

Commit message: `ci: pin goreleaser to v2 + Node 24 actions`.

---

## 6. (Eventually) homebrew tap **[deferred]**

Brief 02 explicitly recommended **path B = defer** until the launch
post brings user demand. If/when commenters ask:

- Create `Musubi42/homebrew-shhh` repo.
- Uncomment the `brews:` block in `.goreleaser.yaml` (or add it per
  brief 02 §5).
- Add a `HOMEBREW_TAP_TOKEN` secret in `Musubi42/shhh` repo
  settings.
- Cut a no-op `v0.3.1` release to publish to the tap.
- Verify `brew tap Musubi42/shhh && brew install shhh` works.
- Update README install section: remove the "Homebrew tap
  deferred" line, add the `brew install` one-liner.

Full recipe: brief 02 step 5, path A.

---

## Quick "what blocks the launch post" view

| # | Action | Blocks brief 06? |
|---|---|---|
| 1 | GH tracking issue | No (nice-to-have) |
| 2 | /feedback to Anthropic | No (no user-visible effect) |
| 3 | Hero + audit GIFs | **Yes** — README without a real hero will underperform |
| 4 | 2 more smoke-test machines | **Yes** — a broken install on Linux x86 kills the post |
| 5 | CI warnings cleanup | No (cosmetic only) |
| 6 | Homebrew tap | No (explicitly deferred) |

So the **only two** items strictly blocking launch are **3** (GIFs)
and **4** (smoke-tests on Linux + one more).
