package send

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/floatpane/matcha/tui"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

func gitLogSubject(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%s")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func gitLogAuthor(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%an <%ae>")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log author: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func gitLogFullMessage(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%B")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log full message: %v", err)
	}
	return string(out)
}

func TestApplyPatchCmdStagesAndCommitCmdCreatesCommit(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv("GIT_COMMITTER_NAME", "Test User")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "greet.txt"), []byte("hello\nworld\nbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Phase 1: apply + stage.
	applyMsg := ApplyPatchCmd(dir, tui.ApplyPatchMsg{
		RawEmail:  patchEmailBody,
		Subject:   "[PATCH] fix greeting",
		From:      "Jane Dev <jane@example.com>",
		AccountID: "test",
	})()
	staged, ok := applyMsg.(tui.PatchStagedMsg)
	if !ok {
		t.Fatalf("expected PatchStagedMsg, got %T", applyMsg)
	}
	if len(staged.Files) == 0 {
		t.Fatal("expected staged files, got none")
	}

	// Phase 2: commit using the same message that CommitPatchCmd builds.
	message := buildCommitMessage(staged.Subject, staged.CommitMsg)
	authorName, authorEmail := parseAuthor(staged.From)
	gitCommit := exec.Command("git", "-C", dir, "commit", "-m", message)
	gitCommit.Env = appendEnv(os.Environ(), "GIT_AUTHOR_NAME", authorName)
	gitCommit.Env = appendEnv(gitCommit.Env, "GIT_AUTHOR_EMAIL", authorEmail)
	gitCommit.Env = appendEnv(gitCommit.Env, "GIT_COMMITTER_NAME", authorName)
	gitCommit.Env = appendEnv(gitCommit.Env, "GIT_COMMITTER_EMAIL", authorEmail)
	if out, err := gitCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// Subject line should have [PATCH] stripped.
	subject := gitLogSubject(t, dir)
	if subject != "fix greeting" {
		t.Errorf("commit subject = %q, want %q (no [PATCH] prefix)", subject, "fix greeting")
	}

	// Full message should include the commit body with trailers.
	full := gitLogFullMessage(t, dir)
	if !strings.Contains(full, "Fix the greeting to be more friendly.") {
		t.Errorf("commit message body missing description\ngot:\n%s", full)
	}
	if !strings.Contains(full, "Signed-off-by: Jane Dev <jane@example.com>") {
		t.Errorf("commit message body missing Signed-off-by trailer\ngot:\n%s", full)
	}

	author := gitLogAuthor(t, dir)
	if author != "Jane Dev <jane@example.com>" {
		t.Errorf("commit author = %q, want %q", author, "Jane Dev <jane@example.com>")
	}
}

func TestApplyPatchCmdNothingStaged(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv("GIT_COMMITTER_NAME", "Test User")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	// Apply the patch to a real file.
	if err := os.WriteFile(filepath.Join(dir, "greet.txt"), []byte("hello\nworld\nbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := ApplyPatchCmd(dir, tui.ApplyPatchMsg{
		RawEmail:  patchEmailBody,
		Subject:   "[PATCH] fix greeting",
		From:      "Jane Dev <jane@example.com>",
		AccountID: "test",
	})()

	// The patch modifies greet.txt, so there should be staged changes.
	res, ok := msg.(tui.PatchStagedMsg)
	if !ok {
		// If it returned PatchApplyResultMsg directly, that's the "nothing
		// staged" path — verify it has no error.
		result, ok2 := msg.(tui.PatchApplyResultMsg)
		if !ok2 {
			t.Fatalf("expected PatchStagedMsg or PatchApplyResultMsg, got %T", msg)
		}
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		return
	}
	if len(res.Files) == 0 {
		t.Error("expected files in PatchStagedMsg")
	}
}

