# Feature: Cursor hook (`shhh install cursor`)

**Status:** research-first draft. The implementation shape
depends on exactly how Cursor exposes extension points, and
specifically on how its MCP client handles MCP tool output
mutation. Do not start implementation until the research step
lands.

## Forcing function

```
$ shhh install cursor
(inside Cursor)
  > read .env
  (Cursor's agent sees placeholders, not the raw key.)
```

Same bar as milestone 1 and the Codex feature. The
`testdata/fixtures/leaky-project/.env` demo works against
Cursor's agent.

## What already exists in the codebase

- `internal/redactor`, `internal/detector`, `internal/session`,
  `internal/rules` — fully reusable.
- `cmd/shhh/cmdhook/sessionstore.go` — agent-agnostic session
  store. Reusable.
- `cmd/shhh/cmdhook/read.go` / `bash.go` — the interceptor
  helpers. The `RedactEnvFile` / `Redact` logic is reusable;
  the Claude-Code-specific JSON envelope is not.

## What's new

Cursor's most likely extension point is **MCP (Model Context
Protocol)**. That changes the shape of what we build.

### Hypothesis: shhh exposes itself as an MCP tool

Instead of intercepting Cursor's own `Read` tool (which may or
may not be hookable), shhh runs a small MCP server that Cursor
connects to. The server exposes a tool — call it
`read_file_safely` — that takes a file path, reads it, redacts
it through the existing redactor, and returns the redacted
content as the tool result.

Cursor's agent is instructed (via a system-prompt preamble,
the MCP tool's description, or a Cursor project rule) to use
`read_file_safely` instead of its built-in read tool when
opening .env or other sensitive files.

**Pros:**
- MCP output mutation *is* supported for MCP tools (this is
  the `updatedMCPToolOutput` field that was locked out for
  Claude Code's built-in tools but available for MCP).
- MCP is a clean, documented protocol.
- A working MCP server opens doors to other agents that speak
  MCP (Claude Desktop, some Codex variants).

**Cons:**
- The agent has to *choose* to use our tool. If it defaults to
  the built-in read, shhh sees nothing. Mitigation: set a
  project-level instruction that says "always use
  `read_file_safely` for files under the project root." Relies
  on the agent following instructions.
- Writing an MCP server is new territory for shhh. The library
  doesn't exist yet in this codebase.

### Alternative hypothesis: Cursor has native hooks

If Cursor exposes a hook system similar to Claude Code's, the
feature looks almost identical to the Claude Code hook —
`cmd/shhh/cmdhook/cursor.go`, a new install target. Much less
work. But I don't know whether Cursor has this.

### Alternative hypothesis: a local HTTP/stdio proxy

shhh runs as a local server that Cursor's agent talks to
directly. Every read/write goes through shhh first. This is
the "proxy daemon" the original PRD §7.3 imagined but the
postmortem explicitly pushed out of v0.1 scope.

**Recommendation:** don't do this unless forced to. The
postmortem warned against building the daemon speculatively.

## Open questions

### Q1. Research: how does Cursor expose extension points?

Same shape as the Codex research step. Read Cursor's current
docs and answer:
- Does Cursor have native hooks? (Probably no, but check.)
- How does Cursor's MCP client work? What's the tool-discovery
  mechanism? Per-project config? User config?
- Can MCP tools replace the built-in file-read tool, or do
  they exist alongside it?
- How does an MCP tool's output flow to the agent — as the
  tool result directly, or wrapped?
- What's Cursor's project config file (`.cursor/...`) and
  how is it structured?

### Q2. If MCP is the path: does shhh ship its own MCP server
or embed an existing Go MCP library?

Writing a minimal MCP server is ~200 lines of protocol code.
Embedding a library is ~0 lines but adds a dependency. Current
repo has zero non-stdlib dependencies.

My instinct: **write a minimal server** in v0.2. Drop to a
library if the protocol surface we need grows beyond a few
methods. This keeps `go.mod` lean and avoids inheriting
someone else's bugs.

### Q3. If MCP is the path: how do we guarantee the agent
actually uses our tool instead of the built-in read?

This is the soft-loop problem. Options:
- **Project rule.** Cursor lets users write project-level
  instructions that the agent is told to follow. Installer
  writes one.
- **Naming.** Name the tool something the agent will prefer
  (`read` instead of `read_file_safely`). Brittle — depends
  on tool-selection heuristics.
- **Disable the built-in read.** Possible only if Cursor
  lets users disable specific built-in tools.

We may not be able to *guarantee* anything here. The best
we can do is probably "make the safe path easy, document
the limitation." If a user reports that Cursor sometimes
bypasses shhh, that's a real bug, but it's also a signal
that the feature needs a stronger enforcement mechanism
than v0.2 can ship.

### Q4. MCP server lifecycle

Where does the MCP server run? Options:
- **Spawned per Cursor session**, as a child of Cursor. Dies
  when Cursor exits. Simple lifecycle, no daemons.
- **Long-lived daemon**, one process per user machine.
  Shared session map across Cursor sessions (and across
  other MCP-speaking agents). More capable, more failure
  modes, more lifecycle bugs. The postmortem warns against
  daemons on spec.

My vote: **per-session child process** in v0.2. Upgrade to
daemon only if a real user need appears.

## Out of scope for this feature

- **MCP for Claude Code.** Claude Code already has a hook
  mechanism that works; there's no reason to rewrite the
  Claude Code path as an MCP server just for consistency.
  MCP is a means to intercept Cursor, not a replacement for
  the hook architecture.
- **Generic MCP server as a standalone product.** If the
  shhh MCP server turns out to be useful to other agents,
  that's a growth-phase question, not a v0.2 concern. The
  MCP server in v0.2 exists to serve Cursor and nothing
  more.
- **MCP compensatory tools** (`compare_secrets`,
  `decode_jwt_safely`, etc.) that the old PRD imagined. Those
  are v0.3+ and contingent on a real user who asked for them.

## What I need from you to start implementing

Answers to Q1 (research green light), plus your gut on Q3
(how to push the agent toward our tool without being able to
force it). The rest I can decide as implementation reveals
the right answers.
