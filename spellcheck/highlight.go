package spellcheck

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Red dotted underline (extended SGR sub-parameter form, supported by
// kitty, WezTerm, iTerm2, Ghostty, foot, modern xterm). Terminals that
// ignore the sub-parameters render a plain underline instead.
const (
	openSGR  = "\x1b[4:4;58:2::255:0:0m"
	closeSGR = "\x1b[4:0;59m"
)

// Highlight walks rendered text and wraps misspelled words in a red dotted
// underline. ANSI sequences already present in the input are preserved.
//
// The text is processed line by line. The line at index skipLine is left
// untouched — pass -1 to highlight every line.
func Highlight(text string, c *Checker, skipLine int) string {
	if c == nil || !c.Loaded() || text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i == skipLine {
			continue
		}
		lines[i] = highlightLine(line, c)
	}
	return strings.Join(lines, "\n")
}

type wordSpan struct {
	start, end int
	word       string
}

func highlightLine(line string, c *Checker) string {
	if line == "" {
		return line
	}

	spans := scanWords(line)
	if len(spans) == 0 {
		return line
	}

	// Splice from end to start so earlier offsets stay valid.
	out := line
	wrapped := false
	for i := len(spans) - 1; i >= 0; i-- {
		s := spans[i]
		if !IsCheckable(s.word) {
			continue
		}
		if c.Check(s.word) {
			continue
		}
		out = out[:s.start] + openSGR + out[s.start:s.end] + closeSGR + out[s.end:]
		wrapped = true
	}
	if !wrapped {
		return line
	}
	return out
}

// scanWords walks the raw line and returns word runs by byte offset.
// ANSI CSI/OSC escape sequences are skipped so they don't fragment words.
func scanWords(line string) []wordSpan {
	var spans []wordSpan
	var b strings.Builder
	start := -1

	flush := func() {
		if b.Len() == 0 {
			return
		}
		w := strings.TrimRight(b.String(), "'’-")
		if w != "" {
			spans = append(spans, wordSpan{start: start, end: start + len(w), word: w})
		}
		b.Reset()
		start = -1
	}

	i := 0
	for i < len(line) {
		if line[i] == 0x1b {
			flush()
			i += ansiSkip(line, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if unicode.IsLetter(r) {
			if start < 0 {
				start = i
			}
			b.WriteRune(r)
			i += size
			continue
		}
		if b.Len() > 0 && (r == '\'' || r == '’' || r == '-') {
			b.WriteRune(r)
			i += size
			continue
		}
		flush()
		i += size
	}
	flush()
	return spans
}

// ansiSkip returns the byte length of the escape sequence beginning at
// line[i] (which must be ESC). Malformed/truncated sequences consume the
// remainder of the line.
func ansiSkip(line string, i int) int {
	if i+1 >= len(line) {
		return 1
	}
	switch line[i+1] {
	case '[':
		// CSI: ESC [ params final (0x40..0x7e)
		j := i + 2
		for j < len(line) {
			c := line[j]
			if c >= 0x40 && c <= 0x7e {
				return j - i + 1
			}
			j++
		}
		return len(line) - i
	case ']':
		// OSC: terminated by BEL or ST (ESC \).
		j := i + 2
		for j < len(line) {
			if line[j] == 0x07 {
				return j - i + 1
			}
			if line[j] == 0x1b && j+1 < len(line) && line[j+1] == '\\' {
				return j - i + 2
			}
			j++
		}
		return len(line) - i
	default:
		return 2
	}
}
