# Matcha demo — README GIF (public/assets/demo.gif).
#
# Run from the repo root:
#   make generate_gif
# which is:
#   .github/workflows/scripts/demo-sandbox.sh && vhs demo.tape
#
# Records against a throwaway $HOME with a fake Maildir inbox, so no real
# mail ever appears on screen.

Output public/assets/demo.gif

# ---------------------------------------------------------------- settings
Set Shell zsh
Set Theme "nord"
Set FontSize 16
Set FontFamily "JetBrainsMono Nerd Font"
Set Width 1200
Set Height 800
Set Padding 20
Set Framerate 30
Set PlaybackSpeed 2
Set TypingSpeed 55ms
Set CursorBlink false
Set WindowBar Colorful
Set WindowBarSize 36
Set BorderRadius 10
Set Margin 24
Set MarginFill "#242933"

# ---------------------------------------------------------------- prep (hidden)
Hide
Type "export HOME=$PWD/.demo-sandbox PATH=$PWD/.demo-sandbox/bin:$PATH PS1='%F{green}❯%f ' && clear"
Enter
Sleep 300ms
Show

# ================================================================ SCENE 1 — HOOK
Sleep 700ms
Type "matcha"
Sleep 400ms
Enter
Sleep 6.5s

# ================================================================ SCENE 2 — INBOX
Enter
Sleep 1.8s
Type@350ms "jjj"
Sleep 400ms
Type@350ms "kkk"
Sleep 500ms
Enter
Sleep 5.5s

# ================================================================ SCENE 3 — SEARCH + PALETTE
Escape
Sleep 600ms
Type "/"
Sleep 400ms
Type "from:grace"
Sleep 400ms
Enter
Sleep 1.4s
Enter
Sleep 1.8s
Escape
Sleep 700ms
Ctrl+K
Sleep 900ms
Type@90ms "thread"
Sleep 700ms
Enter
Sleep 2.2s

# ================================================================ SCENE 4 — COMPOSE
Escape
Sleep 900ms
Down
Sleep 400ms
Enter
Sleep 1s
Type "grace@compiler.dev"
Tab
Type "Re: Launch checklist for v1.0"
Tab
Type "# Build is green ✅"
Enter
Enter
Type "Demo video is **done**. Ship it!"
Sleep 2.5s
Escape
Sleep 900ms
Type "y"
Sleep 900ms

# ================================================================ SCENE 5 — PLUGIN MARKETPLACE
Down
Sleep 250ms
Down
Sleep 250ms
Down
Sleep 400ms
Enter
Sleep 500ms
# The marketplace fetches the plugin registry over the network. Pause the
# recording until it renders so load time never shows up (or shifts sync).
Hide
Wait+Screen@20s /floatpane/
Sleep 300ms
Show
Sleep 2.5s
Type@600ms "jjjjj"
Sleep 4s

# ================================================================ SCENE 6 — OUTRO / CTA
Escape
Sleep 600ms
Ctrl+C
Hide
Type "clear"
Enter
Sleep 200ms
Show
Sleep 500ms
Type "brew install floatpane/matcha/matcha  # docs.matcha.email"
Sleep 4.5s

# ---------------------------------------------------------------- clean reset (hidden)
Hide
Ctrl+U
Type "clear"
Enter
