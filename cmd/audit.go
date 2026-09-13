package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/audit"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	auditUser   string
	auditSince  string
	auditFrom   string
	auditTo     string
	auditAction string
	auditJSON   bool
)

var auditCmd = &cobra.Command{
	Use:   "audit <collection>",
	Short: "Show the access change log for a collection",
	Long: `Show the audit log for a collection — every access change since the
collection was created: members added/removed, repos added/removed,
group changes, and visibility changes.

Examples:
  gitcollect audit my-project
  gitcollect audit my-project --since 2024-01-01
  gitcollect audit my-project --user alice
  gitcollect audit my-project --action member.add
  gitcollect audit my-project --json

Filtering:
  --since / --from / --to   date range (RFC3339 or YYYY-MM-DD)
  --user                    show only entries for this user
  --action                  show only entries with this action type

Note: The audit log is stored locally at ~/.gitcollect/audit/<collection>.log.
To preserve audit history across machines, commit this file to a shared
repository or include it in your gitcollect-collections backup.

To export the audit log:
  gitcollect audit my-project --json > my-project-audit-backup.json`,
	Args: cobra.ExactArgs(1),
	RunE: runAudit,
}

func init() {
	auditCmd.Flags().StringVar(&auditUser, "user", "", "filter the log to entries involving this user")
	auditCmd.Flags().StringVar(&auditSince, "since", "", "filter the log to entries within this duration: 1h, 24h, 7d, 30d, or 90d")
	auditCmd.Flags().StringVar(&auditFrom, "from", "", "filter to entries on or after this date (YYYY-MM-DD); mutually exclusive with --since")
	auditCmd.Flags().StringVar(&auditTo, "to", "", "filter to entries on or before this date (YYYY-MM-DD); mutually exclusive with --since")
	auditCmd.Flags().StringVar(&auditAction, "action", "", "filter to entries matching this action (case-insensitive, e.g. member.add)")
	auditCmd.Flags().BoolVar(&auditJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(auditCmd)
}

// sinceDurations is the exact, closed set of values --since accepts on
// every command that uses parseSince (audit, activity) — no other
// duration string is valid, not even otherwise-well-formed ones like 30m
// or 2h30m. Deliberately strict rather than accepting anything
// time.ParseDuration understands: a fixed set of values is easier to
// document, tab-complete, and never silently misparse a typo'd flag into
// a different duration than intended.
var sinceDurations = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
}

// sinceDurationsOrdered is sinceDurations' keys in logical duration order,
// used to produce a deterministic, human-readable error message.
var sinceDurationsOrdered = []string{"1h", "24h", "7d", "30d", "90d"}

// parseSince accepts only the values in sinceDurations.
func parseSince(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if d, ok := sinceDurations[s]; ok {
		return d, nil
	}
	return 0, fmt.Errorf("invalid --since value %q\n  Valid values: %s", s, strings.Join(sinceDurationsOrdered, ", "))
}

func parseAuditDate(s, flag string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --%s value %q: expected YYYY-MM-DD (e.g. 2024-01-31)", flag, s)
	}
	return t, nil
}

func runAudit(cmd *cobra.Command, args []string) error {
	name := args[0]

	// --since and --from/--to are mutually exclusive.
	if auditSince != "" && (auditFrom != "" || auditTo != "") {
		return NewUsageError(fmt.Errorf("audit: --since and --from/--to are mutually exclusive"))
	}

	since, err := parseSince(auditSince)
	if err != nil {
		return NewUsageError(fmt.Errorf("audit: %w", err))
	}

	from, err := parseAuditDate(auditFrom, "from")
	if err != nil {
		return NewUsageError(fmt.Errorf("audit: %w", err))
	}
	to, err := parseAuditDate(auditTo, "to")
	if err != nil {
		return NewUsageError(fmt.Errorf("audit: %w", err))
	}
	// --to is inclusive through end-of-day.
	if !to.IsZero() {
		to = to.Add(24*time.Hour - time.Nanosecond)
	}

	if _, _, _, err := loadForRead(name); err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	entries, err := audit.Read(name)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	entries = audit.Filter(entries, auditUser, since)
	entries = audit.FilterByDate(entries, from, to)
	entries = audit.FilterByAction(entries, auditAction)

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
