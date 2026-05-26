# shhh — audit GIF recipe (vhs)
#
# Regenerate with:
#     vhs assets/audit.tape
#
# Output: assets/audit.gif. Target ≤ 600 KB, ≤ 6 s, looping.
#
# Stage assumptions:
#  - $HOME/.claude/projects/ contains at least one project with
#    non-zero findings (i.e. shhh has been used in real sessions
#    long enough to have caught secrets).
#  - shhh is on $PATH.
#
# Pre-record check:
#     shhh audit
# should print a non-zero count. If it prints zero, find a heavier
# project directory or use `--cwd` to point at one that has leaks.

Output assets/audit.gif

Set FontSize 18
Set Width 1100
Set Height 600
Set Theme "Catppuccin Mocha"
Set Padding 24
Set TypingSpeed 40ms
Set PlaybackSpeed 1.0

# Optional setup banner.
Type "# Has Claude already seen my secrets?"
Enter
Sleep 800ms

Type "shhh audit"
Enter

# Hold on the table of findings.
Sleep 4000ms
