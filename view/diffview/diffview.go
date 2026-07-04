// Package diffview renders git diffs as solid blocks with uniform background
// colors and ANSI-aware truncation. Each line spans the full width of the
// code box and short lines are padded with spaces so the background color is
// continuous from edge to edge.
package diffview

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	mailpatch "github.com/floatpane/go-mailpatch"
	"github.com/floatpane/matcha/theme"
	"golang.org/x/term"
)

// Style stores the colors for a diff line type as hex strings.
type Style struct {
	Background string
	Foreground string
	IsBold     bool
}

// Styles is the set of styles for the diff view.
type Styles struct {
	Filename Style
	Divider  Style
	Context  Style
	Add      Style
	Delete   Style
	Missing  Style
}

const (
	defaultForeground = "#c9d1d9"
	langYAML          = "yaml"
)

// DefaultStyles returns a dark-theme style that matches crush's diffview.
func DefaultStyles() Styles {
	return Styles{
		Filename: Style{
			Background: "#30363d",
			Foreground: defaultForeground,
			IsBold:     true,
		},
		Divider: Style{
			Background: "#30363d",
			Foreground: "#8b949e",
			IsBold:     true,
		},
		Context: Style{
			Background: "#161b22",
			Foreground: defaultForeground,
		},
		Add: Style{
			Background: "#303a30",
			Foreground: defaultForeground,
		},
		Delete: Style{
			Background: "#3a3030",
			Foreground: defaultForeground,
		},
		Missing: Style{
			Background: "#21262d",
			Foreground: "#8b949e",
		},
	}
}

// DiffView renders parsed git diffs as solid blocks.
type DiffView struct {
	files       []mailpatch.FileChange
	width       int
	height      int
	styles      Styles
	lineNumbers bool

	beforeDigits int
	afterDigits  int
	numWidth     int
	contentWidth int
}

// New creates a DiffView with default settings.
func New() *DiffView {
	return &DiffView{
		lineNumbers: true,
		styles:      DefaultStyles(),
	}
}

// Files sets the parsed file changes to render.
func (dv *DiffView) Files(files []mailpatch.FileChange) *DiffView {
	dv.files = files
	return dv
}

// Width sets the total width for the diff view. Each line will span the full
// width and be padded with the background color so the block is solid.
func (dv *DiffView) Width(width int) *DiffView {
	dv.width = width
	return dv
}

// Height sets the max height for the diff view.
func (dv *DiffView) Height(height int) *DiffView {
	dv.height = height
	return dv
}

// Styles sets the styles for the diff view.
func (dv *DiffView) Styles(styles Styles) *DiffView {
	dv.styles = styles
	return dv
}

// LineNumbers sets whether to display line numbers.
func (dv *DiffView) LineNumbers(show bool) *DiffView {
	dv.lineNumbers = show
	return dv
}

// lineNumberDigits returns the max number of digits needed for before/after line numbers.
func (dv *DiffView) lineNumberDigits() (before, after int) {
	for _, fc := range dv.files {
		for _, h := range fc.Hunks {
			before = max(before, len(strconv.Itoa(h.OldStart+h.OldLines)))
			after = max(after, len(strconv.Itoa(h.NewStart+h.NewLines)))
		}
	}
	if before < 3 {
		before = 3
	}
	if after < 3 {
		after = 3
	}
	return
}

// tokenKind classifies a lexical span in a source code listing.
type tokenKind int

const (
	tokPlain       tokenKind = iota
	tokKeyword               // language keywords (if, for, func, return…)
	tokString                // string and character literals
	tokComment               // line and block comments
	tokNumber                // numeric literals
	tokFunction              // function/method names at call or definition
	tokType                  // type / class / capitalized identifiers
	tokPunctuation           // operators, brackets, semicolons
	tokConstant              // ALL_CAPS constants / boolean / nil literals
)

const (
	langPython     = "python"
	langJavaScript = "javascript"
	langRust       = "rust"
)

