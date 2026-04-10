# Screenshot: Settings Screen
# Shows the settings / account management view

Output screenshots/settings.gif

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

# Navigate to Settings (3rd item if no drafts, 4th if drafts exist)
# In CI we create dummy drafts, so: Inbox -> Compose -> Drafts -> Settings
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Enter

Sleep 1.5s

Screenshot screenshots/settings.png
