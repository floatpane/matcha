package send

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/tui"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// Applying a patch email must stage and commit only the files it touched,
// not the user's unrelated work in progress (`git add -A` swept it into the
// patch commit, authored as the patch sender).
func TestApplyPatchStagesAndCommitsOnlyPatchedFiles(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("greet.txt", "hello\nworld\nbye\n")
	write("notes.txt", "v1\n")
	gitRun(t, dir, "add", "greet.txt", "notes.txt")
	gitRun(t, dir, "commit", "-m", "init")

	write("notes.txt", "v2 (work in progress)\n") // unrelated, unstaged
	write("scratch.txt", "untracked\n")           // unrelated, untracked
	write("staged.txt", "already staged\n")       // unrelated, staged by the user
	gitRun(t, dir, "add", "staged.txt")

	msg := ApplyPatchCmd(dir, tui.ApplyPatchMsg{
		RawEmail: patchEmailBody,
		Subject:  "[PATCH] fix greeting",
		From:     "Jane Dev <jane@example.com>",
	})()
	staged, ok := msg.(tui.PatchStagedMsg)
	if !ok {
		t.Fatalf("expected PatchStagedMsg, got %#v", msg)
	}
	if !slices.Equal(staged.StagedPaths, []string{"greet.txt"}) {
		t.Fatalf("StagedPaths = %v", staged.StagedPaths)
	}

	commit := commitPatchCommand(dir, staged)
	commit.Env = appendEnv(commit.Env, "GIT_CONFIG_GLOBAL", os.DevNull)
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}

	files := gitRun(t, dir, "show", "--name-only", "--format=", "HEAD")
	if files != "greet.txt" {
		t.Fatalf("patch commit contains %q, want only greet.txt", files)
	}
	if author := gitRun(t, dir, "log", "-1", "--format=%an <%ae>"); author != "Jane Dev <jane@example.com>" {
		t.Fatalf("author = %q", author)
	}
	// The user's own changes are untouched: still modified / staged / untracked.
	status := gitRun(t, dir, "status", "--porcelain")
	for _, want := range []string{"M notes.txt", "A  staged.txt", "?? scratch.txt"} {
		if !strings.Contains(status, want) {
			t.Errorf("status lost %q:\n%s", want, status)
		}
	}
}

func TestPatchAccountUsesSelectedAccount(t *testing.T) {
	cfg := &config.Config{Accounts: []config.Account{{ID: "first"}, {ID: "second"}}}
	if a, err := patchAccount(cfg, "second"); err != nil || a.ID != "second" {
		t.Fatalf("selected account: %v %v", a, err)
	}
	if a, err := patchAccount(cfg, ""); err != nil || a.ID != "first" {
		t.Fatalf("default account: %v %v", a, err)
	}
	if _, err := patchAccount(cfg, "gone"); err == nil {
		t.Fatal("unknown account must be an error, not a silent fallback")
	}
	if _, err := patchAccount(&config.Config{}, ""); err == nil {
		t.Fatal("no accounts must be an error")
	}
}