// hlStyles builds a fresh set of lipgloss styles from the active theme.
func hlStyles() map[tokenKind]lipgloss.Style {
	t := theme.ActiveTheme
	return map[tokenKind]lipgloss.Style{
		tokKeyword:     lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		tokString:      lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")),
		tokComment:     lipgloss.NewStyle().Foreground(t.Secondary).Italic(true),
		tokNumber:      lipgloss.NewStyle().Foreground(lipgloss.Color("#D19A66")),
		tokFunction:    lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF")),
		tokType:        lipgloss.NewStyle().Foreground(lipgloss.Color("#56B6C2")),
		tokPunctuation: lipgloss.NewStyle().Foreground(t.SubtleText),
		tokConstant:    lipgloss.NewStyle().Foreground(lipgloss.Color("#C678DD")),
	}
}

// hlRule applies a regex to the source; every match is highlighted with the
// rule's tokenKind. Earlier rules win on overlapping spans. When group is > 0,
// the highlighted span is restricted to that capturing submatch instead of the
// full match (used for lookahead-free function detection).
type hlRule struct {
	re    *regexp.Regexp
	kind  tokenKind
	group int
}

func mustRule(pattern string, kind tokenKind) hlRule {
	return hlRule{re: regexp.MustCompile(pattern), kind: kind}
}

func mustGroupRule(pattern string, kind tokenKind) hlRule {
	return hlRule{re: regexp.MustCompile(pattern), kind: kind, group: 1}
}

func funcRule() hlRule {
	return mustGroupRule(`\b([a-zA-Z_$][a-zA-Z0-9_$]*)\s*\(`, tokFunction)
}

func goStringRule() hlRule {
	return mustRule("`[^`]*`"+`|"(?:\\.|[^"\\])*"`+`|'(?:\\.|[^'\\])*'`, tokString)
}

func jsStringRule() hlRule {
	return mustRule("`(?:\\.|[^`\\])*`"+`|"(?:\\.|[^"\\])*"`+`|'(?:\\.|[^'\\])*'`, tokString)
}

func pyStringRule() hlRule {
	return mustRule(`"""[\s\S]*?"""|'''[\s\S]*?'''`+`|"(?:\\.|[^"\\])*"`+`|'(?:\\.|[^'\\])*'`, tokString)
}

