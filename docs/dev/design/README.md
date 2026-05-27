# shhh — design language

This folder holds the visual and structural design for shhh's user-
facing surfaces: the `shhh audit` HTML report, the terminal output of
`shhh audit`, and the interactive installer TUI.

If you're here to implement, the source-of-truth file is
[`implementation-plan.md`](./implementation-plan.md). The mockups
under [`mockups/`](./mockups/) are the visual references the
implementation must match. The markdown files at this level document
the aesthetic rationale and the terminal surfaces (which can't be
expressed as HTML).

## Commit

shhh is a security tool that reports bad news. Its design has to feel
**serious without being corporate** and **alive without being playful**.
It should look like something a developer would take seriously at 3 AM
when they're rotating keys after a breach — not like a marketing
landing page.

The commit is: **brutalist editorial dark**. Think forensic report,
not dashboard. Think declassified document, not SaaS.

## Typography

| Role | Family | Weights | Why |
|---|---|---|---|
| Display (wordmark, counters, headers) | **Redaction** | 400, 700 | Serif literally designed by Forensic Architecture for censored documents. The family ships with progressive degradation variants (Redaction 10, 20, 35, 50, 70, 100) that simulate photocopier wear. No other font on earth is this thematically on-the-nose for a secret-redaction tool. |
| Display — "censored" variant | **Redaction 35** | 400 | Used sparingly on the wordmark tail and on placeholder tokens to visually reinforce the concept of redaction without hurting readability. |
| Body / code / paths | **Geist Mono** | 300, 400, 500, 600 | Vercel's monospace, modern but not overused. Pairs cleanly with Redaction's serif contrast. Good tabular-numbers support for the counters. |

Both families are available on Google Fonts. The production HTML
report loads them via a single `<link>` in `<head>`; offline use-cases
fall back to `ui-monospace, SFMono-Regular, monospace` and
`ui-serif, Georgia, serif`.

**NEVER** replace these with Inter, Roboto, Arial, or the system
font stack. shhh's visual identity is the typography — if you swap
it out, you've erased the product.

## Color

Dark theme is primary and non-negotiable. A light theme may come
later; it is not in scope for v0.2.

```
--bg:           #0a0a0b    /* near-black, slight cool cast          */
--bg-raised:    #141416    /* hover states, inset panels            */
--fg:           #f5f1e8    /* warm off-white (aged paper)           */
--fg-dim:       #8a8680    /* secondary text, labels                */
--rule:         #2a2a2e    /* hairline separators                   */

--leaked:       #ff4a1c    /* 🚨 already sent to Claude             */
--leaked-bg:    rgba(255, 74, 28, 0.08)
--at-risk:      #f5c518    /* ⚠️ present in files, not yet leaked   */
--at-risk-bg:   rgba(245, 197, 24, 0.06)
--protected:    #50c878    /* ✅ shhh is installed here              */
--protected-bg: rgba(80, 200, 120, 0.06)
--archive:      #8a8680    /* project folder gone, transcript kept  */
--accent:       #3a5bff    /* interactive links, focus rings        */
```

The color hierarchy is deliberate: `leaked` is the loudest because it
represents the only "rotate now, non-negotiable" state. `at-risk`
warns without screaming. `protected` is muted green — we don't want
to make "everything is fine" feel celebratory; it's the *absence* of
danger, not a win.

## Layout principles

1. **Hard edges, no rounded corners.** Border-radius is zero
   everywhere. shhh is an infra tool, not a consumer app.
2. **Hairline rules over boxes.** Sections separate with 1px lines
   (`var(--rule)`), not cards with shadows and padding.
3. **Dense information, generous vertical rhythm.** Terminal-heritage
   density horizontally, editorial spacing vertically between
   sections.
4. **Monospace for anything machine-readable.** Paths, secret
   placeholders, commands, timestamps — all in Geist Mono. Prose and
   headings can use Redaction.
5. **Hover states matter.** Rows shift background and/or left-padding
   on hover. Subtle but confirms interactivity.
6. **Noise/grain overlay.** A faint SVG fractal-noise overlay on
   `body::before` at 4% opacity. Breaks up the flat dark fields and
   gives the page a slight analog texture that matches the
   "photocopied forensic report" feel.

## Motion

Conservative. One orchestrated page-load reveal is worth more than
twenty scattered micro-interactions.

- **Page load**: staggered fade-up (`@keyframes reveal`,
  `translateY(8px) → 0`, 600ms ease, delays from 0 to ~500ms across
  the hierarchy header → counters → delta → section title →
  projects → footer).
- **Hover on project rows**: background shift + 4px left padding,
  150ms.
- **`unprotected` badge**: slow 2s pulse on the border color
  (subtle — not a seizure trigger). Reinforces urgency without
  shouting.
- **No scroll-triggered animations.** The page is short enough that
  everything meaningful is visible on load; scroll effects would
  feel gratuitous.

## Grain and atmosphere

The noise overlay is inline SVG, zero network cost, generated by
`feTurbulence` with `baseFrequency=0.9` at 4% opacity. It does not
scroll (fixed position) so it reads as a film grain on the report
itself, not on the content. Similar in principle to a dust layer on a
declassified PDF scan.

## What the mockups show vs. what ships

The HTML mockups in [`mockups/`](./mockups/) are **pixel-accurate
representations** of what the v0.2 implementation must render. They
use:

- Google Fonts CDN for Redaction and Geist Mono
- Inlined CSS in a single `<style>` block
- Inlined SVG for the grain texture
- Vanilla HTML with zero JS dependencies (one small `<script>` for
  the expand-on-click interaction in `project-detail.html`)

The actual shhh binary will generate HTML using Go's `html/template`
and emit files that structurally match these mockups. The only
difference: production HTML embeds the fonts as base64 data URIs in
the `<style>` block so the report works offline (no CDN dependency).

## Structural tour

```
docs/design/
├── README.md                       ← you are here
├── installer-tui.md                ← wireframes for `shhh install` prompts
├── cli-output.md                   ← mockup for `shhh audit` terminal output
├── implementation-plan.md          ← the consolidated build doc
└── mockups/
    ├── overview.html               ← `shhh audit` HTML report, main page
    └── project-detail.html         ← click a project → drill-down page
```

Open the `.html` files directly in a browser — they're
self-contained.
