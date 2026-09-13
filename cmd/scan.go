package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

var (
	scanOrg     string
	scanFrom    string
	scanGroupBy string
	scanDryRun  bool
	scanApply   bool
	scanVerify  bool
	scanNoArch  bool
)

var scanCmd = &cobra.Command{
	Use:   "scan --org <org> [--from github|gitlab]",
	Short: "Auto-discover org repos and group them into collections by namespace prefix",
	Long: `Fetches every repository in a GitHub org or GitLab group and groups them by
the common prefix in their names (e.g. payments-checkout, payments-gateway →
"payments" collection).

--verify reports what the scan itself establishes: who the token
authenticated as, and how many repositories that token can see in the org.
It does not report a per-repo access level — listing an org does not reveal
one — so treat the repo count as a lower bound: anything the token cannot
see is absent from it entirely.

By default the command prints a preview. Use --apply to write collection YAML
files to ~/.gitcollect/collections/. Use --dry-run to see what would be written
without touching the filesystem.`,
	Args: cobra.NoArgs,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().StringVar(&scanOrg, "org", "", "org or group to scan (required)")
	scanCmd.Flags().StringVar(&scanFrom, "from", "", "platform: github or gitlab (default: github.com)")
	scanCmd.Flags().StringVar(&scanGroupBy, "group-by", "prefix", "grouping strategy: prefix or flat")
	scanCmd.Flags().BoolVar(&scanDryRun, "dry-run", false, "preview collections without writing files")
	scanCmd.Flags().BoolVar(&scanApply, "apply", false, "write collection YAML files")
	scanCmd.Flags().BoolVar(&scanVerify, "verify", false, "report what the current token could actually see")
	scanCmd.Flags().BoolVar(&scanNoArch, "no-archived", false, "exclude archived repos")
	if err := scanCmd.MarkFlagRequired("org"); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(scanCmd)
}

// scanGroup is a named group of repos that share a common prefix.
type scanGroup struct {
	name  string
	repos []api.RepoInfo
}

