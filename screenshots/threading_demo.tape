# Screenshot: Threaded Conversation View
# Shows flat inbox, threaded collapsed root, then expanded thread tree

Output screenshots/threading_demo.gif

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
Type "go run ./screenshots/cmd/threading_demo"
Enter
Show

Sleep 1s
Type "T"
Sleep 1s
Enter
Sleep 1s

Screenshot screenshots/threading_demo.png
