# shhh — documentation

Three top-level sections:

- **[`agents/`](agents/)** — how shhh integrates with each coding
  agent. One page per agent: commands available, which hooks are
  used, what the user sees. Read this to understand what shhh
  does in your editor.
  - [`claude-code.md`](agents/claude-code.md)
  - [`cursor.md`](agents/cursor.md)
  - [`codex.md`](agents/codex.md)

- **[`engines/`](engines/)** — the detection engines shhh can run.
  Read this when choosing or comparing engines.
  - [`gitleaks.md`](engines/gitleaks.md)
  - [`shhh-native.md`](engines/shhh-native.md)

- **[`dev/`](dev/)** — internal documentation: roadmaps, design
  decisions, postmortems, research notes, the implementation log.
  Read this if you are working on shhh itself.
