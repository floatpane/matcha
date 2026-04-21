# Screenshot: Theme Settings
# Shows the theme picker with Rose selected and a live preview

Output screenshots/theme_settings.gif

Set FontSize 14
Set FontFamily "JetBrainsMono Nerd Font"
Set Width 1400
Set Height 800
Set Theme "Catppuccin Mocha"
Set Padding 20
Set WindowBar Colorful
Set WindowBarSize 40
Set BorderRadius 10

Hide
Type "go run ."
Enter
Show

Sleep 2s

# Navigate to Settings (last item in menu)
# Inbox -> Compose -> [Drafts if present] -> Settings
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Enter

Sleep 1s

# Navigate to Theme (2nd option in settings)
Down
Sleep 300ms
Down
Sleep 300ms
Enter

Sleep 1s

# Navigate to Rose (2nd theme in list)
Down
Sleep 500ms

Screenshot screenshots/theme_settings.png
