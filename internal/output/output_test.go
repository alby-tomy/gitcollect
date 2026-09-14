package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout to a pipe for the duration of fn and
// returns everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// withStdin temporarily replaces os.Stdin with content for the duration of
// fn, for testing Confirm/ConfirmWord.
func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	orig := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	go func() {
		w.WriteString(content)
		w.Close()
	}()

	fn()
}

// captureStderr redirects os.Stderr to a pipe for the duration of fn and
// returns everything written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestSimplePrinters(t *testing.T) {
	out := captureStdout(t, func() {
		Success("done %s", "thing")
	})
	if !strings.Contains(out, "✓") || !strings.Contains(out, "done thing") {
		t.Errorf("Success output = %q", out)
	}

	cases := []struct {
		name string
		fn   func(format string, args ...any)
		mark string
	}{
		{"Error", Error, "✗"},
		{"Warn", Warn, "⚠"},
	}
	for _, tc := range cases {
		out := captureStderr(t, func() {
			tc.fn("oops %s", "thing")
		})
		if !strings.Contains(out, tc.mark) || !strings.Contains(out, "oops thing") {
			t.Errorf("%s output = %q", tc.name, out)
		}
	}

	out = captureStderr(t, func() { Info("info %s", "thing") })
	if !strings.Contains(out, "info thing") {
		t.Errorf("Info output = %q", out)
	}

	out = captureStderr(t, func() { Dim("dim %s", "thing") })
	if !strings.Contains(out, "dim thing") {
		t.Errorf("Dim output = %q", out)
	}

	out = captureStderr(t, func() { Suggestion("gitcollect list") })
	if !strings.Contains(out, "Run: gitcollect list") {
		t.Errorf("Suggestion output = %q", out)
	}

	out = captureStderr(t, func() {
		Progress(1, 3, "working")
		Progress(3, 3, "working")
	})
	if !strings.Contains(out, "[1/3] working") || !strings.Contains(out, "[3/3] working") {
		t.Errorf("Progress output = %q", out)
	}
}

func TestTableAlignment(t *testing.T) {
	out := captureStdout(t, func() {
		Table([]string{"REPO", "ACCESS"}, [][]string{
			{"open-repo", "✓ yes"},
			{"restricted-repo-long-name", "✗ no"},
		})
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d lines: %q", len(lines), out)
	}

	// "restricted-repo-long-name" (25 runes) is the widest first column, so
	// every line's second column must start at rune offset 25+2, separated
	// by exactly two spaces — even though "✓"/"✗" are multi-byte UTF-8. This
	// guards against the byte-length-vs-rune-count alignment bug in
	// padRight/Table.
	const col0Width = len("restricted-repo-long-name")
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) < col0Width+2 {
			t.Fatalf("line shorter than expected column boundary: %q", line)
		}
		if sep := string(runes[col0Width : col0Width+2]); sep != "  " {
			t.Errorf("expected two-space separator at rune offset %d, got %q in line %q", col0Width, sep, line)
		}
	}
}

func TestJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	out := captureStdout(t, func() {
		if err := JSON(payload{Name: "acme"}); err != nil {
			t.Fatalf("JSON: %v", err)
		}
	})

	var got payload
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if got.Name != "acme" {
		t.Errorf("got %+v, want Name=acme", got)
	}
}

func TestConfirm(t *testing.T) {
	cases := map[string]bool{
		"y\n":    true,
		"yes\n":  true,
		"Y\n":    true,
		"n\n":    false,
		"\n":     false,
		"nope\n": false,
	}
	for input, want := range cases {
		var got bool
		withStdin(t, input, func() {
			got = Confirm("proceed?")
		})
		if got != want {
			t.Errorf("Confirm() with input %q = %v, want %v", input, got, want)
		}
	}
}

func TestConfirmWord(t *testing.T) {
	var got bool
	withStdin(t, "acme\n", func() {
		got = ConfirmWord("delete?", "acme")
	})
	if !got {
		t.Error("expected ConfirmWord to accept an exact match")
	}

	withStdin(t, "wrong\n", func() {
		got = ConfirmWord("delete?", "acme")
	})
	if got {
		t.Error("expected ConfirmWord to reject a non-matching word")
	}
}

func TestStaleWarning(t *testing.T) {
	out := captureStderr(t, func() {
		StaleWarning("cybersecurity", 45)
	})
	if !strings.Contains(out, "⚠") || !strings.Contains(out, "45 days ago") || !strings.Contains(out, "cybersecurity") {
		t.Errorf("StaleWarning output = %q, want it to mention both the collection name and the day count", out)
	}
}

func TestInviteWarning(t *testing.T) {
	out := captureStderr(t, func() {
		InviteWarning("bob", "alice", "https://github.com/notifications", "")
	})
	if !strings.Contains(out, "bob") || !strings.Contains(out, "alice") || !strings.Contains(out, "https://github.com/notifications") {
		t.Errorf("InviteWarning output = %q", out)
	}
	if strings.Contains(out, "Then retry") {
		t.Errorf("expected no retry line when retryCmd is empty, got %q", out)
	}

	out = captureStderr(t, func() {
		InviteWarning("bob", "alice", "https://github.com/notifications", "gitcollect clone cybersecurity")
	})
	if !strings.Contains(out, "Then retry: gitcollect clone cybersecurity") {
		t.Errorf("expected a retry line when retryCmd is set, got %q", out)
	}
}

func TestDim_WritesToStderr(t *testing.T) {
	// Dim must write to stderr, not stdout.
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			Dim("muted %s", "message")
		})
	})
	if stdout != "" {
		t.Errorf("Dim wrote to stdout, want stderr only: %q", stdout)
	}

	stderr := captureStderr(t, func() {
		Dim("muted %s", "message")
	})
	if !strings.Contains(stderr, "muted message") {
		t.Errorf("Dim output = %q, want it to contain %q", stderr, "muted message")
	}
}

func TestClearProgress_WritesEscapeSequence(t *testing.T) {
	out := captureStderr(t, func() {
		ClearProgress()
	})
	// \r moves to column 0; \033[K clears to end of line.
	if out != "\r\033[K" {
		t.Errorf("ClearProgress() = %q, want %q", out, "\r\033[K")
	}
}

// --- regression: consecutive prompts must both see piped input ---

// pipeStdin points os.Stdin at a file containing input for the test.
func pipeStdin(t *testing.T, input string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("temp stdin: %v", err)
	}
	if _, err := f.WriteString(input); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = old
		f.Close()
	})
}

// A fresh bufio.Reader per prompt threw away everything the previous one
// had read ahead, so only the first answer in a pipe was ever seen.
func TestConfirm_ReadsConsecutivePipedAnswers(t *testing.T) {
	pipeStdin(t, "y\ny\nn\n")

	if !Confirm("first") {
		t.Error("first prompt should read y")
	}
	if !Confirm("second") {
		t.Error("second prompt should read the second y, not EOF")
	}
	if Confirm("third") {
		t.Error("third prompt should read n")
	}
}

func TestConfirmWord_FollowsAConfirm(t *testing.T) {
	pipeStdin(t, "y\nmy-collection\n")

	if !Confirm("first") {
		t.Fatal("first prompt should read y")
	}
	if !ConfirmWord("type it", "my-collection") {
		t.Error("typed confirmation should read the second line")
	}
}

// Exhausted input must read as "no", never block or panic.
func TestConfirm_EOFIsNegative(t *testing.T) {
	pipeStdin(t, "")

	if Confirm("only prompt") {
		t.Error("EOF should not be treated as confirmation")
	}
}