// languageRules returns the ordered highlight rules for a language, or nil
// when the language is not recognized (the caller then renders plain code).
func languageRules(lang string) []hlRule {
	switch normalizeLang(lang) {
	case "go":
		return []hlRule{
			mustRule(`\/\/[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
			goStringRule(),
			mustRule(`\b(break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var)\b`, tokKeyword),
			mustRule(`\b(true|false|nil|iota)\b`, tokConstant),
			mustRule(`\b(bool|byte|complex64|complex128|error|float32|float64|int|int8|int16|int32|int64|rune|string|uint|uint8|uint16|uint32|uint64|uintptr|any|comparable)\b`, tokType),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			mustRule(`\b0[xX][0-9a-fA-F_]+\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,.<>=:+\-*/%&|^!?]`, tokPunctuation),
		}
	case langPython:
		return []hlRule{
			mustRule(`#[^\n]*`, tokComment),
			pyStringRule(),
			mustRule(`\b(False|None|True|And|as|assert|async|await|break|class|continue|def|del|elif|else|except|finally|for|from|global|if|import|in|is|lambda|nonlocal|not|or|pass|raise|return|try|while|with|yield|match|case)\b`, tokKeyword),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			mustRule(`\b0[xX][0-9a-fA-F_]+\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!~@]`, tokPunctuation),
		}
	case langJavaScript, "typescript":
		return []hlRule{
			mustRule(`\/\/[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
			jsStringRule(),
			mustRule(`\b(break|case|catch|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally|for|function|if|import|in|instanceof|let|new|of|return|super|switch|this|throw|try|typeof|var|void|while|with|yield|async|await|static|as|from)\b`, tokKeyword),
			mustRule(`\b(true|false|null|undefined|NaN|Infinity)\b`, tokConstant),
			mustRule(`\b(boolean|number|string|any|unknown|never|void|object|symbol|bigint)\b`, tokType),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			mustRule(`\b0[xX][0-9a-fA-F_]+\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!~?]`, tokPunctuation),
		}
	case langRust:
		return []hlRule{
			mustRule(`\/\/[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(as|async|await|break|const|continue|crate|dyn|else|enum|extern|false|fn|for|if|impl|in|let|loop|match|mod|move|mut|pub|ref|return|self|Self|static|struct|super|trait|true|type|unsafe|use|where|while)\b`, tokKeyword),
			mustRule(`\b(bool|char|f32|f64|i8|i16|i32|i64|i128|isize|str|u8|u16|u32|u64|u128|usize|String|Option|Result|Vec)\b`, tokType),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			mustRule(`\b0[xX][0-9a-fA-F_]+\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!@?]`, tokPunctuation),
		}
	case "c", "cpp", "c++", "cc", "cxx", "h", "hpp":
		return []hlRule{
			mustRule(`\/\/[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(alignas|alignof|and|asm|auto|bool|break|case|catch|char|class|const|constexpr|continue|decltype|default|delete|do|double|else|enum|explicit|export|extern|false|float|for|friend|goto|if|inline|int|long|mutable|namespace|new|noexcept|nullptr|operator|or|private|protected|public|register|reinterpret_cast|return|short|signed|sizeof|static|static_assert|static_cast|struct|switch|template|this|throw|true|try|typedef|typename|union|unsigned|using|virtual|void|volatile|while)\b`, tokKeyword),
			mustRule(`\b(int8_t|int16_t|int32_t|int64_t|uint8_t|uint16_t|uint32_t|uint64_t|size_t|ssize_t|ptrdiff_t|wchar_t|char16_t|char32_t)\b`, tokType),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?[fFuUlL]*\b`, tokNumber),
			mustRule(`\b0[xX][0-9a-fA-F_]+\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!~?]`, tokPunctuation),
		}
	case "java", "kotlin", "kt", "scala", "groovy":
		return []hlRule{
			mustRule(`\/\/[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(abstract|assert|boolean|break|byte|case|catch|char|class|const|continue|default|do|double|else|enum|extends|final|finally|float|for|goto|if|implements|import|instanceof|int|interface|long|native|new|package|private|protected|public|return|short|static|strictfp|super|switch|synchronized|this|throw|throws|transient|try|void|volatile|while|var|val|fun|when|object|data|sealed|by|as)\b`, tokKeyword),
			mustRule(`\b(true|false|null)\b`, tokConstant),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?[fFdDlL]?\b`, tokNumber),
			mustRule(`\b0[xX][0-9a-fA-F_]+\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!~?]`, tokPunctuation),
		}
	case "ruby", "rb":
		return []hlRule{
			mustRule(`#[^\n]*`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(BEGIN|END|alias|and|begin|break|case|class|def|defined\?|do|else|elsif|end|ensure|false|for|if|in|module|next|nil|not|or|redo|rescue|retry|return|self|super|then|true|undef|unless|until|when|while|yield)\b`, tokKeyword),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!@?]`, tokPunctuation),
		}
	case "bash", "sh", "shell", "zsh":
		return []hlRule{
			mustRule(`#[^\n]*`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:[^'\\])*'`, tokString),
			mustRule(`\b(if|then|else|elif|fi|for|do|done|while|until|case|esac|in|function|return|local|export|readonly|declare|typeset|unset|shift|break|continue|exit)\b`, tokKeyword),
			mustRule(`\b(true|false|null)\b`, tokConstant),
			mustRule(`\b[0-9]+\b`, tokNumber),
			mustGroupRule(`\b([a-zA-Z_][a-zA-Z0-9_-]*)\s*\(`, tokFunction),
			mustRule(`[$]\{?[A-Za-z_][A-Za-z0-9_]*\}?`, tokConstant),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!]`, tokPunctuation),
		}
	case "html", "xml", "svg":
		return []hlRule{
			mustRule(`<!--[\s\S]*?-->`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`<\/?[a-zA-Z][a-zA-Z0-9:-]*`, tokKeyword),
			mustRule(`\/?>`, tokPunctuation),
			mustGroupRule(`([a-zA-Z_:][a-zA-Z0-9_:.-]*)\s*=`, tokType),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!?]`, tokPunctuation),
		}
	case "css", "scss", "less":
		return []hlRule{
			mustRule(`\/\*[\s\S]*?\*\/`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(important|inherit|initial|unset|auto|none|inline|block|flex|grid|absolute|relative|fixed|sticky|static|hidden|visible)\b`, tokConstant),
			mustRule(`#[0-9a-fA-F]{3,8}\b`, tokNumber),
			mustRule(`\b[0-9]+(\.[0-9]+)?(px|em|rem|vh|vw|%|s|ms|deg|fr)?\b`, tokNumber),
			mustRule(`[.#][a-zA-Z_][a-zA-Z0-9_-]*`, tokType),
			mustGroupRule(`([a-zA-Z-]+)\s*:`, tokFunction),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|!]`, tokPunctuation),
		}
	case "json":
		return []hlRule{
			mustGroupRule(`("(?:\\.|[^"\\])*")\s*:`, tokType),
			mustRule(`"(?:\\.|[^"\\])*"`, tokString),
			mustRule(`\b(true|false|null)\b`, tokConstant),
			mustRule(`-?\b[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			mustRule(`[{}\[\]:,]`, tokPunctuation),
		}
	case "yaml", "yml":
		return []hlRule{
			mustRule(`#[^\n]*`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(true|false|null|yes|no|on|off)\b`, tokConstant),
			mustRule(`-?\b[0-9]+(\.[0-9]+)?\b`, tokNumber),
			mustGroupRule(`\b([a-zA-Z_][a-zA-Z0-9_.-]*)\s*:`, tokType),
			mustRule(`[:{}\[\],\-]`, tokPunctuation),
		}
	case "sql":
		return []hlRule{
			mustRule(`--[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
			mustRule(`'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(SELECT|FROM|WHERE|INSERT|INTO|UPDATE|DELETE|CREATE|TABLE|DROP|ALTER|ADD|AND|OR|NOT|NULL|PRIMARY|KEY|FOREIGN|REFERENCES|JOIN|LEFT|RIGHT|INNER|OUTER|ON|GROUP|BY|ORDER|HAVING|LIMIT|OFFSET|DISTINCT|AS|VALUES|SET|DEFAULT|CONSTRAINT|UNIQUE|INDEX|VIEW|BEGIN|COMMIT|ROLLBACK|CASE|WHEN|THEN|ELSE|END|IN|IS|LIKE|BETWEEN|EXISTS|UNION|ALL)\b`, tokKeyword),
			mustRule(`\b(INT|INTEGER|BIGINT|SMALLINT|VARCHAR|CHAR|TEXT|BOOLEAN|BOOL|DATE|TIME|TIMESTAMP|FLOAT|DOUBLE|DECIMAL|NUMERIC|SERIAL|UUID|JSON|JSONB|BLOB)\b`, tokType),
			mustRule(`\b[0-9]+(\.[0-9]+)?\b`, tokNumber),
			mustRule(`[{}()\[\];,.<>=+\-*/%]`, tokPunctuation),
		}
	case "markdown", "md":
		return []hlRule{
			mustRule(`<!--[\s\S]*?-->`, tokComment),
			mustRule(`#{1,6}\s.*$`, tokKeyword),
			mustRule("`[^`]*`", tokString),
			mustRule(`\*\*[^*]+\*\*|__[^_]+__`, tokType),
			mustRule(`\[[^\]]*\]\([^)]*\)`, tokFunction),
			mustRule(`^[>\-\*\+]\s`, tokPunctuation),
		}
	}
	return nil
}

