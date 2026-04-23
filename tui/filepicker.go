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
	filePickerItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	filePickerSelectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
	directoryStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	fileSizeStyle               = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type FilePicker struct {
	cursor      int
	currentPath string
	items       []fs.DirEntry
	width       int
	height      int
	showHidden  bool
	pathInput   textinput.Model
	editingPath bool
}

func NewFilePicker(startPath string) *FilePicker {
	pi := textinput.New()
	pi.Placeholder = "Type a path and press Enter..."
	pi.Prompt = "Go to: "
	pi.CharLimit = 512
	pi.SetStyles(ThemedTextInputStyles())

	fp := &FilePicker{
		currentPath: startPath,
		pathInput:   pi,
	}
	fp.readDir()
	return fp
}

func (m *FilePicker) readDir() {
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
	m.items = files
	m.cursor = 0
}

func (m *FilePicker) Init() tea.Cmd {
	return nil
}

func (m *FilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		// Path input mode
		if m.editingPath {
			switch msg.String() {
			case "enter":
				path := m.pathInput.Value()
				if path == "" {
					m.editingPath = false
					m.pathInput.Blur()
					return m, nil
				}
				// Expand ~ to home dir
				if strings.HasPrefix(path, "~") {
					if home, err := os.UserHomeDir(); err == nil {
						path = filepath.Join(home, path[1:])
					}
				}
				info, err := os.Stat(path)
				if err == nil {
					if info.IsDir() {
						m.currentPath = path
						m.readDir()
					} else {
						// It's a file — navigate to its parent and select it
						m.currentPath = filepath.Dir(path)
						m.readDir()
					}
				}
				m.editingPath = false
				m.pathInput.Blur()
				m.pathInput.SetValue("")
				return m, nil
			case "esc":
				m.editingPath = false
				m.pathInput.Blur()
				m.pathInput.SetValue("")
				return m, nil
			}
			var cmd tea.Cmd
			m.pathInput, cmd = m.pathInput.Update(msg)
			return m, cmd
		}

		// Normal browsing mode
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "/":
			m.editingPath = true
			m.pathInput.Focus()
			return m, nil
		case "~":
			if home, err := os.UserHomeDir(); err == nil {
				m.currentPath = home
				m.readDir()
			}
		case "h":
			m.showHidden = !m.showHidden
			m.readDir()
		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			selectedItem := m.items[m.cursor]
			newPath := filepath.Join(m.currentPath, selectedItem.Name())

			if selectedItem.IsDir() {
				m.currentPath = newPath
				m.readDir()
			} else {
				return m, func() tea.Msg {
					return FileSelectedMsg{Paths: []string{newPath}}
				}
			}
		case "backspace":
			parentDir := filepath.Dir(m.currentPath)
			if parentDir != m.currentPath {
				m.currentPath = parentDir
				m.readDir()
			}
		case "esc", "q":
			return m, func() tea.Msg { return CancelFilePickerMsg{} }
		}
	}
	return m, nil
}

func formatFileSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1fG", float64(size)/(1024*1024*1024))
	}
}

func (m *FilePicker) View() tea.View {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Select a File") + "\n")
	b.WriteString(fmt.Sprintf("  %s\n", m.currentPath))

	if m.editingPath {
		b.WriteString(m.pathInput.View() + "\n")
	}

	b.WriteString("\n")

	// Calculate how many items we can show (reserve lines for header + help)
	headerLines := 3
	if m.editingPath {
		headerLines++
	}
	helpLines := 2
	visibleItems := m.height - headerLines - helpLines
	if visibleItems < 3 {
		visibleItems = 3
	}

	// Calculate scroll window
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

		itemName := item.Name()
		sizeStr := ""
		if item.IsDir() {
			itemName = directoryStyle.Render(itemName + "/")
		} else {
			if info, err := item.Info(); err == nil {
				sizeStr = fileSizeStyle.Render("  " + formatFileSize(info.Size()))
			}
		}

		line := fmt.Sprintf("%s%s%s", cursor, itemName, sizeStr)

		if m.cursor == i {
			b.WriteString(filePickerSelectedItemStyle.Render(line))
		} else {
			b.WriteString(filePickerItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	if len(m.items) == 0 {
		b.WriteString(fileSizeStyle.Render("  (empty directory)") + "\n")
	}

	hiddenLabel := "show"
	if m.showHidden {
		hiddenLabel = "hide"
	}
	b.WriteString("\n" + helpStyle.Render(fmt.Sprintf("↑/↓: navigate • enter: select • backspace: up • /: go to path • ~: home • h: %s hidden • esc: cancel", hiddenLabel)))

	return tea.NewView(docStyle.Render(b.String()))
}
