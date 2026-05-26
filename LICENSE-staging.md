# LICENSE-staging.md — third-party engine attribution (draft)

This document captures the licensing, attribution and compliance
context for the third-party detection engines shhh embeds or links
against. It is a **staging** file: a working draft maintained while
the engine architecture is being finalised. Once the design lands,
the relevant parts will be promoted into the final `LICENSE`,
`NOTICE`, `THIRD_PARTY_LICENSES.md` and/or a `shhh licenses`
subcommand.

## shhh's own license

shhh is distributed under the **MIT License**, Copyright © 2026
Raphaël — Musubi SASU. See `LICENSE` at the repository root.

## Third-party detection engines

### gitleaks

- **Project**: https://github.com/gitleaks/gitleaks
- **Version embedded**: see `go.mod` (currently `github.com/zricethezav/gitleaks/v8 v8.30.1`)
- **License**: MIT License, Copyright © 2019 Zachary Rice
- **Role in shhh**: default detection engine. shhh links the
  gitleaks library through its Go module (`detect`, `config`,
  `report` packages). gitleaks is initialised in
  `internal/detector/gitleaks.go` via `detect.NewDetectorDefaultConfig()`
  and its findings are translated into shhh placeholders.

### shhh-context (shhh's native engine)

- **Project**: this repository — `internal/detector/legacy_*.go`,
  `internal/scanner/scanner.go` (the env cross-reference pass),
  and the structural URL handling in `internal/redactor`.
- **License**: same as shhh (MIT).
- **Role in shhh**: complementary engine providing two capabilities
  gitleaks does not address by design — env-var cross-reference
  ("value defined in `.env` found hardcoded elsewhere") and
  semantic redaction (`postgres://user:pwd@host/db` keeps `host/db`
  visible to the agent). May be enabled alongside, instead of, or
  in addition to gitleaks.

## License compatibility

| Component       | License | Compatible with MIT (shhh) |
|-----------------|---------|----------------------------|
| gitleaks v8     | MIT     | yes — same license         |
| shhh-context    | MIT     | yes — same project         |

MIT × MIT introduces no obligations beyond preserving copyright
notices. shhh may be linked, modified, redistributed, and (later)
sold without further constraint.

## Compliance obligations we must meet

MIT requires shhh to preserve the upstream copyright notice and the
permission text in any "copies or substantial portions" of the
embedded code. In practice, for a compiled Go binary that links
gitleaks at module level:

1. **Source distribution**: nothing extra to do. The Go module
   cache and the `vendor/` mirror already include gitleaks'
   `LICENSE` file. As long as we distribute source as a Go module,
   compliance is automatic.

2. **Binary distribution** (`install.sh`, GitHub releases,
   Homebrew, etc.): include the upstream `LICENSE` text alongside
   the binary. Recommended channels:
   - a `THIRD_PARTY_LICENSES.md` file shipped in release
     archives (lists gitleaks + any other module with a notice
     requirement), and
   - a `shhh licenses` subcommand that prints the same content,
     so users who only have the binary can inspect it.

3. **README/website**: a short attribution paragraph crediting
   gitleaks as the default detection engine, with a link to their
   repo. Not a strict legal requirement under MIT, but it is the
   community norm and matters for goodwill toward upstream.

## Operational notes

- **Pinning**: gitleaks is pinned via `go.mod`. Upgrades should be
  intentional (dependabot PR + a bench-diff run before merging).
  A regression in upstream regex would otherwise propagate
  silently through shhh.
- **No vendored fork**: shhh links gitleaks as an external Go
  module. We do not maintain a fork. If we ever need a patch,
  prefer upstreaming first; only fork if upstream rejects or
  delays.
- **Stripped allowlist (potential)**: gitleaks' default config
  (`config/gitleaks.toml`) includes a global path allowlist
  (lockfiles, vendor dirs, binaries, fonts). shhh inherits it as
  long as we call `NewDetectorDefaultConfig()`. If shhh later
  ships its own merged `.shhhignore`, this allowlist becomes the
  starting set, with shhh additions layered on top.

## Acknowledgements

shhh's redaction quality is built on gitleaks' multi-year
maintenance of secret-detection regexes. The shhh team owes the
gitleaks maintainers credit for the bulk of the pattern coverage
that ships out of the box.