// normalizeLang lower-cases and trims the language hint and folds common
// aliases onto canonical names so that "Python", "py", and "python" all
// resolve to the same rule set.
func normalizeLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch l {
	case "py":
		return langPython
	case "js", "jsx":
		return langJavaScript
	case "ts", "tsx":
		return "typescript"
	case "rs":
		return langRust
	case "rb":
		return "ruby"
	case "sh", "zsh":
		return "bash"
	case "yml":
		return langYAML
	case "c++", "cc", "cxx", "hpp":
		return "cpp"
	case "kt":
		return "kotlin"
	case "md":
		return "markdown"
	}
	return l
}

// highlightCode returns code with ANSI color escapes applied for the given
// language. When the language is unknown or empty the original text is
// returned unchanged. This is a local copy of view.highlightCode to avoid an
// import cycle (view imports view/diffview).
func highlightCode(code, lang string) string {
	rules := languageRules(lang)
	if rules == nil || strings.TrimSpace(code) == "" {
		return code
	}

	type span struct {
		start, end int
		ruleIdx    int
	}
	var spans []span
	for ri, r := range rules {
		for _, m := range r.re.FindAllStringSubmatchIndex(code, -1) {
			start, end := m[0], m[1]
			if r.group > 0 && r.group*2+1 < len(m) {
				if m[r.group*2] >= 0 {
					start, end = m[r.group*2], m[r.group*2+1]
				}
			}
			spans = append(spans, span{start, end, ri})
		}
	}

	bestRule := make([]int, len(code))
	for i := range bestRule {
		bestRule[i] = -1
	}
	for _, s := range spans {
		for i := s.start; i < s.end && i < len(code); i++ {
			if bestRule[i] == -1 || s.ruleIdx < bestRule[i] {
				bestRule[i] = s.ruleIdx
			}
		}
	}

	styles := hlStyles()
	var b strings.Builder
	i := 0
	for i < len(code) {
		ri := bestRule[i]
		j := i + 1
		for j < len(code) && bestRule[j] == ri {
			j++
		}
		segment := code[i:j]
		if ri < 0 {
			b.WriteString(segment)
		} else {
			b.WriteString(styles[rules[ri].kind].Render(segment))
		}
		i = j
	}
	return b.String()
}

