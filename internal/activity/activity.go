// Package activity implements gitcollect's code-activity log: commits
// observed in a collection's repos via the GitHub/GitLab API, recorded as
// one line of newline-delimited JSON per commit under
// ~/.gitcollect/activity/<collection>.log.
//
// This is deliberately separate from internal/audit: audit.log records
// access mutations gitcollect itself performed (member add, repo access
// changes, etc.); activity.log records git commits gitcollect observed by
// asking the platform — a different kind of event from a different source,
// fetched live each run rather than driven by gitcollect's own mutations.
package activity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alby-tomy/gitcollect/internal/config"
)

// Entry is one recorded commit.
type Entry struct {
	Timestamp   time.Time `json:"timestamp"` // when gitcollect recorded this entry
	Collection  string    `json:"collection"`
	Repo        string    `json:"repo"`
	Branch      string    `json:"branch"`
	CommitSHA   string    `json:"commit_sha"`
	Author      string    `json:"author"`
	Message     string    `json:"message"`
	CommittedAt time.Time `json:"committed_at"` // when the commit itself was made
}

func logPath(collection string) (string, error) {
	dir, err := config.ActivityDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, collection+".log"), nil
}

// Append writes entry as one line of newline-delimited JSON to the
// collection's activity log, creating ~/.gitcollect/activity and the log
// file if needed. As with audit.Append, the line is written in a single
// os.File.Write call so concurrent appenders don't interleave partial
// lines.
func Append(entry Entry) error {
	dir, err := config.ActivityDir()
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
		return fmt.Errorf("could not encode activity entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("could not open activity log for %s: %w", entry.Collection, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("could not write activity log for %s: %w", entry.Collection, err)
	}
	return nil
}

// Read returns every entry for collection, newest first. A missing log is
// not an error: it returns an empty slice.
func Read(collection string) ([]Entry, error) {
	path, err := logPath(collection)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("could not read activity log for %s: %w", collection, err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("could not parse activity log for %s: %w", collection, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not read activity log for %s: %w", collection, err)
	}

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

// Filter keeps entries matching repo (if non-empty), user (if non-empty,
// matched against Author), and within the last `since` duration of their
// CommittedAt time (if since is non-zero).
func Filter(entries []Entry, repo, user string, since time.Duration) []Entry {
	var cutoff time.Time
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}

	filtered := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if repo != "" && e.Repo != repo {
			continue
		}
		if user != "" && e.Author != user {
			continue
		}
		if since > 0 && e.CommittedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// KnownSHAs returns the set of "repo\x00sha" pairs already present in
// entries, so a caller fetching fresh commits from the platform can tell
// which ones are genuinely new before appending — Append itself does not
// deduplicate.
func KnownSHAs(entries []Entry) map[string]bool {
	known := make(map[string]bool, len(entries))
	for _, e := range entries {
		known[e.Repo+"\x00"+e.CommitSHA] = true
	}
	return known
}
