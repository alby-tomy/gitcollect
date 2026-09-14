package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/config"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

// doctorCheck holds the result of a single diagnostic check.
type doctorCheck struct {
	Label string `json:"label"`
	// Status is "ok", "info", "warn" or "error". Only "error" affects
	// the exit code; "info" is a standing note, not a finding, and is
	// excluded from the warning count.
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

var doctorJSON bool

// doctorCheckFn is injectable for tests; returns the list of diagnostic checks.
var doctorCheckFn = func() []doctorCheck { return runDoctorChecks() }

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check gitcollect configuration and token validity",
	Long: `Check gitcollect configuration and token validity.

Reports authentication status for each configured host, collection
health (staleness), and token scope availability.

Examples:
  gitcollect doctor
  gitcollect doctor --json

Exit codes:
  0  all checks ok or only warnings
  1  at least one check failed`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	checks := doctorCheckFn()

	if doctorJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(checks); err != nil {
			return err
		}
		for _, c := range checks {
			if c.Status == "error" {
				return fmt.Errorf("doctor: %d check(s) failed", countStatus(checks, "error"))
			}
		}
		return nil
	}

	fmt.Fprintf(os.Stderr, "\nChecking gitcollect configuration...\n\n")

	var sections = map[string][]doctorCheck{}
	var sectionOrder []string
	for _, c := range checks {
		sec := checkSection(c.Label)
		if _, seen := sections[sec]; !seen {
			sectionOrder = append(sectionOrder, sec)
		}
		sections[sec] = append(sections[sec], c)
	}

	for _, sec := range sectionOrder {
		fmt.Fprintf(os.Stderr, "%s\n", sec)
		for _, c := range sections[sec] {
			icon := checkIcon(c.Status)
			fmt.Fprintf(os.Stderr, "%s %s\n", icon, c.Message)
			if c.Fix != "" && c.Status != "ok" {
				label := "Run"
				if c.Status == "info" {
					label = "Tip"
				}
				fmt.Fprintf(os.Stderr, "  %s: %s\n", label, c.Fix)
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	warns := countStatus(checks, "warn")
	errs := countStatus(checks, "error")
	if errs > 0 {
		output.Error("gitcollect doctor complete · %d warning(s), %d error(s)", warns, errs)
		return fmt.Errorf("doctor: %d check(s) failed", errs)
	}
	if warns > 0 {
		output.Warn("gitcollect doctor complete · %d warning(s)", warns)
		return nil
	}
	output.Success("gitcollect doctor complete · all checks passed")
	return nil
}

func runDoctorChecks() []doctorCheck {
	var checks []doctorCheck

	// AUTH: check each configured host.
	hosts, err := config.Hosts()
	if err != nil || len(hosts) == 0 {
		checks = append(checks, doctorCheck{
			Label:   "AUTH/github.com",
			Status:  "error",
			Message: "github.com — no token stored",
			Fix:     "gitcollect auth",
		})
	} else {
		for _, host := range hosts {
			token, err := config.LoadToken(host)
			if err != nil || token == "" {
				checks = append(checks, doctorCheck{
					Label:   "AUTH/" + host,
					Status:  "error",
					Message: host + " — no token stored",
					Fix:     "gitcollect auth --host " + host,
				})
				continue
			}
			client := api.NewClient(host, token)
			user, err := client.GetAuthenticatedUser()
			if err != nil {
				checks = append(checks, doctorCheck{
					Label:   "AUTH/" + host,
					Status:  "error",
					Message: host + " — token invalid or expired",
					Fix:     "gitcollect auth --host " + host,
				})
				continue
			}
			checks = append(checks, doctorCheck{
				Label:   "AUTH/" + host,
				Status:  "ok",
				Message: fmt.Sprintf("%s — authenticated as %s", host, user.Login),
			})

			// TOKEN SCOPES: check for repo scope.
			scopes, err := client.GetTokenScopes()
			if err == nil {
				hasRepo := false
				for _, s := range scopes {
					if s == "repo" {
						hasRepo = true
						break
					}
				}
				if !hasRepo {
					checks = append(checks, doctorCheck{
						Label:   "SCOPES/" + host,
						Status:  "warn",
						Message: host + " — repo scope missing (required for private repos)",
						Fix:     "gitcollect auth --host " + host,
					})
				} else {
					checks = append(checks, doctorCheck{
						Label:   "SCOPES/" + host,
						Status:  "ok",
						Message: host + " — repo scope present",
					})
				}
			}
		}
	}

	// COLLECTIONS: check staleness.
	names, err := collection.List()
	if err == nil {
		for _, name := range names {
			col, err := collection.Load(name)
			if err != nil {
				checks = append(checks, doctorCheck{
					Label:   "COLLECTIONS/" + name,
					Status:  "warn",
					Message: fmt.Sprintf("%s — could not load: %v", name, err),
				})
				continue
			}
			age := time.Since(col.UpdatedAt)
			if age > 30*24*time.Hour {
				checks = append(checks, doctorCheck{
					Label:   "COLLECTIONS/" + name,
					Status:  "warn",
					Message: fmt.Sprintf("%s — last updated %.0f days ago — consider sharing the latest YAML", name, age.Hours()/24),
				})
			} else {
				checks = append(checks, doctorCheck{
					Label:   "COLLECTIONS/" + name,
					Status:  "ok",
					Message: fmt.Sprintf("%s — %d repos · %d members · %s", name, len(col.Repos), len(col.Members), col.Host),
				})
			}
		}
	}

	// Audit log portability reminder. Status "info", not "warn": this is a
	// standing property of how gitcollect stores audit logs, not something
	// wrong with this installation that the user could act on. Reporting it
	// as a warning meant every run ended with at least one warning and the
	// "all checks passed" result was unreachable — which trains people to
	// ignore the warning count entirely.
	checks = append(checks, doctorCheck{
		Label:   "AUDIT",
		Status:  "info",
		Message: "Audit logs are stored locally only — not backed up or shared",
		Fix:     "gitcollect audit <collection> --json > backup.json",
	})

	return checks
}

func checkSection(label string) string {
	if idx := strings.Index(label, "/"); idx >= 0 {
		return label[:idx]
	}
	return label
}

func checkIcon(status string) string {
	switch status {
	case "ok":
		return "✓"
	case "warn":
		return "⚠"
	case "info":
		return "·"
	default:
		return "✗"
	}
}

func countStatus(checks []doctorCheck, status string) int {
	n := 0
	for _, c := range checks {
		if c.Status == status {
			n++
		}
	}
	return n
}
