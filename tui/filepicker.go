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
	"github.com/charmbracelet/x/ansi"
)

var (
	filePickerItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	filePickerSelectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
	directoryStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	fileSizeStyle               = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// pickerDoubleClick is the longest gap between two clicks on the same entry
// that still counts as a double click in the file pickers.
const pickerDoubleClick = 500 * time.Millisecond

type FilePicker struct {
	cursor      int
	offset      int // first visible item
	currentPath string
	items       []fs.DirEntry
	itemSizes   map[string]string
	width       int
	height      int
	showHidden  bool
	pathInput   textinput.Model
	editingPath bool

	lastClickTime time.Time
	lastClickIdx  int
}

func NewFilePicker(startPath string) *FilePicker {
	pi := textinput.New()
	pi.Placeholder = "Type a path and press Enter..."
	pi.Prompt = "Go to: "
	pi.CharLimit = 512
	pi.SetStyles(ThemedTextInputStyles())

	fp := &FilePicker{
		currentPath:  startPath,
		itemSizes:    make(map[string]string),
		pathInput:    pi,
		lastClickIdx: -1,
	}
	fp.readDir()
	return fp
}

func (m *FilePicker) readDir() {
	m.cursor = 0
	m.offset = 0
	m.lastClickIdx = -1
	files, err := os.ReadDir(m.currentPath)
	if err != nil {
		m.items = []fs.DirEntry{}
		m.itemSizes = make(map[string]string)
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
	m.itemSizes = make(map[string]string, len(files))
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if info, err := f.Info(); err == nil {
			m.itemSizes[filepath.Join(m.currentPath, f.Name())] = tfs(info.Size())
		}
	}
}

// selectName moves the cursor to the entry called name, if listed.
func (m *FilePicker) selectName(name string) bool {
	for i, item := range m.items {
		if item.Name() == name {
			m.cursor = i
			return true
		}
	}
	return false
}

func (m *FilePicker) Init() tea.Cmd {
	return nil
}

func (m *FilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if w := pickerInnerWidth(m.width); w > 0 {
			m.pathInput.SetWidth(max(1, w-lipgloss.Width(m.pathInput.Prompt)-1))
		}

	case tea.MouseWheelMsg:
		if !m.editingPath {
			m.cursor = pickerWheel(msg, m.cursor, len(m.items))
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || m.editingPath {
			return m, nil
		}
		top, start, end := m.listLayout()
		idx := start + msg.Y - top
		if msg.Y < top || idx >= end {
			return m, nil
		}
		if pickerDoubleClicked(&m.lastClickTime, &m.lastClickIdx, idx) {
			m.cursor = idx
			return m, m.openSelected()
		}
		m.cursor = idx
		return m, nil

	case tea.KeyPressMsg:
		// Path input mode
		if m.editingPath {
			return m, m.updatePathInput(msg)
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
		case keyEnter:
			return m, m.openSelected()
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

// updatePathInput handles keys while the "Go to:" path input is open: enter
// jumps to the typed directory (or to a typed file's directory, selecting
// the file), esc closes the input.
func (m *FilePicker) updatePathInput(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case keyEnter:
		path := m.pathInput.Value()
		m.editingPath = false
		m.pathInput.Blur()
		m.pathInput.SetValue("")
		if path != "" {
			m.goToPath(path)
		}
		return nil
	case "esc":
		m.editingPath = false
		m.pathInput.Blur()
		m.pathInput.SetValue("")
		return nil
	}
	var cmd tea.Cmd
	m.pathInput, cmd = m.pathInput.Update(msg)
	return cmd
}

// goToPath shows the directory at path, or the directory containing the file
// at path with that file selected. Unknown paths are ignored.
func (m *FilePicker) goToPath(path string) {
	// Expand ~ to home dir
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.IsDir() {
		m.currentPath = path
		m.readDir()
		return
	}
	// It's a file — navigate to its parent and select it
	m.currentPath = filepath.Dir(path)
	m.readDir()
	name := filepath.Base(path)
	if !m.selectName(name) && strings.HasPrefix(name, ".") {
		m.showHidden = true
		m.readDir()
		m.selectName(name)
	}
}

// openSelected enters the highlighted directory, or picks the highlighted
// file.
func (m *FilePicker) openSelected() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	selectedItem := m.items[m.cursor]
	newPath := filepath.Join(m.currentPath, selectedItem.Name())
	if selectedItem.IsDir() {
		m.currentPath = newPath
		m.readDir()
		return nil
	}
	return func() tea.Msg {
		return FileSelectedMsg{Paths: []string{newPath}}
	}
}

// headerLines renders the rows above the list (title, path, the path input
// while it is open, and a blank line).
func (m *FilePicker) headerLines() []string {
	width := pickerInnerWidth(m.width)
	lines := []string{
		fitPickerLine(titleStyle.Render("Select a File"), width),
		pickerPathLine(m.currentPath, width),
	}
	if m.editingPath {
		lines = append(lines, fitPickerLine(m.pathInput.View(), width))
	}
	return append(lines, "")
}

// helpView renders the help text wrapped to the screen.
func (m *FilePicker) helpView() string {
	hiddenLabel := "show"
	if m.showHidden {
		hiddenLabel = "hide"
	}
	text := fmt.Sprintf("↑/↓: navigate • enter: select • backspace: up • /: go to path • ~: home • h: %s hidden • esc: cancel", hiddenLabel)
	return helpStyle.Render(wrapBlock(text, pickerInnerWidth(m.width)))
}

// listLayout returns the screen row of the first visible item and the range
// of items on screen. The view and mouse handling share it.
func (m *FilePicker) listLayout() (top, start, end int) {
	header := len(m.headerLines())
	visible := len(m.items)
	if m.height > 0 {
		// One blank line separates the list from the help text.
		visible = m.height - docStyle.GetVerticalMargins() - header - 1 - blockHeight(m.helpView())
	}
	start, end = pickerWindow(&m.offset, m.cursor, len(m.items), visible)
	return docStyle.GetMarginTop() + header, start, end
}

func (m *FilePicker) View() tea.View {
	width := pickerInnerWidth(m.width)
	lines := m.headerLines()

	_, start, end := m.listLayout()
	for i := start; i < end; i++ {
		item := m.items[i]
		cursor := "  "
		if m.cursor == i {
			cursor = "▌ "
		}

		itemName := item.Name()
		sizeStr := ""
		if item.IsDir() {
			itemName = directoryStyle.Render(itemName + "/")
		} else {
			if size, ok := m.itemSizes[filepath.Join(m.currentPath, item.Name())]; ok {
				sizeStr = fileSizeStyle.Render("  " + size)
			}
		}

		line := fmt.Sprintf("%s%s%s", cursor, itemName, sizeStr)

		if m.cursor == i {
			line = filePickerSelectedItemStyle.Render(line)
		} else {
			line = filePickerItemStyle.Render(line)
		}
		lines = append(lines, fitPickerLine(line, width))
	}

	if len(m.items) == 0 {
		lines = append(lines, fitPickerLine(fileSizeStyle.Render("  (empty directory)"), width))
	}

	lines = append(lines, "", m.helpView())
	return tea.NewView(docStyle.Render(strings.Join(lines, "\n")))
}

// pickerInnerWidth is the width inside docStyle's margins (0 = unknown).
func pickerInnerWidth(width int) int {
	if width <= 0 {
		return 0
	}
	return max(1, width-docStyle.GetHorizontalMargins())
}

// fitPickerLine cuts line to width cells (0 = no limit) so it never wraps.
func fitPickerLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	return ansi.Truncate(line, width, "…")
}

// pickerPathLine renders a directory path indented, keeping its end (the
// most specific part) when it is too long for the screen.
func pickerPathLine(path string, width int) string {
	const indent = "  "
	line := indent + path
	if width <= 0 || ansi.StringWidth(line) <= width {
		return line
	}
	keep := width - len(indent)
	if keep < 2 {
		return ansi.Truncate(line, width, "…")
	}
	return indent + ansi.TruncateLeft(path, ansi.StringWidth(path)-keep+1, "…")
}

// pickerWheel moves a list cursor one entry per wheel notch, without
// wrapping around.
func pickerWheel(msg tea.MouseWheelMsg, cursor, n int) int {
	if n == 0 {
		return cursor
	}
	switch msg.Button {
	case tea.MouseWheelDown:
		return min(n-1, cursor+1)
	case tea.MouseWheelUp:
		return max(0, cursor-1)
	default:
		return cursor
	}
}

// pickerDoubleClicked records a click on entry idx and reports whether it
// completes a double click on that same entry.
func pickerDoubleClicked(lastTime *time.Time, lastIdx *int, idx int) bool {
	now := time.Now()
	double := idx == *lastIdx && now.Sub(*lastTime) < pickerDoubleClick
	*lastTime, *lastIdx = now, idx
	if double {
		*lastIdx = -1 // a third click starts a new pair
	}
	return double
}

// pickerWindow returns the range of a list of n entries to draw in visible
// rows, moving *offset only as far as needed to keep the cursor on screen.
func pickerWindow(offset *int, cursor, n, visible int) (start, end int) {
	visible = max(1, visible)
	if cursor < *offset {
		*offset = cursor
	}
	if cursor >= *offset+visible {
		*offset = cursor - visible + 1
	}
	*offset = max(0, min(*offset, n-visible))
	return *offset, min(n, *offset+visible)
}
