# This VHS tape file creates a demo video showcasing Matcha's features
# Run with: vhs demo.tape

# Output configuration
Output demo.gif

# Terminal settings
Set FontSize 14
Set FontFamily "JetBrainsMono Nerd Font"
Set Width 1400
Set Height 800
Set Theme "Catppuccin Mocha"
Set Padding 20
Set Framerate 30
Set PlaybackSpeed 0.7

# Window chrome
Set WindowBar Colorful
Set WindowBarSize 40
Set BorderRadius 10

# ============================================
# DEMO START
# ============================================

Sleep 500ms

# Show the command being typed but actually run "go run ."
Hide
Type "go run ."
Enter
Show

# Wait for app to load and show main menu
Sleep 2s

# ============================================
# SECTION 1: Show Menu Navigation
# ============================================

# Highlight the menu by navigating through options
Down
Sleep 600ms
Down
Sleep 600ms
Down
Sleep 600ms
Down
Sleep 600ms

# Navigate back up to "View Inbox"
Up
Sleep 400ms
Up
Sleep 400ms
Up
Sleep 400ms
Up
Sleep 600ms

# ============================================
# SECTION 2: Compose Email Demo
# ============================================

# Go to Compose Email
Down
Sleep 400ms
Enter
Sleep 1.5s

# Type the recipient
Type "friend@example.com"
Sleep 400ms
Tab
Sleep 300ms
Tab
Sleep 100ms
Tab
Sleep 200ms

# Type the subject
Type "Hello from the terminal!"
Sleep 400ms
Tab
Sleep 300ms

# Type the email body with Markdown
Type "# Greetings!"
Enter
Enter
Type "I'm sending this email from **Matcha** - "
Type "a beautiful terminal email client."
Enter
Enter
Type "Features I love:"
Enter
Type "- Keyboard-first design"
Enter
Type "- Markdown support"
Enter
Type "- Multi-account management"
Enter
Enter
Type "Try it yourself!"
Sleep 1s

# Show the completed compose form
Sleep 1.5s

# Exit without sending (for demo)
Escape
Sleep 800ms
Type "y"
Sleep 1s

# ============================================
# SECTION 3: Quick Settings Peek
# ============================================

# Navigate to Settings
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Enter

# Show settings briefly
Sleep 1.5s

# Go back to menu
Escape
Sleep 1s

# ============================================
# FINALE
# ============================================

# Show main menu one more time
Sleep 1.5s

# Exit gracefully
Ctrl+C
Sleep 800ms

# Final message
Type "# Email, right in your terminal."
Enter
Sleep 1.5s
