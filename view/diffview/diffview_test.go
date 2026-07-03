package diffview

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	mailpatch "github.com/floatpane/go-mailpatch"
)

var testResetRe = regexp.MustCompile(`\x1b\[0*m`)

// TestRenderLineNoNewlines verifies constraint 1: one line = one line.
func TestRenderLineNoNewlines(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 3,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "func main() {"},
				{Kind: mailpatch.Add, Text: "\tfmt.Println(\"hello world\")"},
				{Kind: mailpatch.Add, Text: "}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d: %q", len(lines), out)
	}
}

// TestRenderLineFullWidth verifies constraint 2: edge-to-edge background
// and exact visible width.
func TestRenderLineFullWidth(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 1,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "func main() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	lines := strings.Split(out, "\n")
	addLine := lines[len(lines)-1]
	w := ansi.StringWidth(addLine)
	if w != 80 {
		t.Errorf("add line width = %d, want 80. line: %q", w, addLine)
	}
}

// TestRenderLineBackgroundContinuity verifies constraint 2+3: the background
// color SGR must appear after every internal reset so the background is
// continuous edge-to-edge. This is the critical "ANSI reset trap" test.
func TestRenderLineBackgroundContinuity(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 1,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "func main() Foo() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()

	bgSGR := "48;2;48;58;48"
	bgCount := strings.Count(out, bgSGR)
	if bgCount < 3 {
		t.Errorf("expected BG SGR to appear at least 3 times (start + after each token reset), got %d. output: %q", bgCount, out)
	}
}

// TestRenderLineNoBareResetInContent verifies constraint 3: no bare \x1b[m
// in the middle of content. The only bare resets should be from the outer
// Render at the very end of each line (for padding), not between tokens.
func TestRenderLineNoBareResetInContent(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 1,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "func main() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	lines := strings.Split(out, "\n")

	// The add line is the last line (after header + divider)
	addLine := lines[len(lines)-1]
	stripped := stripANSIDV(addLine)
	contentEnd := strings.LastIndex(stripped, "}")

	// Map visible content end to byte position in the ANSI string
	visibleIdx := 0
	byteIdx := 0
	for byteIdx < len(addLine) && visibleIdx <= contentEnd {
		if addLine[byteIdx] == '\x1b' {
			for byteIdx < len(addLine) && addLine[byteIdx] != 'm' && addLine[byteIdx] != 'H' && addLine[byteIdx] != 'G' {
				byteIdx++
			}
			if byteIdx < len(addLine) {
				byteIdx++
			}
			continue
		}
		visibleIdx++
		byteIdx++
	}

	contentPortion := addLine[:byteIdx]
	bareResets := testResetRe.FindAllString(contentPortion, -1)
	if len(bareResets) > 0 {
		t.Errorf("found %d bare reset(s) in content before '}' - background is broken: %q", len(bareResets), contentPortion)
	}
}

// TestRenderLineHighlightApplied verifies highlighting is applied.
func TestRenderLineHighlightApplied(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 1,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "func main() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI highlighting in output, got plain: %q", out)
	}
	stripped := stripANSIDV(out)
	if !strings.Contains(stripped, "func main() {}") {
		t.Errorf("expected 'func main() {}' in stripped output, got: %q", stripped)
	}
}

// TestRenderLineContextHighlight verifies context lines are highlighted.
func TestRenderLineContextHighlight(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Modified,
		Hunks: []mailpatch.Hunk{{
			OldStart: 1, OldLines: 1,
			NewStart: 1, NewLines: 1,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Context, Text: "func main() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI highlighting in context line, got plain: %q", out)
	}
}

// TestRenderLineDeleteHighlight verifies delete lines are highlighted.
func TestRenderLineDeleteHighlight(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		OldPath: "main.go",
		NewPath: "main.go",
		Type:    mailpatch.Modified,
		Hunks: []mailpatch.Hunk{{
			OldStart: 1, OldLines: 1,
			NewStart: 0, NewLines: 0,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Delete, Text: "func old() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI highlighting in delete line, got plain: %q", out)
	}
}

// TestRenderLineBoldCancelled verifies that after a bold highlighted keyword,
// the bold attribute is cancelled (22) for plain text between tokens.
func TestRenderLineBoldCancelled(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 1,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "func x() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	if !strings.Contains(out, "22;") {
		t.Errorf("expected bold cancellation (22;) after highlighted keyword, got: %q", out)
	}
}