func TestApplyPatchCmdCommitFailureWarning(t *testing.T) {
	dir := t.TempDir()

	// Seed the file so the patch applies, but don't git init — staging
	// should fail and produce a warning.
	if err := os.WriteFile(filepath.Join(dir, "greet.txt"), []byte("hello\nworld\nbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := ApplyPatchCmd(dir, tui.ApplyPatchMsg{
		RawEmail:  patchEmailBody,
		Subject:   "[PATCH] fix greeting",
		From:      "Jane Dev <jane@example.com>",
		AccountID: "test",
	})
	msg := cmd()
	res, ok := msg.(tui.PatchApplyResultMsg)
	if !ok {
		t.Fatalf("expected PatchApplyResultMsg for staging failure, got %T", msg)
	}
	if res.Err != nil {
		t.Fatalf("unexpected apply error: %v", res.Err)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning about git add failure, got none")
	}
}

func TestParseAuthor(t *testing.T) {
	tests := []struct {
		from      string
		wantName  string
		wantEmail string
	}{
		{"Jane Dev <jane@example.com>", "Jane Dev", "jane@example.com"},
		{"jane@example.com", "", "jane@example.com"},
		{"", "", ""},
		{"Plain Name", "Plain Name", ""},
	}
	for _, tt := range tests {
		name, email := parseAuthor(tt.from)
		if name != tt.wantName || email != tt.wantEmail {
			t.Errorf("parseAuthor(%q) = (%q, %q), want (%q, %q)",
				tt.from, name, email, tt.wantName, tt.wantEmail)
		}
	}
}

func TestStripPatchPrefix(t *testing.T) {
	tests := []struct {
		subject string
		want    string
	}{
		{"[PATCH] fix greeting", "fix greeting"},
		{"[PATCH v2] fix greeting", "fix greeting"},
		{"[PATCH 1/3] fix greeting", "fix greeting"},
		{"[RFC PATCH v3 2/5] fix greeting", "fix greeting"},
		{"fix greeting", "fix greeting"},
		{"[bug] fix greeting", "[bug] fix greeting"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripPatchPrefix(tt.subject)
		if got != tt.want {
			t.Errorf("stripPatchPrefix(%q) = %q, want %q", tt.subject, got, tt.want)
		}
	}
}

func TestBuildCommitMessage(t *testing.T) {
	got := buildCommitMessage("[PATCH] fix greeting", "The description.\n\nSigned-off-by: Jane <jane@example.com>")
	want := "fix greeting\n\nThe description.\n\nSigned-off-by: Jane <jane@example.com>"
	if got != want {
		t.Errorf("buildCommitMessage = %q, want %q", got, want)
	}

	// No commit body — just the clean subject.
	got = buildCommitMessage("[PATCH v2] fix greeting", "")
	if got != "fix greeting" {
		t.Errorf("buildCommitMessage with empty body = %q, want %q", got, "fix greeting")
	}
}

const patchEmailBody = `Fix the greeting to be more friendly.

This updates the greeting to use "there" instead of "world" for
a warmer tone.

Co-developed-by: Bob Smith <bob@example.com>
Signed-off-by: Jane Dev <jane@example.com>

---
 greet.txt | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)

diff --git a/greet.txt b/greet.txt
index cdd2c8b..2b56e0a 100644
--- a/greet.txt
+++ b/greet.txt
@@ -1,3 +1,3 @@
 hello
-world
+there
 bye
`

// TestApplyPatchCmdCRLFNoDiffStatLeak verifies that when a patch email has
// \r\n line endings (RFC 5322 standard), the diffstat block does not leak
// into the commit message. Before the fix, the mailpatch parser's "---"
// separator detection failed on "\r\n" endings, causing the entire diffstat
// (file table + summary) to end up in the git commit description.
func TestApplyPatchCmdCRLFNoDiffStatLeak(t *testing.T) {
	crlfBody := strings.ReplaceAll(patchEmailBody, "\n", "\r\n")

	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv("GIT_COMMITTER_NAME", "Test User")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "greet.txt"), []byte("hello\nworld\nbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	applyMsg := ApplyPatchCmd(dir, tui.ApplyPatchMsg{
		RawEmail:  crlfBody,
		Subject:   "[PATCH] fix greeting",
		From:      "Jane Dev <jane@example.com>",
		AccountID: "test",
	})()
	staged, ok := applyMsg.(tui.PatchStagedMsg)
	if !ok {
		t.Fatalf("expected PatchStagedMsg, got %T", applyMsg)
	}

	message := buildCommitMessage(staged.Subject, staged.CommitMsg)

	// The commit message must NOT contain diffstat artifacts.
	if strings.Contains(message, "files changed") {
		t.Errorf("commit message contains diffstat summary:\n%s", message)
	}
	if strings.Contains(message, "greet.txt |") {
		t.Errorf("commit message contains diffstat file entry:\n%s", message)
	}
	if strings.Contains(message, "insertion(+)") {
		t.Errorf("commit message contains diffstat insertion count:\n%s", message)
	}
	// The commit message must NOT contain the "---" separator.
	if strings.Contains(message, "\n---\n") {
		t.Errorf("commit message contains diffstat separator:\n%s", message)
	}
	// The commit message SHOULD contain the actual description.
	if !strings.Contains(message, "Fix the greeting to be more friendly.") {
		t.Errorf("commit message missing description:\n%s", message)
	}
	if !strings.Contains(message, "Signed-off-by: Jane Dev <jane@example.com>") {
		t.Errorf("commit message missing Signed-off-by trailer:\n%s", message)
	}
}

