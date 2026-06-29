package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	auditUser  string
	auditSince string
	auditJSON  bool
)

var auditCmd = &cobra.Command{
	Use:   "audit <collection>",
	Short: "Show the access change log for a collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runAudit,
}

func init() {
	auditCmd.Flags().StringVar(&auditUser, "user", "", "filter the log to entries involving this user")
	auditCmd.Flags().StringVar(&auditSince, "since", "", "filter the log to entries within this duration (e.g. 7d, 30d, 90d, 24h)")
	auditCmd.Flags().BoolVar(&auditJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(auditCmd)
}

// parseSince accepts gitcollect's documented day-based shorthand (7d, 30d,
// 90d) in addition to anything time.ParseDuration understands (24h, 30m).
func parseSince(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid --since %q: %w", s, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --since %q: %w", s, err)
	}
	return d, nil
}

func runAudit(cmd *cobra.Command, args []string) error {
	name := args[0]

	since, err := parseSince(auditSince)
	if err != nil {
		return NewUsageError(fmt.Errorf("audit: %w", err))
	}

	if _, _, err := loadForRead(name); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	entries, err := audit.Read(name)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	entries = audit.Filter(entries, auditUser, since)

	if auditJSON {
		return output.JSON(entries)
	}

	if len(entries) == 0 {
		output.Info("no audit entries found for %q", name)
		return nil
	}

	for _, e := range entries {
		ts := e.Timestamp.Local().Format("2006-01-02 15:04")
		result := ""
		if e.Result != "ok" {
			result = "  [" + e.Result + "]"
		}
		fmt.Printf("%s  %-10s  %-20s  %-20s  %s%s\n", ts, e.Actor, e.Action, e.Target, e.Detail, result)
	}
	return nil
}