func runScan(_ *cobra.Command, _ []string) error {
	host := scanFrom
	switch host {
	case "", "github":
		host = "github.com"
	case "gitlab":
		host = "gitlab.com"
	}

	client, err := currentClient(host)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	if host == "github.com" {
		if err := checkScanScopes(client); err != nil {
			return err
		}
	}

	caller, err := currentUser(client)
	if err != nil {
		return fmt.Errorf("scan: resolve user: %w", err)
	}
	callerID, err := currentUserID(client)
	if err != nil {
		return fmt.Errorf("scan: resolve user id: %w", err)
	}

	output.Info("Scanning %s/%s...\n", host, scanOrg)

	repos, err := client.ListOrgRepos(scanOrg)
	if err != nil {
		return fmt.Errorf("scan: list repos: %w", err)
	}

	if scanNoArch {
		filtered := repos[:0]
		for _, r := range repos {
			if !r.Archived {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	}

	if len(repos) == 0 {
		output.Dim("No repositories found in %s/%s.", host, scanOrg)
		return nil
	}

	output.Success("Found %d repositor%s in %s/%s", len(repos),
		plural(len(repos), "y", "ies"), host, scanOrg)

	// Group repos.
	var groups []scanGroup
	switch scanGroupBy {
	case "flat":
		groups = []scanGroup{{name: scanOrg, repos: repos}}
	default: // "prefix"
		groups = groupByPrefix(repos)
	}

	if scanVerify {
		printVerificationChain(caller, scanOrg, repos)
	}

	printScanSummary(groups)

	if !scanApply && !scanDryRun {
		fmt.Println()
		output.Dim("Use --apply to write collection files, or --dry-run to preview them.")
		return nil
	}

	// Apply or dry-run: write/preview one collection per group.
	var written, skipped int
	for _, g := range groups {
		colName := sanitizeCollectionName(scanOrg + "-" + g.name)
		if g.name == scanOrg {
			colName = sanitizeCollectionName(scanOrg)
		}

		if scanDryRun {
			fmt.Printf("\n[dry-run] Would write collection %q with %d repo%s:\n",
				colName, len(g.repos), plural(len(g.repos), "", "s"))
			for _, r := range g.repos {
				arch := ""
				if r.Archived {
					arch = " (archived)"
				}
				fmt.Printf("    %s%s\n", r.Name, arch)
			}
			continue
		}

		// --apply: create the collection file, but never clobber one that
		// is already there. collection.New does NOT check existence — it
		// only validates the name — so the existence test has to happen
		// here. Relying on New's error meant an existing collection was
		// silently overwritten (losing its members, groups and per-repo
		// access rules) while an invalid name was misreported as
		// "already exists".
		exists, err := collection.Exists(colName)
		if err != nil {
			output.Warn("  could not check whether %q exists: %v", colName, err)
			continue
		}
		if exists {
			output.Dim("  %s (already exists, skipped)", colName)
			skipped++
			continue
		}

		col, err := collection.New(colName, host,
			api.UserInfo{ID: callerID, Login: caller}, collection.VisibilityPrivate)
		if err != nil {
			output.Warn("  could not create %q: %v", colName, err)
			continue
		}
		col.Namespace = scanOrg

		repoAccess := make([]collection.RepoAccess, 0, len(g.repos))
		for _, r := range g.repos {
			repoAccess = append(repoAccess, collection.RepoAccess{
				Name:   r.Name,
				Groups: []string{},
				Users:  []string{},
			})
		}
		col.Repos = repoAccess

		if err := col.Save(); err != nil {
			output.Warn("  could not save %q: %v", colName, err)
			continue
		}
		output.Success("  %-40s (%d repo%s)", colName, len(g.repos), plural(len(g.repos), "", "s"))
		written++
	}

	if scanApply {
		fmt.Println()
		if written > 0 {
			output.Success("Created %d collection%s.", written, plural(written, "", "s"))
			fmt.Printf("Run: gitcollect sync <name>   to clone repos into each collection.\n")
		}
		if skipped > 0 {
			output.Dim("%d collection%s already existed and were skipped.", skipped, plural(skipped, "", "s"))
		}
	}

	return nil
}

// checkScanScopes verifies the GitHub token has at least public_repo scope,
// which is the minimum needed to list org repos. Warns if repo scope is absent
// (private repos will be invisible). Only called for GitHub — GitLab tokens
// don't expose an OAuth scope header.
func checkScanScopes(client api.Client) error {
	scopes, err := client.GetTokenScopes()
	if err != nil {
		return fmt.Errorf("scan: could not check token scopes: %w", err)
	}
	scopeSet := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = true
	}
	if !scopeSet["repo"] && !scopeSet["public_repo"] {
		return fmt.Errorf(
			"scan: GitHub token is missing required scopes (need repo or public_repo)\n\n" +
				"  Generate a new token at:\n" +
				"  https://github.com/settings/tokens/new?scopes=repo\n\n" +
				"  Then run: gitcollect auth",
		)
	}
	if !scopeSet["repo"] {
		output.Warn("Token has public_repo scope only — private repos will not be listed.")
		output.Dim("  Regenerate with 'repo' scope to include private repos.")
	}
	return nil
}

// groupByPrefix groups repos by the first segment of their name before a
// hyphen or underscore. A repo whose name has no separator becomes its own
// single-repo group, named after the repo — there is no shared "other"
// bucket, so an org with many separator-less names produces many one-repo
// collections. Use --group-by flat to put everything in one instead.
func groupByPrefix(repos []api.RepoInfo) []scanGroup {
	buckets := make(map[string][]api.RepoInfo)
	for _, r := range repos {
		prefix := repoPrefix(r.Name)
		buckets[prefix] = append(buckets[prefix], r)
	}

	// Sort bucket names for stable output.
	names := make([]string, 0, len(buckets))
	for n := range buckets {
		names = append(names, n)
	}
	sort.Strings(names)

	groups := make([]scanGroup, 0, len(names))
	for _, n := range names {
		groups = append(groups, scanGroup{name: n, repos: buckets[n]})
	}
	return groups
}

// repoPrefix returns the first hyphen/underscore-delimited segment of name,
// or the full name if there is no separator.
func repoPrefix(name string) string {
	for i, c := range name {
		if c == '-' || c == '_' {
			return name[:i]
		}
	}
	return name
}

// sanitizeCollectionName converts a raw name into a valid collection name:
// replaces runs of non-alphanumeric/non-dash chars with a single dash and
// strips leading/trailing dashes.
func sanitizeCollectionName(name string) string {
	var b strings.Builder
	prevDash := false
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

func printScanSummary(groups []scanGroup) {
	fmt.Printf("\nGroups discovered (%d):\n", len(groups))
	for _, g := range groups {
		fmt.Printf("  %-24s %d repo%s\n", g.name, len(g.repos), plural(len(g.repos), "", "s"))
		for _, r := range g.repos {
			arch := ""
			if r.Archived {
				arch = " [archived]"
			}
			fmt.Printf("      • %s%s\n", r.Name, arch)
		}
	}
}

// printVerificationChain reports what the scan actually established.
//
// It used to print three fixed ✓ lines regardless of what the token could
// reach, and the help promised a per-repo access level that was never
// computed — a verification step that always passes is worse than none,
// because it reassures about access it never checked. Everything below is
// derived from the listing that just succeeded, and the closing note says
// plainly what the listing cannot tell you.
func printVerificationChain(caller, org string, repos []api.RepoInfo) {
	archived, private := 0, 0
	for _, r := range repos {
		if r.Archived {
			archived++
		}
		if r.Private {
			private++
		}
	}

	fmt.Println()
	fmt.Printf("Verified with the current token:\n")
	fmt.Printf("  authenticated as  %s\n", caller)
	fmt.Printf("  org listed        %s\n", org)
	fmt.Printf("  repos visible     %d (%d private, %d archived)\n", len(repos), private, archived)
	fmt.Println()
	output.Dim("  Repositories this token cannot see are not counted above.")
	if private == 0 {
		output.Dim("  No private repos were returned — check the token has 'repo' scope if you expected some.")
	}
}

func plural(n int, singular, pluralSuffix string) string {
	if n == 1 {
		return singular
	}
	return pluralSuffix
}
