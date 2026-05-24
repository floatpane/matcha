package loglevel

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func captureLog(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
		Set(LevelInfo)
	}()

	fn()

	return buf.String()
}

func TestDebugfRequiresDebugLevel(t *testing.T) {
	for _, level := range []Level{LevelSilent, LevelInfo, LevelVerbose} {
		Set(level)
		output := captureLog(t, func() {
			Debugf("details %d", 1)
		})
		if output != "" {
			t.Fatalf("Debugf wrote at level %v: %q", level, output)
		}
	}

	Set(LevelDebug)
	output := captureLog(t, func() {
		Debugf("details %d", 1)
	})
	if !strings.Contains(output, "debug: details 1") {
		t.Fatalf("Debugf did not write expected message: %q", output)
	}
}

func TestVerbosefRequiresVerboseLevel(t *testing.T) {
	Set(LevelInfo)
	output := captureLog(t, func() {
		Verbosef("more details")
	})
	if output != "" {
		t.Fatalf("Verbosef wrote at info level: %q", output)
	}

	for _, level := range []Level{LevelVerbose, LevelDebug} {
		Set(level)
		output := captureLog(t, func() {
			Verbosef("more details")
		})
		if !strings.Contains(output, "verbose: more details") {
			t.Fatalf("Verbosef did not write at level %v: %q", level, output)
		}
	}
}

func TestInfofRequiresInfoLevel(t *testing.T) {
	Set(LevelSilent)
	output := captureLog(t, func() {
		Infof("hello")
	})
	if output != "" {
		t.Fatalf("Infof wrote at silent level: %q", output)
	}

	Set(LevelInfo)
	output = captureLog(t, func() {
		Infof("hello")
	})
	if !strings.Contains(output, "info: hello") {
		t.Fatalf("Infof did not write at info level: %q", output)
	}
}

func TestSetAndGet(t *testing.T) {
	Set(LevelDebug)
	if Get() != LevelDebug {
		t.Fatalf("Get() = %v, want %v", Get(), LevelDebug)
	}

	Set(LevelInfo)
	if Get() != LevelInfo {
		t.Fatalf("Get() = %v, want %v", Get(), LevelInfo)
	}
}
