# 06 — Launch post (Reddit + HN + Twitter/X)

## Context

shhh is a niche but high-empathy product: every developer who's
ever piped a `.env` into Claude has a quiet "...wait, what did
that just send?" moment. The launch needs to convert that anxiety
into installs.

This brief assumes:
- Brief 02 is done — `curl | sh` works from the README.
- Brief 03 is done — README has a hero GIF and the audit teaser.
- Brief 01 is done — Read→Edit ledger is either killed or
  documented (you must not let it be the first comment).

Skip the post until those are green. A launch with a broken
install link or a Read→Edit gotcha buries the product.

## Two angles, two posts

### Angle A — "I built a tool to stop leaking secrets to Claude. Then I ran it on my history."

The viral angle from the `marketing-self-leak-hook` memory. Lead
with the **retrospective audit**, not the prospective hook.

**Why this one works:** every reader has a Claude history. They
can run `shhh audit` in 60 seconds and find their own leaks. The
post becomes a self-fulfilling viral artifact: people share their
numbers in the comments.

**Targets:** r/programming, r/devops, Hacker News (Show HN).

**Draft title (Reddit):**
> I built a tool that redacts secrets before they reach Claude Code. Then I ran it on my own history and found 47 leaks.

**Draft title (HN):**
> Show HN: shhh – redact secrets before they reach Claude / Codex / Cursor

**Draft body sketch (the post itself):**

```
A few weeks back I noticed how casually I'd been letting Claude
Code read .env files, run `printenv`, paste git diffs. The IDE
wasn't keeping any of it — but the model provider was.

So I wrote a hook that drops in front of each tool call,
detects secrets locally, and replaces them with typed
placeholders before the LLM ever sees them. Claude still
reasons normally — it sees [STRIPE_LIVE_KEY:sk_live_...:hash]
instead of the raw value.

[hero GIF / screenshot]

Then I built an audit feature that reads ~/.claude/projects/*
and tells you what's already gone out. I ran it on my own
history: 47 secrets across 19 projects. Two were live Stripe
keys; one was an AWS root.

[audit screenshot]

It's a single MIT-licensed Go binary. ~600 LOC of redaction
logic on top of gitleaks (which it links as a library and
attributes properly — `shhh licenses` prints the MIT notice).

Install: curl -fsSL https://musubi42.github.io/shhh/install.sh | sh
Repo: https://github.com/Musubi42/shhh

No network calls. No daemon. Runs locally. The detection engine
is configurable — gitleaks by default, plus a first-party
"shhh-native" layer that catches env cross-references and
preserves DB URL host/db structure while redacting creds.

If you've been using Claude Code for more than a week, I'd be
genuinely curious what shhh audit finds in your own sessions.
Report back with numbers (placeholder counts only, obviously).
```

### Angle B — "Comparing 2 secret scanners on my real codebase took one command"

Niche/techie audience that cares about secret-scanner quality.
Lead with the `shhh bench` HTML dashboard.

**Why this one works:** anyone who's tried gitleaks vs
trufflehog vs detect-secrets has stories about false-positive
floods on `go.sum` / `package-lock.json`. The bench command
gives them a one-shot answer for their codebase.

**Targets:** r/golang, infosec corners of Twitter, dev.to.

**Draft title:**
> Built a side-by-side bench for secret scanners. Ran it on my repo. gitleaks ignored 442 false positives my detector flagged on go.sum.

**Body sketch:**

```
I had two secret detection engines linked in the same Go binary:
my home-grown one and gitleaks-as-a-library. To pick a default,
I built `shhh bench` — it runs both on the same corpus and
spits out an HTML report.

[bench table screenshot]

Headline: my entropy-gate detector found 449 HIGH_ENTROPY
"secrets" in go.sum. 442 of them were Go module hashes.
gitleaks ships a default path allowlist that ignores lockfiles,
so it skipped them entirely.

That was the deciding signal. gitleaks became the default;
my detector kept around for the things gitleaks misses by
design (env cross-references, structural URL preservation).

The bench output is here:
https://github.com/Musubi42/shhh (just `shhh bench .` after
install).

The tool also runs as a hook in front of Claude Code, so the
detection isn't just a one-off scan — it gates every tool call.
But the bench dashboard is what convinced me to ship.
```

Angle B converts a smaller audience but a higher-quality one
(security/devtools people who write about tools they use).
Don't post both on the same day — pick A first, hold B for
1–2 weeks later when traffic has settled.

## Asset checklist (before posting)

- [ ] Hero GIF embedded in the README and as `og:image` on
      `web/index.html`.
- [ ] `shhh audit` output screenshot, hand-shot on YOUR machine
      (real, non-staged numbers).
- [ ] `shhh bench .` HTML dashboard screenshot (the engine
      comparison table + agreement line).
- [ ] `shhh ignore list` screenshot (the gitleaks attribution
      block — credibility cue).
- [ ] `web/index.html` reachable at the GitHub Pages URL with
      no 404s on its links.
- [ ] All install commands tested on a fresh machine within the
      last 24 hours (see brief 02 acceptance).

## Pre-post sanity check

Imagine the worst-faith commenter. They open the repo. What's
their first complaint? Common ones:

1. **"But you're sending the placeholder which is just a hash of
   the secret — what's stopping a malicious model from rainbow-
   tabling that?"** — counter: the hash is SHA1[:8] of the value
   + a session salt. It's not reversible without the salt. Also,
   the placeholder is meant to be ephemeral context, not a
   secret store. Have this explanation ready.
2. **"How is this different from running gitleaks myself?"** —
   counter: gitleaks scans files at rest. shhh redacts in-flight
   between the agent's tool call and the LLM. Different
   integration point.
3. **"What if shhh crashes / has a bug? Does my secret leak?"** —
   counter: the hook is fail-closed by design — a crash means
   the tool call fails, the model doesn't see anything. Document
   this in the README.
4. **"This is just an MCP server."** — it isn't (yet). The
   Claude Code integration is a PreToolUse hook, not MCP. Have a
   one-line explanation ready.
5. **"What about Cursor / Codex?"** — link the briefs in
   `docs/ready-to-publish/`. Honesty is the best move.

## Post timing

- **Day of post:** be available for 6 hours after posting.
  Comments roll in fast; an absentee author kills momentum.
- **Hacker News:** post in the 8–10 AM ET window on a weekday.
  HN is heavily front-page-loaded by early voters.
- **Reddit:** Sunday evening US time or Tuesday morning US time
  for r/programming. Watch for the "self-promotion" rule wording
  per subreddit — most allow open-source releases as long as the
  story is honest.
- **Twitter/X:** same day, 3–4 tweet thread. First tweet: the
  hook (the audit number). Last tweet: the install link.

## Tracking

Set up before posting:
- GitHub repo "traffic" tab is the cheap version. Save a
  pre-post baseline so you can read the delta.
- Optional: a simple goatcounter / plausible page on
  `musubi42.github.io/shhh/` for landing-page traffic.
- Save the post URLs (Reddit + HN + Twitter thread) somewhere
  permanent — useful for blog posts later.

## Files to touch

- Nothing in the codebase. This brief is purely the launch
  campaign.
- `docs/launch-2026-XX-XX.md` (new) — post-mortem write-up
  ~24h after launch. Numbers, top comments, what to fix in v2.

## Out of scope here

- Pricing, monetization, sponsorship. shhh stays MIT/free.
- Cold outreach to dev influencers — possible follow-up but
  not the launch lever.
- Localized posts (HN.fr, r/france, etc.) — same content needs
  translation, separate session.
