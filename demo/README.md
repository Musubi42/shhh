# shhh leak test

**Does shhh actually keep secrets out of a coding agent? Run it yourself
and check the transcript.**

This is not a benchmark of the LLM and not a simulation. It drives a
**real** Claude Code session against a fixed fake repo, twice — once with
shhh off, once with shhh on — and then greps the **real session
transcript** that Claude Code wrote to disk for the raw secret values.

The pass criterion is deterministic even though the agent is not: shhh
redacts *before* the model sees anything, so the question is never "did
the model behave" — it is "does the raw secret string appear in the
transcript". Zero = pass.

## Run it

```sh
go build -o bin/shhh ./cmd/shhh   # from the repo root, once
cd demo && ./run.sh
```

Requirements: [Claude Code](https://claude.com/claude-code) (`claude`) and
`python3` on your PATH. Each run makes real API calls (~$0.03–0.10 with
`--model sonnet`). Override the model with `SHHH_LEAKTEST_MODEL=opus`.

You get a scorecard:

```
================================================================
  shhh leak test — demo/leaktest
  7 secrets · 5 decoys · 6 files
================================================================

  shhh OFF  (control — no redaction)
    raw secrets in transcript ....  7 / 7   LEAKED

  shhh ON
    raw secrets in transcript ....  0 / 7   ✓ SAFE
    secrets redacted .............  7 / 7   (of files the agent opened)
    decoys wrongly redacted ......  1 / 5   (precision — noise, not a leak)
    files the agent opened .......  6 / 6

================================================================
  RESULT: PASS — 0 secrets reached the model with shhh on.
  precision: 1 decoy(s) redacted unnecessarily — noise, no secret leaked
================================================================
```

`RESULT` keys only on the security guarantee: 0 raw secrets in the
shhh-ON transcript. A false positive (a decoy redacted unnecessarily) is
reported as a precision number — it is noise, not a leak, and does not
flip the verdict. If the agent happens to open no secret-bearing files in
a run, the result is `INCONCLUSIVE` rather than a hollow pass.

## What's in the test

`leaktest/` is a fake payments service. Every secret in it is invented —
nothing real, safe to leak. `manifest.json` is the ground truth:

- **7 secrets** — a Stripe live key, an AWS secret, a GitHub PAT, two
  database passwords, a SendGrid key, a Google OAuth secret, spread across
  `.env`, `src/config.py`, `src/db.js`, `credentials.json`. With shhh on,
  none may appear raw in the transcript.
- **5 decoys** — a `.env.example` with obvious placeholders, a git commit
  SHA, a UUID, a literal `your-api-key-here`. These look secret-ish but are
  not. Redacting one is a **false positive**, reported as a precision
  number — noise, not a leak, so it does not fail the test.

## How verification works

`verify.py` does plain substring counting on the transcript JSONL — the
moral equivalent of `grep -F`. No regex, no LLM, no trust in the model's
narration. The scorecard prints the path to both transcripts so you can
check them yourself.

A secret counts as *redacted* when the agent opened its file (each file
carries a plain-text `LEAKTESTMARKER_*` so we know what was opened) and the
raw value is absent. A file the agent skips this run is reported as "not
tested" rather than silently scored as a pass.

## Found a leak?

That is the point — **open an issue**. The interesting gaps are tool paths
shhh does not yet cover: a secret reached via `Glob`, an MCP tool, a
`WebFetch`, or some Bash invocation the wrapper misses. A failing scorecard
with the transcript path is a perfect bug report.

## Notes / limitations

- The verifier matches raw substrings. Secrets containing `"` or `\` would
  be JSON-escaped in the transcript and could evade a naive match — the
  fixture deliberately avoids those characters.
- The agent is non-deterministic: which files it opens can vary run to
  run. The scorecard reports coverage honestly; re-run if a file was
  skipped.
- `run.sh` copies `leaktest/` into a neutral temp directory and runs the
  agent from there, so the shhh repo path and its `CLAUDE.md` never enter
  the agent's view — otherwise the agent can tell it is inside a redaction
  tool's fixture and refuse the task. It uses `--setting-sources project`
  so your global `~/.claude` config (including a globally-installed shhh
  hook) does not affect either run; the shhh-ON run gets the hook from a
  settings.json written into that temp copy. All of it is removed on exit.
- Known precision gap: shhh redacts a `ghp_`-prefixed GitHub-token
  placeholder built from 36 identical characters in `.env.example` — the
  pattern rule has no repeated-character guard. Harmless (no secret
  leaked); tracked as a precision item, not a leak.