// langFromPath maps a file path's extension to a language hint understood by
// highlightCode. Unknown or unsupported extensions yield "" (no highlight).
func langFromPath(path string) string {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	switch ext {
	case "go":
		return "go"
	case "py":
		return "py"
	case "js":
		return "js"
	case "ts":
		return "ts"
	case "rs":
		return "rs"
	case "rb":
		return "rb"
	case "sh":
		return "bash"
	case "yml":
		return langYAML
	case "yaml":
		return langYAML
	case "c":
		return "c"
	case "h":
		return "c"
	case "cpp":
		return "cpp"
	case "cc":
		return "cpp"
	case "hpp":
		return "cpp"
	case "java":
		return "java"
	case "kt":
		return "kt"
	case "scala":
		return "scala"
	case "sql":
		return "sql"
	case "html":
		return "html"
	case "xml":
		return "xml"
	case "css":
		return "css"
	case "scss":
		return "scss"
	case "json":
		return "json"
	case "md":
		return "markdown"
	case "toml", "txt":
		return ""
	}
	return ""
}

// resetRe matches all forms of the SGR reset sequence: \x1b[m, \x1b[0m,
// \x1b[00m, etc. lipgloss v2 emits \x1b[m (bare m, equivalent to \x1b[0m).
var resetRe = regexp.MustCompile(`\x1b\[0*m`)

// rewriteResets replaces every SGR reset inside highlighted text with a
// re-application of the outer diff-line style. This is the fix for the ANSI
// reset trap: lipgloss.Style.Render inserts \x1b[m after each highlighted
// token, which kills the outer background. By replacing those internal resets
// with the outer style's full SGR (attributes + foreground + background), the
// background stays continuous edge-to-edge and plain text between tokens
// renders in the correct foreground color.
func rewriteResets(text string, s Style) string {
	if !strings.Contains(text, "\x1b[") {
		return text
	}
	return resetRe.ReplaceAllString(text, outerSGR(s))
}

