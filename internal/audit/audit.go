// Package audit implements gitcollect's access change log: every mutation
// is appended as one line of newline-delimited JSON under
// ~/.gitcollect/audit/<collection>.log.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alby-tomy/gitcollect/internal/config"
)

// AuditEntry is one line of a collection's audit log.
type AuditEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Collection string    `json:"collection"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Detail     string    `json:"detail"`
	Result     string    `json:"result"`
}

func logPath(collection string) (string, error) {
	dir, err := config.AuditDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, collection+".log"), nil
}

// Append writes entry as one line of newline-delimited JSON to the
// collection's audit log, creating ~/.gitcollect/audit and the log file if
// needed. The line is written in a single os.File.Write call so concurrent
// appenders don't interleave partial lines.
func Append(entry AuditEntry) error {
	dir, err := config.AuditDir()
	if err != nil {
		return err
	}
	if err := config.EnsureDir(dir); err != nil {
		return err
	}

	path, err := logPath(entry.Collection)
	if err != nil {
		return err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("could not encode audit entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("could not open audit log for %s: %w", entry.Collection, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("could not write audit log for %s: %w", entry.Collection, err)
	}
	return nil
}

// Read returns every entry for collection, newest first. A missing log is
// not an error: it returns an empty slice.
func Read(collection string) ([]AuditEntry, error) {
	path, err := logPath(collection)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []AuditEntry{}, nil
		}
		return nil, fmt.Errorf("could not read audit log for %s: %w", collection, err)
	}
	defer f.Close()

	var entries []AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("could not parse audit log for %s: %w", collection, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not read audit log for %s: %w", collection, err)
	}

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

// Filter keeps entries where user matches Actor or Target (if user is
// non-empty) and Timestamp falls within the last `since` duration (if
// since is non-zero).
func Filter(entries []AuditEntry, user string, since time.Duration) []AuditEntry {
	var cutoff time.Time
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}

	filtered := make([]AuditEntry, 0, len(entries))
	for _, e := range entries {
		if user != "" && e.Actor != user && e.Target != user {
			continue
		}
		if since > 0 && e.Timestamp.Before(cutoff) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}
