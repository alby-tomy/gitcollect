// Package output implements gitcollect's terminal output conventions:
// stdout carries data and success, stderr carries errors, warnings,
// progress, and prompts. Colour is disabled when NO_COLOR is set or the
// relevant stream is not a terminal.
package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"
	"golang.org/x/term"
)

var (
	successColor = color.New(color.FgGreen)
	errorColor   = color.New(color.FgRed)
	warnColor    = color.New(color.FgYellow)
	infoColor    = color.New(color.FgCyan)
	dimColor     = color.New(color.FgHiBlack)
)

func init() {
	if os.Getenv("NO_COLOR") != "" || !isTerminal(os.Stdout) || !isTerminal(os.Stderr) {
		color.NoColor = true
	}
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// Success prints a green "✓ " line to stdout.
func Success(format string, args ...any) {
	successColor.Fprintln(os.Stdout, "✓ "+fmt.Sprintf(format, args...))
}

// Error prints a red "✗ " line to stderr.
func Error(format string, args ...any) {
	errorColor.Fprintln(os.Stderr, "✗ "+fmt.Sprintf(format, args...))
}

// Warn prints a yellow "⚠ " line to stderr.
func Warn(format string, args ...any) {
	warnColor.Fprintln(os.Stderr, "⚠ "+fmt.Sprintf(format, args...))
}

// Info prints a cyan informational line to stderr.
func Info(format string, args ...any) {
	infoColor.Fprintln(os.Stderr, fmt.Sprintf(format, args...))
}

// Dim prints a muted line to stderr, used for skipped or secondary items.
func Dim(format string, args ...any) {
	dimColor.Fprintln(os.Stderr, fmt.Sprintf(format, args...))
}

// Progress overwrites the current stderr line with a "[current/total] label"
// indicator. Once current reaches total it prints a trailing newline so the
// final state is left on screen for whatever is printed next.
func Progress(current, total int, label string) {
	fmt.Fprintf(os.Stderr, "\r[%d/%d] %s", current, total, label)
	if current >= total {
		fmt.Fprintln(os.Stderr)
	}
}

// Table prints headers and rows as aligned, space-padded columns to stdout.
func Table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if w := utf8.RuneCountInString(cell); w > widths[i] {
					widths[i] = w
				}
			}
		}
	}

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	writeRow := func(cells []string) {
		var b strings.Builder
		for i, cell := range cells {
			if i < len(widths) {
				b.WriteString(padRight(cell, widths[i]))
			} else {
				b.WriteString(cell)
			}
			if i < len(cells)-1 {
				b.WriteString("  ")
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}

	writeRow(headers)
	for _, row := range rows {
		writeRow(row)
	}
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// JSON marshals v as indented JSON to stdout.
func JSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Confirm prints "<prompt> [y/N]: " to stderr and reads a line from stdin.
// Only "y" or "yes" (case-insensitive) are treated as confirmation.
func Confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	answer := strings.ToLower(strings.TrimSpace(readLine()))
	return answer == "y" || answer == "yes"
}

// ConfirmWord prints prompt to stderr and requires the user to type word
// exactly, for destructive confirmations such as delete.
func ConfirmWord(prompt, word string) bool {
	fmt.Fprintf(os.Stderr, "%s (type %q to confirm): ", prompt, word)
	return strings.TrimSpace(readLine()) == word
}

func readLine() string {
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return line
}

// Suggestion prints a "Run: <cmd>" hint line to stderr.
func Suggestion(cmd string) {
	dimColor.Fprintln(os.Stderr, "Run: "+cmd)
}

// StaleWarning warns that collectionName's local manifest hasn't been
// updated in daysSince days, so it may no longer reflect the owner's
// latest changes — informational only, never blocks the command.
func StaleWarning(collectionName string, daysSince int) {
	Warn("%q was last updated %d days ago.", collectionName, daysSince)
	dimColor.Fprintln(os.Stderr, "  If you are not the owner, ask them for the latest collection file.")
}

// InviteWarning explains a pending GitHub collaborator invite: username
// has been granted access on the manifest's side, but GitHub itself won't
// let them collaborate until they accept the emailed invite at notifURL.
// If retryCmd is non-empty, a "Then retry: <retryCmd>" line is appended —
// used when this is printed in response to a failed clone/pull/status,
// where there's a concrete next command to re-run; omitted for the
// informational notice printed right after a successful member/repo
// grant, where there's nothing to retry yet.
func InviteWarning(username, owner, notifURL, retryCmd string) {
	Warn("%s has a pending collaborator invite from %s.", username, owner)
	dimColor.Fprintln(os.Stderr, "  Accept it at: "+notifURL)
	if retryCmd != "" {
		dimColor.Fprintln(os.Stderr, "  Then retry: "+retryCmd)
	}
}
