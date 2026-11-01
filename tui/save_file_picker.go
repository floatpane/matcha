package tui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	savePickerItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	savePickerSelectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
	savePickerDirectoryStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	savePickerInputStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

// SaveFileSelectedMsg is emitted when the user confirms a save path.
type SaveFileSelectedMsg struct {
	Path string
}

// SaveFilePicker is a TUI file browser for choosing a save location.
// Unlike FilePicker, it lets the user navigate directories and type a
// filename to save to (rather than selecting an existing file).
type SaveFilePicker struct {
	cursor          int
	currentPath     string
	items           []fs.DirEntry
	width           int
	height          int
	showHidden      bool
	filenameInput   textinput.Model
	editingFilename bool
}

// NewSaveFilePicker creates a save file picker starting at the given directory
// with an optional suggested filename.
func NewSaveFilePicker(startDir string, suggestedFilename string) *SaveFilePicker {
	pi := textinput.New()
	pi.Placeholder = "filename..."
	pi.Prompt = "Export as: "
	pi.CharLimit = 255
	pi.SetValue(suggestedFilename)
	pi.SetStyles(ThemedTextInputStyles())

	fp := &SaveFilePicker{
		currentPath:   startDir,
		filenameInput: pi,
	}
	fp.readDir()
	return fp
}

func (m *SaveFilePicker) readDir() {
	files, err := os.ReadDir(m.currentPath)
	if err != nil {
		m.items = []fs.DirEntry{}
		return
	}
	if !m.showHidden {
		filtered := files[:0]
		for _, f := range files {
			if !strings.HasPrefix(f.Name(), ".") {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	// Only show directories - we're choosing a save location, not selecting a file
	dirs := files[:0]
	for _, f := range files {
		if f.IsDir() {
			dirs = append(dirs, f)
		}
	}
	m.items = dirs
	m.cursor = 0
}

func (m *SaveFilePicker) Init() tea.Cmd {
	return nil
}

func (m *SaveFilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		// Filename input mode
		if m.editingFilename {
			switch msg.String() {
			case keyEnter:
				filename := m.filenameInput.Value()
				if filename == "" {
					return m, nil
				}
				savePath := filepath.Join(m.currentPath, filename)
				m.editingFilename = false
				return m, func() tea.Msg {
					return SaveFileSelectedMsg{Path: savePath}
				}
			case "esc":
				m.editingFilename = false
				return m, nil
			}
			var cmd tea.Cmd
			m.filenameInput, cmd = m.filenameInput.Update(msg)
			return m, cmd
		}

		// Normal browsing mode
		switch msg.String() {
		case "up", "k":
			if len(m.items) > 0 {
				m.cursor = (m.cursor - 1 + len(m.items)) % len(m.items)
			}
		case keyDown, "j":
			if len(m.items) > 0 {
				m.cursor = (m.cursor + 1) % len(m.items)
			}
		case "enter":
			if len(m.items) > 0 {
				selectedItem := m.items[m.cursor]
				newPath := filepath.Join(m.currentPath, selectedItem.Name())
				if selectedItem.IsDir() {
					m.currentPath = newPath
					m.readDir()
				}
			}
		case "s":
			// Start editing the filename
			m.editingFilename = true
			m.filenameInput.Focus()
			return m, nil
		case "h":
			m.showHidden = !m.showHidden
			m.readDir()
		case "backspace":
			parentDir := filepath.Dir(m.currentPath)
			if parentDir != m.currentPath {
				m.currentPath = parentDir
				m.readDir()
			}
		case "~":
			if home, err := os.UserHomeDir(); err == nil {
				m.currentPath = home
				m.readDir()
			}
		case "esc", "q":
			return m, func() tea.Msg { return CancelFilePickerMsg{} }
		}
	}
	return m, nil
}

func (m *SaveFilePicker) View() tea.View {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Export Email — Choose Save Location") + "\n")
	fmt.Fprintf(&b, "  %s\n", m.currentPath)

	if m.editingFilename {
		b.WriteString(m.filenameInput.View() + "\n")
	} else {
		filename := m.filenameInput.Value()
		if filename == "" {
			filename = "(empty)"
		}
		fmt.Fprintf(&b, "  %s%s\n", savePickerInputStyle.Render("Export as: "), filename)
	}

	b.WriteString("\n")

	// Calculate how many items we can show
	headerLines := 4
	helpLines := 2
	visibleItems := m.height - headerLines - helpLines
	if visibleItems < 3 {
		visibleItems = 3
	}

	start := 0
	if m.cursor >= visibleItems {
		start = m.cursor - visibleItems + 1
	}
	end := start + visibleItems
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := start; i < end; i++ {
		item := m.items[i]
		cursor := "  "
		if m.cursor == i {
			cursor = "> "
		}

		itemName := savePickerDirectoryStyle.Render(item.Name() + "/")

		line := fmt.Sprintf("%s%s", cursor, itemName)

		if m.cursor == i {
			b.WriteString(savePickerSelectedItemStyle.Render(line))
		} else {
			b.WriteString(savePickerItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	if len(m.items) == 0 {
		b.WriteString(savePickerInputStyle.Render("  (no subdirectories — press 's' to save here)") + "\n")
	}

	hiddenLabel := "show"
	if m.showHidden {
		hiddenLabel = "hide"
	}

	if m.editingFilename {
		b.WriteString("\n" + helpStyle.Render("enter: save • esc: cancel editing"))
	} else {
		b.WriteString("\n" + helpStyle.Render(fmt.Sprintf("↑/↓: navigate • enter: open dir • s: type filename • backspace: up • ~: home • h: %s hidden • esc: cancel", hiddenLabel)))
	}

	return tea.NewView(docStyle.Render(b.String()))
}
