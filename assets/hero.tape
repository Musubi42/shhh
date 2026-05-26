# shhh — hero GIF recipe (vhs)
#
# Regenerate with:
#     vhs assets/hero.tape
#
# Output: assets/hero.gif. Target ≤ 800 KB, ≤ 8 s, looping.
#
# Stage assumptions:
#  - $HOME/demo/ exists with a .env that contains a redactable
#    Stripe-shaped key (see testdata/fixtures/leaky-project/.env
#    for a copy-able example).
#  - shhh is on $PATH and already installed into Claude Code.
#  - `claude` is on $PATH and authenticated.
#
# If any of those is missing, the recording will show errors
# instead of the value moment. Re-record after fixing the stage.

Output assets/hero.gif

Set FontSize 18
Set Width 1100
Set Height 600
Set Theme "Catppuccin Mocha"
Set Padding 24
Set TypingSpeed 40ms
Set PlaybackSpeed 1.0

# Setup line — just to anchor the viewer.
Type "cd ~/demo"
Enter
Sleep 400ms

Type "shhh install claude-code"
Enter
Sleep 800ms

Type "claude"
Enter
Sleep 1500ms

# The money shot: Claude reading .env and seeing the placeholder.
Type "read .env"
Enter
Sleep 3500ms

# Hold on the redacted output before the loop restarts.
Sleep 1500ms
