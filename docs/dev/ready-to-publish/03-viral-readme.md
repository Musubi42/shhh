# 03 — Viral README rewrite

## Context

The current `README.md` (149 lines) is **informative but flat**.
It tells the reader what shhh is, lists install commands, and
links to PRD/CLAUDE.md. It does not:

- Show a screenshot or animated demo in the first 30 seconds.
- Lead with the **emotional hook** (you are leaking secrets to
  Claude right now).
- Surface the `shhh audit` retrospective — the most viral feature.
- Reflect the engine architecture that just shipped (gitleaks
  default, `.shhhignore` cascade, multi-engine, `shhh ignore`
  subcommand).

There is already a viral landing page at `web/index.html`
(commit `61704d1` — "viral demo page — real transcript diff,
shhh off vs on"). Reuse the assets and copy from there where
the framing is already good.

## Goal

A first-time visitor on the GitHub repo page sees, in this order:

1. **The pitch in one sentence** — already there, sharpen if needed.
2. **A 4-second GIF or static screenshot of the proof** — Claude
   reading `.env`, seeing `[STRIPE_LIVE_KEY:sk_live_...]`. This is
   the moment the value of shhh becomes obvious. Without it the
   reader bounces.
3. **The audit hook in two lines of code** — `shhh audit` prints
   "47 leaks found in your existing Claude history". Most readers
   stop here and run it on their own machine. That's the goal.
4. **Install in one line**, the engine attribution as a
   "transparent by default" cue, then the rest below the fold.

## What to keep, what to add

### Keep
- The MIT statement at the bottom.
- The `Wire it into your agent` section is fine but should move
  below the demo.
- The "shhh runs locally, no network calls" trust paragraph —
  critical, often the first comment on a Reddit thread.

### Add
1. **Hero image / GIF.** The `web/index.html` page has a static
   transcript diff already prepared (left: Claude sees the raw
   key; right: Claude sees the placeholder). Capture that as a
   `.gif` (preferred) or a high-DPI `.png` and commit to
   `assets/hero.gif` or similar. Embed in README at the very top:
   ```md
   ![shhh in action: Claude reads .env with and without shhh](assets/hero.gif)
   ```
   GIF making: `vhs` (charm) or QuickTime → ffmpeg → gif. Target
   ≤ 800 KB, ≤ 8 seconds, looping. README hero GIFs over 1 MB
   fail on mobile.

2. **The audit hook**. After the hero, before install:
   ```md
   ## Wait — has Claude already seen your secrets?

   ```sh
   shhh audit
   ```

   shhh reads `~/.claude/projects/*` and tells you how many
   secrets are already in your Claude history. Most users find
   between 5 and 50. None of them is fun to rotate.
   ```
   Optionally a second screenshot of the audit terminal output
   here. The numbers are real and shocking by themselves.

3. **The engine selector + ignore inspector**, briefly. Two
   one-liners under "See what shhh is doing":
   ```sh
   shhh ignore list        # show the active .shhhignore cascade
   shhh bench .            # compare detection engines on this repo
   ```

4. **Known limitations** (only if option D was chosen in
   [`01-kill-read-edit-ledger-bug.md`](01-kill-read-edit-ledger-bug.md)).

### Cut or shrink
- The `## Why this exists` block can shrink to two sentences;
  the demo replaces most of the explanation.
- `## Development` can move to the bottom or to `CONTRIBUTING.md`.
- The PRD link is one click off, that's fine; don't lead with it.

## Reference structure

Aim for ~80 lines, not 149. Below is the target shape — adjust
copy as needed but keep the order:

```
# shhh

> Stop leaking secrets to AI coding agents.

![hero GIF]

shhh redacts secrets — API keys, tokens, connection strings —
before they reach the LLM. The agent sees typed placeholders
([STRIPE_LIVE_KEY:sk_live_...]) and keeps reasoning. The raw
value never leaves your machine.

```
$ shhh install claude-code
$ claude
claude> read .env
  # Claude sees: STRIPE_LIVE_KEY=[STRIPE_LIVE_KEY:sk_live_...:b4135099]
  # Not the raw key.
```

---

## Wait — Claude has probably already seen some.

```
$ shhh audit
```

shhh scans `~/.claude/projects/` and reports every secret that's
already left your machine. Most users find 5–50.

[optional screenshot of audit output]

---

## Install

curl ...
go install ...
homebrew (if shipped) ...

shhh ships with two detection engines:
- **gitleaks** (default, MIT, ~222 rules)
- **shhh-native** (env cross-reference + structural URL preservation)

Pick one or both at install time.

---

## See what shhh is doing

shhh scan .          # secrets in the current directory
shhh audit           # forensic audit of Claude history
shhh ignore list     # active .shhhignore cascade (with gitleaks defaults link)
shhh bench .         # compare engines on real content

---

## Trust

shhh runs entirely locally. No network calls. No daemon. Single
Go binary, MIT, ~600 LOC of redaction logic on top of the
gitleaks library. Run `shhh licenses` to see the full
attribution.

---

## Known limitations (only under option D)

[Read→Edit ledger paragraph]

---

## Development / License / Links

[the existing tail block, condensed]
```

## Concrete asset checklist

- [ ] `assets/hero.gif` (≤ 800 KB, ≤ 8 s, loop).
- [ ] `assets/audit-screenshot.png` (high-DPI, 1200 wide ish,
      transparent terminal background to look pro).
- [ ] Optional: `assets/bench-screenshot.png` showing the
      3-engine bench table.
- [ ] Optional: `assets/ignore-list-screenshot.png` showing
      `shhh ignore list` with the gitleaks attribution.

If you don't have a video tool you like, the static screenshot
of the transcript diff is acceptable. The audit screenshot is
**not** optional — it's the carry-the-post asset.

## Recording recipe (terminal GIF)

```sh
# Install vhs once:
brew install charmbracelet/tap/vhs

# Write a vhs script: shhh-hero.tape
cat > shhh-hero.tape <<'EOF'
Output assets/hero.gif
Set FontSize 18
Set Width 1200
Set Height 600
Set Theme "Catppuccin Mocha"
Type 'shhh install claude-code'
Enter
Sleep 1s
Type 'claude'
Enter
# ... etc
EOF

vhs shhh-hero.tape
```

Alternative: record with QuickTime, convert with
`ffmpeg -i recording.mov -vf "fps=12,scale=900:-1" assets/hero.gif`.
GIF dithering on terminal cell-aligned text usually looks fine.

## Acceptance

Open the rendered README on GitHub (push a branch first if you
want to preview). Time yourself reading it from the top until
you understand what shhh does and how to install it. Target:
**under 15 seconds**. If your eyes hit a wall of prose before the
GIF or the audit teaser, the structure is wrong.

Cross-check on mobile (GitHub renders narrower; long lines wrap).
The hero GIF should not blow the width.

## Files to touch

- `README.md` — full rewrite per the shape above.
- `assets/` — new directory, commit hero GIF + screenshots.
- Possibly `web/index.html` — keep the long-form pitch there;
  the README can link to it for "see the full story".