// TestRenderLineItalicCancelled verifies that after an italic highlighted
// comment, the italic attribute is cancelled (23) for plain text.
func TestRenderLineItalicCancelled(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 2,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "// comment"},
				{Kind: mailpatch.Add, Text: "func x() {}"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()
	if !strings.Contains(out, "23;") {
		t.Errorf("expected italic cancellation (23;) after highlighted comment, got: %q", out)
	}
}

// TestLangFromPath verifies language detection from file paths.
func TestLangFromPath(t *testing.T) {
	cases := map[string]string{
		"main.go":     "go",
		"app.py":      "py",
		"index.js":    "js",
		"app.ts":      "ts",
		"main.rs":     "rs",
		"app.rb":      "rb",
		"script.sh":   "bash",
		"config.yml":  "yaml",
		"config.yaml": "yaml",
		"main.c":      "c",
		"header.h":    "c",
		"src.cpp":     "cpp",
		"src.cc":      "cpp",
		"src.hpp":     "cpp",
		"Main.java":   "java",
		"App.kt":      "kt",
		"build.scala": "scala",
		"query.sql":   "sql",
		"page.html":   "html",
		"data.xml":    "xml",
		"style.css":   "css",
		"theme.scss":  "scss",
		"data.json":   "json",
		"README.md":   "markdown",
		"file.toml":   "",
		"file.txt":    "",
		"unknown.xyz": "",
		"Makefile":    "",
	}
	for path, want := range cases {
		got := langFromPath(path)
		if got != want {
			t.Errorf("langFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestOuterSGR verifies the SGR construction matches expected format.
func TestOuterSGR(t *testing.T) {
	cases := []struct {
		name string
		s    Style
		want string
	}{
		{"Add", Style{Background: "#303a30", Foreground: "#c9d1d9"}, "\x1b[22;23;38;2;201;209;217;48;2;48;58;48m"},
		{"Context", Style{Background: "#161b22", Foreground: "#c9d1d9"}, "\x1b[22;23;38;2;201;209;217;48;2;22;27;34m"},
		{"Filename", Style{Background: "#30363d", Foreground: "#c9d1d9", IsBold: true}, "\x1b[1;23;38;2;201;209;217;48;2;48;54;61m"},
	}
	for _, tc := range cases {
		got := outerSGR(tc.s)
		if got != tc.want {
			t.Errorf("%s: outerSGR() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestRenderLineTrailingCR verifies that trailing \r (from \r\n line endings
// in email format-patches) is stripped, preventing carriage returns that
// garble the rendered output and break background fill.
func TestRenderLineTrailingCR(t *testing.T) {
	dv := New().Width(80)
	files := []mailpatch.FileChange{{
		NewPath: "main.go",
		Type:    mailpatch.Added,
		Hunks: []mailpatch.Hunk{{
			OldStart: 0, OldLines: 0,
			NewStart: 1, NewLines: 3,
			Lines: []mailpatch.Line{
				{Kind: mailpatch.Add, Text: "func main() {\r"},
				{Kind: mailpatch.Add, Text: "\tfmt.Println(\"hi\")\r"},
				{Kind: mailpatch.Add, Text: "}\r"},
			},
		}},
	}}
	dv.Files(files)
	out := dv.String()

	// No \r should survive into the rendered output
	if strings.Contains(out, "\r") {
		t.Errorf("rendered output contains \\r which will garble the terminal: %q", out)
	}

	// All lines should be exactly 80 visible chars
	for i, line := range strings.Split(out, "\n") {
		w := ansi.StringWidth(line)
		if w != 80 {
			t.Errorf("line %d width = %d, want 80 (trailing CR may have broken fill): %q", i, w, line)
		}
	}

	// Verify the text content is preserved (minus \r)
	stripped := stripANSIDV(out)
	if !strings.Contains(stripped, "func main() {") {
		t.Errorf("expected 'func main() {' in output, got: %q", stripped)
	}
}

// TestCleanLineText verifies that cleanLineText strips trailing \r.
func TestCleanLineText(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello\r", "hello"},
		{"hello\r\r", "hello"},
		{"hello", "hello"},
		{"\r", ""},
		{"", ""},
		{"hello world\r", "hello world"},
	}
	for _, tc := range cases {
		got := cleanLineText(tc.in)
		if got != tc.want {
			t.Errorf("cleanLineText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func stripANSIDV(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			i++
			for i < len(s) && s[i] != 'm' && s[i] != 'H' && s[i] != 'G' {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

var _ = fmt.Sprintf
