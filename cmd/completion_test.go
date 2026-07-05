package cmd

import (
	"os"
	"strings"
	"testing"
)

// discardStdout redirects os.Stdout to os.DevNull for the duration of fn.
// Use this instead of captureStdout when the output can be large (e.g. shell
// completion scripts), since captureStdout's pipe buffer can deadlock.
func discardStdout(fn func()) {
	old := os.Stdout
	dev, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		fn()
		return
	}
	os.Stdout = dev
	defer func() {
		os.Stdout = old
		dev.Close()
	}()
	fn()
}

func resetCompletionFns() func() {
	old := completionIsTerminalFn
	return func() { completionIsTerminalFn = old }
}

func TestCompletion_HintPrintedWhenTTY(t *testing.T) {
	defer resetCompletionFns()()
	completionIsTerminalFn = func() bool { return true }

	stderr := captureStderr(func() {
		discardStdout(func() {
			if err := runCompletion(nil, []string{"bash"}); err != nil {
				t.Fatalf("runCompletion: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "Pipe this to your shell profile") {
		t.Errorf("expected TTY hint in stderr, got: %q", stderr)
	}
}

func TestCompletion_HintPrintedWhenPiped(t *testing.T) {
	defer resetCompletionFns()()
	completionIsTerminalFn = func() bool { return false }

	stderr := captureStderr(func() {
		discardStdout(func() {
			if err := runCompletion(nil, []string{"bash"}); err != nil {
				t.Fatalf("runCompletion: %v", err)
			}
		})
	})

	if stderr == "" {
		t.Error("expected activation hint in stderr when piped, got empty string")
	}
	if !strings.Contains(stderr, "activate") {
		t.Errorf("expected activation hint to mention 'activate' in stderr, got: %q", stderr)
	}
}

func TestCompletion_BashHintMentionsSource(t *testing.T) {
	defer resetCompletionFns()()
	completionIsTerminalFn = func() bool { return false }

	stderr := captureStderr(func() {
		discardStdout(func() {
			if err := runCompletion(nil, []string{"bash"}); err != nil {
				t.Fatalf("runCompletion: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "source") {
		t.Errorf("expected bash activation hint to mention 'source', got: %q", stderr)
	}
}
