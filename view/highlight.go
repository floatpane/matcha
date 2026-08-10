package view

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/theme"
)

// TokenKind classifies a lexical span in a source code listing.
type TokenKind int

const (
	tokPlain       TokenKind = iota
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

// highlightStyles builds a fresh set of lipgloss styles from the active theme.
// Colors are chosen so highlighted code stays readable on both light and dark
// backgrounds and harmonize with the rest of the email rendering.
func highlightStyles() map[TokenKind]lipgloss.Style {
	t := theme.ActiveTheme
	return map[TokenKind]lipgloss.Style{
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

// rule applies a regex to the source; every match is highlighted with
// the rule's TokenKind. Earlier rules win on overlapping spans. When group
// is > 0, the highlighted span is restricted to that capturing submatch
// instead of the full match (used for lookahead-free function detection,
// since Go's RE2 engine does not support lookaheads).
type rule struct {
	re    *regexp.Regexp
	kind  TokenKind
	group int
}

func mustRule(pattern string, kind TokenKind) rule {
	return rule{re: regexp.MustCompile(pattern), kind: kind}
}

func mustGroupRule(pattern string, kind TokenKind) rule {
	return rule{re: regexp.MustCompile(pattern), kind: kind, group: 1}
}

// funcRule matches an identifier followed by '(' and highlights only the
// identifier (group 1), avoiding unsupported lookahead. A leading word
// boundary prevents matching inside other identifiers.
func funcRule() rule {
	return mustGroupRule(`\b([a-zA-Z_$][a-zA-Z0-9_$]*)\s*\(`, tokFunction)
}

// goStringRule matches Go's three string forms: backtick raw strings,
// double-quoted strings, and single-quoted runes. Built via concatenation
// because Go raw-string literals cannot contain backticks.
func goStringRule() rule {
	return mustRule("`[^`]*`"+`|"(?:\\.|[^"\\])*"`+`|'(?:\\.|[^'\\])*'`, tokString)
}

// jsStringRule adds JavaScript template literals (backtick strings with
// escapes) on top of the normal quote forms.
func jsStringRule() rule {
	return mustRule("`(?:\\.|[^`\\])*`"+`|"(?:\\.|[^"\\])*"`+`|'(?:\\.|[^'\\])*'`, tokString)
}

// pyStringRule handles Python triple-quoted, double-quoted, and single-quoted strings.
func pyStringRule() rule {
	return mustRule(`"""[\s\S]*?"""|'''[\s\S]*?'''`+`|"(?:\\.|[^"\\])*"`+`|'(?:\\.|[^'\\])*'`, tokString)
}

// languageRules returns the ordered highlight rules for a language, or nil
// when the language is not recognized (the caller then renders plain code).
func languageRules(lang string) []rule {
	switch normalizeLang(lang) {
	case "go":
		return []rule{
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
		return []rule{
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
		return []rule{
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
		return []rule{
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
		return []rule{
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
		return []rule{
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
		return []rule{
			mustRule(`#[^\n]*`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(BEGIN|END|alias|and|begin|break|case|class|def|defined\?|do|else|elsif|end|ensure|false|for|if|in|module|next|nil|not|or|redo|rescue|retry|return|self|super|then|true|undef|unless|until|when|while|yield)\b`, tokKeyword),
			mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
			mustRule(`\b[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			funcRule(),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!@?]`, tokPunctuation),
		}
	case "bash", "sh", "shell", "zsh":
		return []rule{
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
		return []rule{
			mustRule(`<!--[\s\S]*?-->`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`<\/?[a-zA-Z][a-zA-Z0-9:-]*`, tokKeyword),
			mustRule(`\/?>`, tokPunctuation),
			mustGroupRule(`([a-zA-Z_:][a-zA-Z0-9_:.-]*)\s*=`, tokType),
			mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!?]`, tokPunctuation),
		}
	case "css", "scss", "less":
		return []rule{
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
		return []rule{
			mustGroupRule(`("(?:\\.|[^"\\])*")\s*:`, tokType),
			mustRule(`"(?:\\.|[^"\\])*"`, tokString),
			mustRule(`\b(true|false|null)\b`, tokConstant),
			mustRule(`-?\b[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?\b`, tokNumber),
			mustRule(`[{}\[\]:,]`, tokPunctuation),
		}
	case "yaml", "yml":
		return []rule{
			mustRule(`#[^\n]*`, tokComment),
			mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(true|false|null|yes|no|on|off)\b`, tokConstant),
			mustRule(`-?\b[0-9]+(\.[0-9]+)?\b`, tokNumber),
			mustGroupRule(`\b([a-zA-Z_][a-zA-Z0-9_.-]*)\s*:`, tokType),
			mustRule(`[:{}\[\],\-]`, tokPunctuation),
		}
	case "sql":
		return []rule{
			mustRule(`--[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
			mustRule(`'(?:\\.|[^'\\])*'`, tokString),
			mustRule(`\b(SELECT|FROM|WHERE|INSERT|INTO|UPDATE|DELETE|CREATE|TABLE|DROP|ALTER|ADD|AND|OR|NOT|NULL|PRIMARY|KEY|FOREIGN|REFERENCES|JOIN|LEFT|RIGHT|INNER|OUTER|ON|GROUP|BY|ORDER|HAVING|LIMIT|OFFSET|DISTINCT|AS|VALUES|SET|DEFAULT|CONSTRAINT|UNIQUE|INDEX|VIEW|BEGIN|COMMIT|ROLLBACK|CASE|WHEN|THEN|ELSE|END|IN|IS|LIKE|BETWEEN|EXISTS|UNION|ALL)\b`, tokKeyword),
			mustRule(`\b(INT|INTEGER|BIGINT|SMALLINT|VARCHAR|CHAR|TEXT|BOOLEAN|BOOL|DATE|TIME|TIMESTAMP|FLOAT|DOUBLE|DECIMAL|NUMERIC|SERIAL|UUID|JSON|JSONB|BLOB)\b`, tokType),
			mustRule(`\b[0-9]+(\.[0-9]+)?\b`, tokNumber),
			mustRule(`[{}()\[\];,.<>=+\-*/%]`, tokPunctuation),
		}
	case "markdown", "md":
		return []rule{
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
		return "yaml"
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
// returned unchanged so plain <pre> blocks keep their whitespace intact.
func highlightCode(code, lang string) string {
	rules := languageRules(lang)
	if rules == nil || strings.TrimSpace(code) == "" {
		return code
	}

	// Collect every match from every rule, remembering the rule's priority
	// (its index in `rules` — earlier rules win on overlap).
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

	// Coverage array: bestRule[i] = ruleIdx owning byte i, or -1.
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

	// Render: group consecutive bytes owned by the same rule.
	styles := highlightStyles()
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

// HighlightDiff applies ANSI color to a unified diff string: green for
// added lines, red for deleted lines, cyan for diff headers and hunk
// markers, and yellow for file path lines. Context lines are left plain.
func HighlightDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	addFg := lipgloss.Color("#98C379")
	delFg := lipgloss.Color("#E06C75")
	addBg := lipgloss.Color("#1a3a1a")
	delBg := lipgloss.Color("#3a1a1a")
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#56B6C2")).Bold(true)
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B"))
	markerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#C678DD")).Bold(true)
	noNewlineStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Secondary).Italic(true)
	addStyle := lipgloss.NewStyle().Foreground(addFg).Background(addBg)
	delStyle := lipgloss.NewStyle().Foreground(delFg).Background(delBg)
	addSymStyle := lipgloss.NewStyle().Foreground(addFg).Bold(true)
	delSymStyle := lipgloss.NewStyle().Foreground(delFg).Bold(true)

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		switch {
		case strings.HasPrefix(line, "diff --git"):
			b.WriteString(headerStyle.Render(line))
		case strings.HasPrefix(line, "index "):
			b.WriteString(hunkStyle.Render(line))
		case strings.HasPrefix(line, "new file mode"),
			strings.HasPrefix(line, "deleted file mode"),
			strings.HasPrefix(line, "old mode"),
			strings.HasPrefix(line, "new mode"):
			b.WriteString(markerStyle.Render(line))
		case strings.HasPrefix(line, "rename from"),
			strings.HasPrefix(line, "rename to"),
			strings.HasPrefix(line, "copy from"),
			strings.HasPrefix(line, "copy to"):
			b.WriteString(markerStyle.Render(line))
		case strings.HasPrefix(line, "similarity index"),
			strings.HasPrefix(line, "dissimilarity index"):
			b.WriteString(hunkStyle.Render(line))
		case strings.HasPrefix(line, "Binary files"):
			b.WriteString(pathStyle.Render(line))
		case strings.HasPrefix(line, "GIT binary patch"):
			b.WriteString(markerStyle.Render(line))
		case strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "+++ "):
			b.WriteString(pathStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			b.WriteString(hunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			b.WriteString(addSymStyle.Render("+") + addStyle.Render(line[1:]))
		case strings.HasPrefix(line, "-"):
			b.WriteString(delSymStyle.Render("-") + delStyle.Render(line[1:]))
		case strings.HasPrefix(line, "\\"):
			b.WriteString(noNewlineStyle.Render(line))
		default:
			b.WriteString(line)
		}
	}
	return b.String()
}
