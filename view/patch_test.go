package view

import (
	"strings"
	"testing"
)

const testPatchBody = `What?

Adds support for h3-h6, lists, inline formatting, and horizontal rules.

Why?

Adds more tags, so that the renderer parses more of the emails fully.

---
 clib/htmlconv.c | 297 ++++++++++++++----
 view/html.go    | 244 +++++++++++----
 2 files changed, 541 insertions(+), 146 deletions(-)

diff --git a/clib/htmlconv.c b/clib/htmlconv.c
index f4c5692..e592bec 100644
--- a/clib/htmlconv.c
+++ b/clib/htmlconv.c
@@ -83,6 +83,7 @@ static HTMLElement* result_add(HTMLConvertResult* r) {
 HTMLElement* e = &r->elements[r->count++];
 e->type = HELEM_TEXT;
-e->style = 0;
+e->style = 1;
 e->text = NULL;
--
2.45.1
`

const testPatchSubject = "[PATCH 1/1] feat: more HTML tags"

func TestDetectPatch(t *testing.T) {
	info := DetectPatch(testPatchBody, BodyMIMETypePlain, testPatchSubject, "Drew <me@andrinoff.com>")
	if info == nil {
		t.Fatal("DetectPatch returned nil for a patch body")
	}
	if info.Subject != "feat: more HTML tags" {
		t.Errorf("Subject = %q, want %q", info.Subject, "feat: more HTML tags")
	}
	if info.Author != "Drew <me@andrinoff.com>" {
		t.Errorf("Author = %q", info.Author)
	}
	if !info.HasDiff {
		t.Error("HasDiff = false")
	}
	if info.SeriesIndex != 1 || info.SeriesTotal != 1 {
		t.Errorf("Series = %d/%d", info.SeriesIndex, info.SeriesTotal)
	}
	if info.Stat.FilesChanged != 1 {
		t.Errorf("Stat.FilesChanged = %d", info.Stat.FilesChanged)
	}
	if !strings.Contains(info.Diff, "diff --git") {
		t.Error("Diff does not contain diff --git")
	}
	if !strings.Contains(info.CommitMessage, "Adds support for") {
		t.Error("CommitMessage missing body text")
	}
}

func TestDetectPatchNonPatch(t *testing.T) {
	plain := "just a normal email, no diff\n"
	info := DetectPatch(plain, BodyMIMETypePlain, "Hello", "someone@example.com")
	if info != nil {
		t.Error("DetectPatch returned non-nil for a non-patch email")
	}
}

func TestDetectPatchHTML(t *testing.T) {
	info := DetectPatch(testPatchBody, BodyMIMETypeHTML, testPatchSubject, "someone@example.com")
	if info != nil {
		t.Error("DetectPatch returned non-nil for HTML body")
	}
}

func TestDetectPatchVersionedSeries(t *testing.T) {
	info := DetectPatch(testPatchBody, BodyMIMETypePlain, "[PATCH v2 3/5] fix: important bug", "dev@example.com")
	if info == nil {
		t.Fatal("DetectPatch returned nil")
	}
	if info.SeriesVersion != 2 {
		t.Errorf("SeriesVersion = %d, want 2", info.SeriesVersion)
	}
	if info.SeriesIndex != 3 || info.SeriesTotal != 5 {
		t.Errorf("Series = %d/%d, want 3/5", info.SeriesIndex, info.SeriesTotal)
	}
}

func TestRenderPatchBody(t *testing.T) {
	rendered, ok := RenderPatchBody(testPatchBody, BodyMIMETypePlain, testPatchSubject, "Drew <me@andrinoff.com>", 120)
	if !ok {
		t.Fatal("RenderPatchBody returned false for a patch body")
	}
	if !strings.Contains(rendered, "Git Patch") {
		t.Error("rendered output missing 'Git Patch' banner")
	}
	if !strings.Contains(rendered, "feat: more HTML tags") {
		t.Error("rendered output missing subject")
	}
	if !strings.Contains(rendered, "htmlconv.c") {
		t.Error("rendered output missing diff file content")
	}
}

