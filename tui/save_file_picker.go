package tui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	offset          int // first visible directory
	currentPath     string
	items           []fs.DirEntry
	width           int
	height          int
	showHidden      bool
	filenameInput   textinput.Model
	editingFilename bool

	lastClickTime time.Time
	lastClickIdx  int
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
		lastClickIdx:  -1,
	}
	fp.readDir()
	return fp
}

func (m *SaveFilePicker) readDir() {
	m.cursor = 0
	m.offset = 0
	m.lastClickIdx = -1
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
}

func (m *SaveFilePicker) Init() tea.Cmd {
	return nil
}

func (m *SaveFilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if w := pickerInnerWidth(m.width); w > 0 {
			m.filenameInput.SetWidth(max(1, w-lipgloss.Width(m.filenameInput.Prompt)-1))
		}

	case tea.MouseWheelMsg:
		if !m.editingFilename {
			m.cursor = pickerWheel(msg, m.cursor, len(m.items))
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || m.editingFilename {
			return m, nil
		}
		top, start, end := m.listLayout()
		idx := start + msg.Y - top
		if msg.Y < top || idx >= end {
			return m, nil
		}
		m.cursor = idx
		if pickerDoubleClicked(&m.lastClickTime, &m.lastClickIdx, idx) {
			m.openSelected()
		}
		return m, nil

	case tea.KeyPressMsg:
		// Filename input mode
		if m.editingFilename {
			return m, m.updateFilenameInput(msg)
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
			m.openSelected()
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

// updateFilenameInput handles keys while the filename is being edited: enter
// confirms the save path, esc stops editing.
func (m *SaveFilePicker) updateFilenameInput(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case keyEnter:
		filename := m.filenameInput.Value()
		if filename == "" {
			return nil
		}
		savePath := filepath.Join(m.currentPath, filename)
		m.editingFilename = false
		return func() tea.Msg {
			return SaveFileSelectedMsg{Path: savePath}
		}
	case "esc":
		m.editingFilename = false
		return nil
	}
	var cmd tea.Cmd
	m.filenameInput, cmd = m.filenameInput.Update(msg)
	return cmd
}

// openSelected enters the highlighted directory.
func (m *SaveFilePicker) openSelected() {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return
	}
	selectedItem := m.items[m.cursor]
	if selectedItem.IsDir() {
		m.currentPath = filepath.Join(m.currentPath, selectedItem.Name())
		m.readDir()
	}
}

// headerLines renders the rows above the directory list (title, path,
// filename, and a blank line).
func (m *SaveFilePicker) headerLines() []string {
	width := pickerInnerWidth(m.width)
	lines := []string{
		fitPickerLine(titleStyle.Render("Export Email — Choose Save Location"), width),
		pickerPathLine(m.currentPath, width),
	}
	if m.editingFilename {
		lines = append(lines, fitPickerLine(m.filenameInput.View(), width))
	} else {
		filename := m.filenameInput.Value()
		if filename == "" {
			filename = "(empty)"
		}
		lines = append(lines, fitPickerLine(fmt.Sprintf("  %s%s", savePickerInputStyle.Render("Export as: "), filename), width))
	}
	return append(lines, "")
}

// helpView renders the help text wrapped to the screen.
func (m *SaveFilePicker) helpView() string {
	text := "enter: save • esc: cancel editing"
	if !m.editingFilename {
		hiddenLabel := "show"
		if m.showHidden {
			hiddenLabel = "hide"
		}
		text = fmt.Sprintf("↑/↓: navigate • enter: open dir • s: type filename • backspace: up • ~: home • h: %s hidden • esc: cancel", hiddenLabel)
	}
	return helpStyle.Render(wrapBlock(text, pickerInnerWidth(m.width)))
}

// listLayout returns the screen row of the first visible directory and the
// range of directories on screen. The view and mouse handling share it.
func (m *SaveFilePicker) listLayout() (top, start, end int) {
	header := len(m.headerLines())
	visible := len(m.items)
	if m.height > 0 {
		// One blank line separates the list from the help text.
		visible = m.height - docStyle.GetVerticalMargins() - header - 1 - blockHeight(m.helpView())
	}
	start, end = pickerWindow(&m.offset, m.cursor, len(m.items), visible)
	return docStyle.GetMarginTop() + header, start, end
}

func (m *SaveFilePicker) View() tea.View {
	width := pickerInnerWidth(m.width)
	lines := m.headerLines()

	_, start, end := m.listLayout()
	for i := start; i < end; i++ {
		item := m.items[i]
		cursor := "  "
		if m.cursor == i {
			cursor = "▌ "
		}

		line := fmt.Sprintf("%s%s", cursor, savePickerDirectoryStyle.Render(item.Name()+"/"))
		if m.cursor == i {
			line = savePickerSelectedItemStyle.Render(line)
		} else {
			line = savePickerItemStyle.Render(line)
		}
		lines = append(lines, fitPickerLine(line, width))
	}

	if len(m.items) == 0 {
		lines = append(lines, fitPickerLine(savePickerInputStyle.Render("  (no subdirectories — press 's' to save here)"), width))
	}

	lines = append(lines, "", m.helpView())
	return tea.NewView(docStyle.Render(strings.Join(lines, "\n")))
}
