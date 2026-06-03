package gitmail_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/floatpane/matcha/gitmail"
)

const patchEmail = `From 9f8e7d6c Mon Sep 17 00:00:00 2001
From: Ada Lovelace <ada@example.com>
Date: Mon, 2 Jun 2025 11:30:00 +0000
Subject: [PATCH 1/1] greet: change the world

---
 greet.txt | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)

diff --git a/greet.txt b/greet.txt
index 111..222 100644
--- a/greet.txt
+++ b/greet.txt
@@ -1,3 +1,3 @@
 hello
-world
+there
 bye
--
2.45.1
`

func TestApply(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "greet.txt"), []byte("hello\nworld\nbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := gitmail.Apply([]byte(patchEmail), dir, gitmail.Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if s.Subject != "greet: change the world" {
		t.Errorf("Subject = %q", s.Subject)
	}
	if s.Series.Index != 1 || s.Series.Total != 1 {
		t.Errorf("Series = %+v", s.Series)
	}
	if len(s.Files) != 1 || s.Files[0].Path != "greet.txt" {
		t.Fatalf("Files = %+v", s.Files)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "greet.txt"))
	if string(got) != "hello\nthere\nbye\n" {
		t.Errorf("content = %q", got)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	orig := []byte("hello\nworld\nbye\n")
	os.WriteFile(filepath.Join(dir, "greet.txt"), orig, 0o644)

	if _, err := gitmail.Apply([]byte(patchEmail), dir, gitmail.Options{DryRun: true}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "greet.txt"))
	if string(got) != string(orig) {
		t.Errorf("dry run wrote to disk: %q", got)
	}
}

func TestReverse(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "greet.txt"), []byte("hello\nworld\nbye\n"), 0o644)

	if _, err := gitmail.Apply([]byte(patchEmail), dir, gitmail.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := gitmail.Apply([]byte(patchEmail), dir, gitmail.Options{Reverse: true}); err != nil {
		t.Fatalf("reverse: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "greet.txt"))
	if string(got) != "hello\nworld\nbye\n" {
		t.Errorf("reverse did not restore: %q", got)
	}
}

func TestIsPatch(t *testing.T) {
	if !gitmail.IsPatch([]byte(patchEmail)) {
		t.Error("IsPatch = false for a real patch")
	}
	plain := "Subject: hi\n\njust a normal email, no diff\n"
	if gitmail.IsPatch([]byte(plain)) {
		t.Error("IsPatch = true for a non-patch")
	}
}