// outerSGR builds the SGR escape sequence for a diff-line Style so it can be
// re-emitted after an internal reset. It includes bold/italic cancellation
// (or bold if the style is bold) plus the foreground and background colors in
// truecolor (38;2;R;G;B / 48;2;R;G;B) form, matching what lipgloss emits for
// hex color strings.
func outerSGR(s Style) string {
	var parts []string
	if s.IsBold {
		parts = append(parts, "1")
	} else {
		parts = append(parts, "22") // cancel bold/dim
	}
	parts = append(parts, "23") // cancel italic
	if s.Foreground != "" {
		r, g, b := hexToRGB(s.Foreground)
		parts = append(parts, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if s.Background != "" {
		r, g, b := hexToRGB(s.Background)
		parts = append(parts, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// hexToRGB parses a #RRGGBB hex color string into its RGB components.
func hexToRGB(hex string) (r, g, b int) {
	hex = strings.TrimPrefix(hex, "#")
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0
	}
	return
}

// styleFor creates a lipgloss style for a full-width segment with the given colors.
func styleFor(s Style, width int) lipgloss.Style {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color(s.Background)).
		Foreground(lipgloss.Color(s.Foreground)).
		Width(width)
	if s.IsBold {
		style = style.Bold(true)
	}
	return style
}

// String renders the diff view as a solid block.
func (dv *DiffView) String() string {
	if len(dv.files) == 0 {
		return ""
	}

	// Compute widths
	totalWidth := dv.width
	if totalWidth <= 0 {
		cols, _, _ := getTerminalSize()
		if cols > 0 {
			totalWidth = cols - 4
		} else {
			totalWidth = 80
		}
	}
	dv.beforeDigits, dv.afterDigits = dv.lineNumberDigits()
	// Format: " {before}  {after} " (each column: leading space, digits, trailing space)
	dv.numWidth = (dv.beforeDigits + 2) + (dv.afterDigits + 2)
	dv.contentWidth = totalWidth - dv.numWidth
	if dv.contentWidth < 20 {
		dv.contentWidth = 20
	}

	var b strings.Builder
	printedLines := 0

	for fi, fc := range dv.files {
		if !dv.shouldPrint(printedLines) {
			break
		}
		if dv.height > 0 && printedLines+1 >= dv.height {
			b.WriteString(dv.renderEllipsisRow())
			b.WriteString("\n")
			break
		}
		b.WriteString(dv.renderFileHeader(fc))
		b.WriteString("\n")
		printedLines++

		// Determine the source language once per file for syntax highlighting.
		// For pure deletions there is no NewPath, so fall back to OldPath.
		lang := langFromPath(fc.NewPath)
		if lang == "" {
			lang = langFromPath(fc.OldPath)
		}

		for _, hunk := range fc.Hunks {
			if !dv.shouldPrint(printedLines) {
				break
			}
			if dv.height > 0 && printedLines+1 >= dv.height {
				b.WriteString(dv.renderEllipsisRow())
				b.WriteString("\n")
				goto done
			}
			b.WriteString(dv.renderHunkDivider(hunk))
			b.WriteString("\n")
			printedLines++

			beforeLine := hunk.OldStart
			afterLine := hunk.NewStart

			for _, line := range hunk.Lines {
				if !dv.shouldPrint(printedLines) {
					beforeLine, afterLine = dv.advanceLineNums(line, beforeLine, afterLine)
					printedLines++
					continue
				}
				if dv.height > 0 && printedLines+1 >= dv.height {
					b.WriteString(dv.renderEllipsisRow())
					b.WriteString("\n")
					goto done
				}
				b.WriteString(dv.renderLine(line, beforeLine, afterLine, lang))
				b.WriteString("\n")
				printedLines++
				beforeLine, afterLine = dv.advanceLineNums(line, beforeLine, afterLine)
			}
		}

		if fi < len(dv.files)-1 {
			b.WriteString("\n")
			printedLines++
		}
	}

done:
	return strings.TrimSuffix(b.String(), "\n")
}

func (dv *DiffView) shouldPrint(printed int) bool {
	return dv.height <= 0 || printed < dv.height
}

func (dv *DiffView) advanceLineNums(line mailpatch.Line, before, after int) (int, int) {
	switch line.Kind {
	case mailpatch.Context:
		return before + 1, after + 1
	case mailpatch.Add:
		return before, after + 1
	case mailpatch.Delete:
		return before + 1, after
	}
	return before, after
}

// lineNumbersStr returns the formatted " {before}  {after} " line number column.
func (dv *DiffView) lineNumbersStr(before, after int) string {
	beforeStr := strings.Repeat(" ", dv.beforeDigits)
	if before > 0 {
		beforeStr = fmt.Sprintf("%*d", dv.beforeDigits, before)
	}
	afterStr := strings.Repeat(" ", dv.afterDigits)
	if after > 0 {
		afterStr = fmt.Sprintf("%*d", dv.afterDigits, after)
	}
	return " " + beforeStr + "  " + afterStr + " "
}

// fullLine builds a single full-width styled line containing line numbers
// and content, with the same background color throughout so it renders as a
// solid block.
func (dv *DiffView) fullLine(s Style, before, after int, prefix, text string) string {
	var line string
	if dv.lineNumbers {
		nums := dv.lineNumbersStr(before, after)
		content := ansi.Truncate(prefix+text, dv.contentWidth, "…")
		content = fillLine(content, dv.contentWidth)
		line = nums + content
	} else {
		line = ansi.Truncate(prefix+text, dv.contentWidth, "…")
		line = fillLine(line, dv.contentWidth)
	}
	return styleFor(s, dv.width).Render(line)
}

func (dv *DiffView) renderFileHeader(fc mailpatch.FileChange) string {
	var label string
	switch fc.Type {
	case mailpatch.Modified:
		label = fc.NewPath
	case mailpatch.Added:
		label = "+++ " + fc.NewPath
	case mailpatch.Deleted:
		label = "--- " + fc.OldPath
	case mailpatch.Renamed:
		label = fc.OldPath + " → " + fc.NewPath
	case mailpatch.Copied:
		label = fc.OldPath + " ⇒ " + fc.NewPath
	default:
		label = fc.NewPath
	}
	return dv.fullLine(dv.styles.Filename, 0, 0, "  ", label)
}

func (dv *DiffView) renderHunkDivider(hunk mailpatch.Hunk) string {
	content := fmt.Sprintf("  @@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
	return dv.fullLine(dv.styles.Divider, 0, 0, "", content)
}

// cleanLineText removes trailing \r (and other control chars) that survive
// from \r\n line endings in email format-patches. The mailpatch parser splits
// on \n only, so a trailing \r would cause a carriage return in the terminal,
// garbling the rendered line and breaking the background fill.
func cleanLineText(text string) string {
	return strings.TrimRight(text, "\r")
}

func (dv *DiffView) renderLine(line mailpatch.Line, beforeLine, afterLine int, lang string) string {
	text := cleanLineText(line.Text)
	switch line.Kind {
	case mailpatch.Context:
		hl := rewriteResets(highlightCode(text, lang), dv.styles.Context)
		return dv.fullLine(dv.styles.Context, beforeLine, afterLine, "  ", hl)
	case mailpatch.Add:
		hl := rewriteResets(highlightCode(text, lang), dv.styles.Add)
		return dv.fullLine(dv.styles.Add, 0, afterLine, "+ ", hl)
	case mailpatch.Delete:
		hl := rewriteResets(highlightCode(text, lang), dv.styles.Delete)
		return dv.fullLine(dv.styles.Delete, beforeLine, 0, "- ", hl)
	}
	return ""
}

func (dv *DiffView) renderEllipsisRow() string {
	return dv.fullLine(dv.styles.Missing, 0, 0, "  ", "…")
}

// fillLine pads text with trailing spaces so it is exactly width cells wide.
// It also truncates if the text is too wide. ANSI escape codes are preserved.
func fillLine(text string, width int) string {
	w := ansi.StringWidth(text)
	if w >= width {
		return ansi.Truncate(text, width, "…")
	}
	return text + strings.Repeat(" ", width-w)
}

// RenderUnified renders parsed file changes as a solid unified diff block.
func RenderUnified(files []mailpatch.FileChange, width int) string {
	return New().Files(files).Width(width).String()
}

// getTerminalSize is duplicated here to avoid a circular dependency with the
// view package. It returns the terminal dimensions.
func getTerminalSize() (cols, rows int, ok bool) {
	s, ok := terminalSizeFrom(os.Stdin)
	if ok {
		return s.cols, s.rows, true
	}
	s, ok = terminalSizeFrom(os.Stdout)
	return s.cols, s.rows, ok
}

func terminalSizeFrom(f *os.File) (termSize, bool) {
	cols, rows, err := term.GetSize(int(f.Fd()))
	if err != nil || cols < 1 || rows < 1 {
		return termSize{}, false
	}
	return termSize{cols: cols, rows: rows}, true
}

type termSize struct {
	cols, rows int
}