func TestRenderPatchBodyNonPatch(t *testing.T) {
	plain := "just a normal email\n"
	_, ok := RenderPatchBody(plain, BodyMIMETypePlain, "Hello", "someone@example.com", 80)
	if ok {
		t.Error("RenderPatchBody returned true for a non-patch email")
	}
}

func TestHighlightDiff(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
index 111..222 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
 old
-removed
+added
 context
`
	result := HighlightDiff(diff)
	if result == diff {
		t.Error("HighlightDiff returned input unchanged — no ANSI codes applied")
	}
	if !strings.Contains(result, "diff --git") {
		t.Error("HighlightDiff lost diff content")
	}
}

func TestExtractDiffStatText(t *testing.T) {
	stat := extractDiffStatText(testPatchBody)
	if stat == "" {
		t.Fatal("extractDiffStatText returned empty for a patch with diffstat")
	}
	if !strings.Contains(stat, "htmlconv.c") {
		t.Errorf("diffstat missing file path, got: %q", stat)
	}
	if !strings.Contains(stat, "files changed") {
		t.Errorf("diffstat missing summary line, got: %q", stat)
	}
}

func TestExtractDiffStatTextCRLF(t *testing.T) {
	body := strings.ReplaceAll(testPatchBody, "\n", "\r\n")
	stat := extractDiffStatText(body)
	if stat == "" {
		t.Fatal("extractDiffStatText returned empty for CRLF patch body")
	}
	if !strings.Contains(stat, "htmlconv.c") {
		t.Errorf("CRLF diffstat missing file path, got: %q", stat)
	}
}

func TestDetectPatchCRLF(t *testing.T) {
	body := strings.ReplaceAll(testPatchBody, "\n", "\r\n")
	info := DetectPatch(body, BodyMIMETypePlain, testPatchSubject, "Drew <me@andrinoff.com>")
	if info == nil {
		t.Fatal("DetectPatch returned nil for CRLF patch body")
	}
	if strings.Contains(info.CommitMessage, "---") {
		t.Errorf("CRLF commit message should not contain diffstat separator, got: %q", info.CommitMessage)
	}
	if strings.Contains(info.CommitMessage, "files changed") {
		t.Errorf("CRLF commit message should not contain diffstat summary, got: %q", info.CommitMessage)
	}
	if strings.Contains(info.CommitMessage, "htmlconv.c") {
		t.Errorf("CRLF commit message should not contain diffstat file entries, got: %q", info.CommitMessage)
	}
	if !strings.Contains(info.CommitMessage, "Adds support for") {
		t.Errorf("CRLF commit message missing body text, got: %q", info.CommitMessage)
	}
}

func TestRenderDiffStatBlock(t *testing.T) {
	statText := ` main.go    | 5 +++++
 utils.go   | 3 ---
 2 files changed, 5 insertions(+), 3 deletions(-)`
	result := renderDiffStatBlock(statText)
	if result == statText {
		t.Error("renderDiffStatBlock returned input unchanged — no ANSI codes applied")
	}
	stripped := stripANSITest(result)
	if !strings.Contains(stripped, "main.go") {
		t.Error("renderDiffStatBlock lost file path")
	}
	if !strings.Contains(stripped, "5 insertions") {
		t.Error("renderDiffStatBlock lost insertions count")
	}
}

func TestDetectPatchDiffStatText(t *testing.T) {
	info := DetectPatch(testPatchBody, BodyMIMETypePlain, testPatchSubject, "Drew <me@andrinoff.com>")
	if info == nil {
		t.Fatal("DetectPatch returned nil")
	}
	if info.DiffStatText == "" {
		t.Error("DiffStatText should not be empty for a patch with diffstat")
	}
	if !strings.Contains(info.DiffStatText, "htmlconv.c") {
		t.Errorf("DiffStatText missing file path, got: %q", info.DiffStatText)
	}
}

func stripANSITest(s string) string {
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
