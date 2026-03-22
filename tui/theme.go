package tui

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/theme"
)

// RebuildStyles updates all package-level style variables to match the active theme.
// This must be called after theme.SetTheme() and at startup.
func RebuildStyles() {
	t := theme.ActiveTheme

	// styles.go
	DialogBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.AccentDark).
		Padding(1, 2).
		BorderTop(true).
		BorderLeft(true).
		BorderRight(true).
		BorderBottom(true)

	HelpStyle = lipgloss.NewStyle().Foreground(t.Secondary)
	TipStyle = lipgloss.NewStyle().Foreground(t.Tip).Italic(true)
	SuccessStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	InfoStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	H1Style = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true).
		Align(lipgloss.Center)

	H2Style = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(false).
		Align(lipgloss.Center)

	// choice.go
	titleStyle = lipgloss.NewStyle().Foreground(t.AccentText).Background(t.AccentDark).Padding(0, 1)
	logoStyle = lipgloss.NewStyle().Foreground(t.Accent)
	listHeader = lipgloss.NewStyle().Foreground(t.SubtleText).PaddingBottom(1)
	itemStyle = lipgloss.NewStyle().PaddingLeft(2)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(t.Accent)

	// settings.go
	accountItemStyle = lipgloss.NewStyle().PaddingLeft(2)
	selectedAccountItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(t.Accent)
	accountEmailStyle = lipgloss.NewStyle().Foreground(t.Secondary)
	dangerStyle = lipgloss.NewStyle().Foreground(t.Danger)
	settingsFocusedStyle = lipgloss.NewStyle().Foreground(t.Accent)
	settingsBlurredStyle = lipgloss.NewStyle().Foreground(t.Secondary)

	// composer.go
	suggestionStyle = lipgloss.NewStyle().Foreground(t.Secondary)
	selectedSuggestionStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	suggestionBoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Secondary).Padding(0, 1)
	focusedStyle = lipgloss.NewStyle().Foreground(t.Accent)
	blurredStyle = lipgloss.NewStyle().Foreground(t.Secondary)
	noStyle = lipgloss.NewStyle()
	helpStyle = lipgloss.NewStyle().Foreground(t.SubtleText)
	focusedButton = focusedStyle.Render("[ Send ]")
	blurredButton = blurredStyle.Render("[ Send ]")
	emailRecipientStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	attachmentStyle = lipgloss.NewStyle().PaddingLeft(4).Foreground(t.Secondary)
	fromSelectorStyle = lipgloss.NewStyle().Foreground(t.Accent)
	smimeToggleStyle = lipgloss.NewStyle().PaddingLeft(4).Foreground(t.Secondary)

	// inbox.go
	tabStyle = lipgloss.NewStyle().Padding(0, 2)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 2).Foreground(t.Accent).Bold(true).Underline(true)
	tabBarStyle = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).PaddingBottom(1).MarginBottom(1)
	dateStyle = lipgloss.NewStyle().Foreground(t.MutedText)
	unreadEmailStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	readEmailStyle = lipgloss.NewStyle().Foreground(t.Secondary)

	// folder_inbox.go
	sidebarStyle = lipgloss.NewStyle().
		Width(sidebarWidth).
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		PaddingRight(1).
		PaddingLeft(1)
	sidebarTitleStyle = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true).
		PaddingBottom(1)
	folderStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1)
	activeFolderStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1).
		Background(t.Accent).
		Foreground(t.Contrast).
		Bold(true)
	moveOverlayStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.AccentDark).
		Padding(1, 2)
	moveOverlayTitleStyle = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true).
		PaddingBottom(1)
	moveItemStyle = lipgloss.NewStyle().
		PaddingLeft(1)
	moveSelectedItemStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		Foreground(t.Accent).
		Bold(true)

	// filepicker.go
	filePickerItemStyle = lipgloss.NewStyle().PaddingLeft(2)
	filePickerSelectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(t.Accent)
	directoryStyle = lipgloss.NewStyle().Foreground(t.Directory)
	fileSizeStyle = lipgloss.NewStyle().Foreground(t.Secondary)

	// trash_archive.go
	mailboxTabStyle = lipgloss.NewStyle().Padding(0, 3)
	activeMailboxTabStyle = lipgloss.NewStyle().Padding(0, 3).Foreground(t.Accent).Bold(true).Underline(true)
}

// ThemedTextInputStyles returns textinput.Styles using the active theme colors.
func ThemedTextInputStyles() textinput.Styles {
	t := theme.ActiveTheme
	return textinput.Styles{
		Focused: textinput.StyleState{
			Placeholder: lipgloss.NewStyle().Foreground(t.DimText),
			Suggestion:  lipgloss.NewStyle().Foreground(t.DimText),
			Prompt:      lipgloss.NewStyle().Foreground(t.AccentText),
			Text:        lipgloss.NewStyle(),
		},
		Blurred: textinput.StyleState{
			Placeholder: lipgloss.NewStyle().Foreground(t.DimText),
			Suggestion:  lipgloss.NewStyle().Foreground(t.DimText),
			Prompt:      lipgloss.NewStyle().Foreground(t.MutedText),
			Text:        lipgloss.NewStyle().Foreground(t.DimText),
		},
		Cursor: textinput.CursorStyle{
			Color: t.AccentText,
			Shape: tea.CursorBlock,
			Blink: true,
		},
	}
}

// ThemedTextAreaStyles returns textarea.Styles using the active theme colors.
func ThemedTextAreaStyles() textarea.Styles {
	t := theme.ActiveTheme
	return textarea.Styles{
		Focused: textarea.StyleState{
			Base:             lipgloss.NewStyle(),
			CursorLine:       lipgloss.NewStyle(),
			CursorLineNumber: lipgloss.NewStyle().Foreground(t.MutedText),
			EndOfBuffer:      lipgloss.NewStyle().Foreground(t.Secondary),
			LineNumber:       lipgloss.NewStyle().Foreground(t.MutedText),
			Placeholder:      lipgloss.NewStyle().Foreground(t.DimText),
			Prompt:           lipgloss.NewStyle().Foreground(t.AccentText),
			Text:             lipgloss.NewStyle(),
		},
		Blurred: textarea.StyleState{
			Base:             lipgloss.NewStyle(),
			CursorLine:       lipgloss.NewStyle().Foreground(t.DimText),
			CursorLineNumber: lipgloss.NewStyle().Foreground(t.MutedText),
			EndOfBuffer:      lipgloss.NewStyle().Foreground(t.Secondary),
			LineNumber:       lipgloss.NewStyle().Foreground(t.MutedText),
			Placeholder:      lipgloss.NewStyle().Foreground(t.DimText),
			Prompt:           lipgloss.NewStyle().Foreground(t.MutedText),
			Text:             lipgloss.NewStyle().Foreground(t.DimText),
		},
		Cursor: textarea.CursorStyle{
			Color: t.AccentText,
			Shape: tea.CursorBlock,
			Blink: true,
		},
	}
}
