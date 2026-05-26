# assets/

README hero GIF + supplementary screenshots, generated from the
`.tape` scripts in this directory via [charmbracelet/vhs](https://github.com/charmbracelet/vhs).

## One-time setup

```sh
brew install charmbracelet/tap/vhs
```

(or see vhs releases for Linux / Windows binaries).

## Regenerate the hero GIF

```sh
vhs assets/hero.tape
# → writes assets/hero.gif
```

The hero GIF is referenced from the top of `README.md`. Keep it
**under 800 KB** and **under 8 seconds** so it loads fast on
mobile.

## Regenerate the audit screenshot

```sh
vhs assets/audit.tape
# → writes assets/audit.gif
```

The audit recording shows `shhh audit` running on a real
`~/.claude/projects/` directory with non-zero findings. Run this
on a machine where shhh has been used for a while so the numbers
are non-trivial.

## Why `.tape` and not a real recording

`.tape` files are deterministic: a future contributor can
regenerate the same GIF after a bug-fix or UI tweak without
having to set up a stage. If you record with QuickTime or asciinema
once, the next person can't reproduce.