func TestRewriteToHeader(t *testing.T) {
	const raw = "From: sender@example.com\r\nSubject: [PATCH] fix\r\n\r\nbody"
	got := string(rewriteToHeader([]byte(raw), "to@example.com"))
	if !strings.Contains(got, "To: to@example.com") {
		t.Errorf("rewriteToHeader did not insert To header:\n%s", got)
	}
	// Should preserve existing headers and body.
	if !strings.Contains(got, "From: sender@example.com") {
		t.Errorf("rewriteToHeader removed From header:\n%s", got)
	}
	if !strings.Contains(got, "body") {
		t.Errorf("rewriteToHeader removed body:\n%s", got)
	}
}

func TestRewriteToHeaderReplacesExisting(t *testing.T) {
	const raw = "From: sender@example.com\r\nTo: old@example.com\r\nSubject: [PATCH] fix\r\n\r\nbody"
	got := string(rewriteToHeader([]byte(raw), "new@example.com"))
	if strings.Contains(got, "To: old@example.com") {
		t.Errorf("rewriteToHeader did not replace old To header:\n%s", got)
	}
	if !strings.Contains(got, "To: new@example.com") {
		t.Errorf("rewriteToHeader did not insert new To header:\n%s", got)
	}
}

func TestRewriteCcHeader(t *testing.T) {
	const raw = "From: sender@example.com\r\nSubject: [PATCH] fix\r\n\r\nbody"
	got := string(rewriteCcHeader([]byte(raw), "cc1@example.com, cc2@example.com"))
	if !strings.Contains(got, "Cc: cc1@example.com, cc2@example.com") {
		t.Errorf("rewriteCcHeader did not insert Cc header:\n%s", got)
	}
}

func TestRewriteHeaderLFLineEndings(t *testing.T) {
	const raw = "From: sender@example.com\nSubject: [PATCH] fix\n\nbody"
	got := string(rewriteToHeader([]byte(raw), "to@example.com"))
	if !strings.Contains(got, "To: to@example.com") {
		t.Errorf("rewriteToHeader did not handle LF line endings:\n%s", got)
	}
	// Verify it preserved LF line endings, not CRLF.
	if strings.Contains(got, "\r\n") {
		t.Errorf("rewriteToHeader changed LF to CRLF:\n%s", got)
	}
}
