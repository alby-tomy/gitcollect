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
	scanOrg         string
	scanFrom        string
	scanGroupBy     string
	scanUser        string
	scanInteractive bool
	scanDryRun      bool
	scanApply       bool
	scanVerify      bool
	scanNoArch      bool
)

var scanCmd = &cobra.Command{
	Use:   "scan (--org <org> | --user <login>) [--from github|gitlab]",
	Short: "Auto-discover repos and group them into collections by shared name",
	Long: `Fetches every repository in a GitHub org or GitLab group — or, with --user,
in a personal account — and groups them by the most widely shared meaningful
word in their names.

Grouping strategies (--group-by):

  token   (default) the shared word wherever it appears, so china-pricing,
          eu-pricing and us-pricing all land in "pricing". A word must be
          shared by at least two repos to name a group, and generic terms
          (service, api, core, ...) are ignored so cart-service and
          search-service are not collapsed into "service".
  prefix  the first hyphenated segment only. Use it when repos are named
          with a deliberate namespace prefix (payments-gateway → payments);
          note it splits china-pricing and eu-pricing apart.
  flat    one collection for everything.

Use --interactive to confirm each repo's category before anything is
written, and to reassign or drop the ones that were placed wrongly.

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
	scanCmd.Flags().StringVar(&scanOrg, "org", "", "org or group to scan (use --user for a personal account)")
	scanCmd.Flags().StringVar(&scanFrom, "from", "", "platform: github or gitlab (default: github.com)")
	scanCmd.Flags().StringVar(&scanGroupBy, "group-by", "token", "grouping strategy: token, prefix or flat")
	scanCmd.Flags().StringVar(&scanUser, "user", "", "personal account to scan instead of an org (use your own login for private repos)")
	scanCmd.Flags().BoolVar(&scanInteractive, "interactive", false, "confirm each repo's category before writing")
	scanCmd.Flags().BoolVar(&scanDryRun, "dry-run", false, "preview collections without writing files")
	scanCmd.Flags().BoolVar(&scanApply, "apply", false, "write collection YAML files")
	scanCmd.Flags().BoolVar(&scanVerify, "verify", false, "report what the current token could actually see")
	scanCmd.Flags().BoolVar(&scanNoArch, "no-archived", false, "exclude archived repos")
	// An org and a personal account are separate endpoints, so exactly one
	// of them has to be named - "--org" alone used to be required, which is
	// what made a personal account impossible to scan.
	scanCmd.MarkFlagsMutuallyExclusive("org", "user")
	scanCmd.MarkFlagsOneRequired("org", "user")
	rootCmd.AddCommand(scanCmd)
}

// scanGroup is a named group of repos that share a common name token.
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

	// An org and a personal account are different endpoints; asking for one
	// through the other's just fails, so the target decides which to call.
	target := scanOrg
	output.Info("Scanning %s/%s...\n", host, target)

	var repos []api.RepoInfo
	if scanUser != "" {
		target = scanUser
		// Passing an empty user asks for the authenticated account's own
		// listing, the only one that includes private repos - so scanning
		// yourself sees everything you own, not just what is public.
		lookup := scanUser
		if strings.EqualFold(scanUser, caller) {
			lookup = ""
		}
		repos, err = client.ListUserRepos(lookup)
	} else {
		repos, err = client.ListOrgRepos(scanOrg)
	}
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
		output.Dim("No repositories found in %s/%s.", host, target)
		return nil
	}

	output.Success("Found %d repositor%s in %s/%s", len(repos),
		plural(len(repos), "y", "ies"), host, target)

	// Group repos.
	var groups []scanGroup
	switch scanGroupBy {
	case "flat":
		groups = []scanGroup{{name: target, repos: repos}}
	case "prefix":
		groups = groupByPrefix(repos)
	default: // "token"
		groups = groupByToken(repos)
	}

	if scanInteractive {
		groups = reviewGroups(groups)
		if len(groups) == 0 {
			output.Dim("Every repo was dropped; nothing to write.")
			return nil
		}
	}

	if scanVerify {
		printVerificationChain(caller, target, repos)
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
		colName := sanitizeCollectionName(target + "-" + g.name)
		if g.name == target {
			colName = sanitizeCollectionName(target)
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

// genericToken holds name parts that describe what a repo *is* rather than
// what it belongs to. Without this, "cart-service", "search-service" and
// "availability-service" all collapse into a single "service" bucket, which
// is a worse answer than not grouping at all.
var genericToken = map[string]bool{
	"api": true, "app": true, "application": true, "backend": true,
	"cli": true, "client": true, "common": true, "config": true,
	"core": true, "demo": true, "deploy": true, "docs": true,
	"example": true, "frontend": true, "infra": true, "internal": true,
	"lib": true, "library": true, "main": true, "manager": true,
	"microservice": true, "module": true, "pkg": true, "platform": true,
	"proto": true, "repo": true, "sdk": true, "server": true,
	"service": true, "shared": true, "svc": true, "system": true,
	"test": true, "tests": true, "tool": true, "tools": true,
	"ui": true, "utils": true, "util": true, "web": true, "worker": true,
}

// tokenize splits a repo name into its lowercase word parts.
func tokenize(name string) []string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	})
	return parts
}

// groupByToken buckets repos by the most widely shared meaningful word in
// their names, wherever that word sits.
//
// This exists because grouping on the first segment alone gets the common
// case backwards. Given china-pricing, eu-pricing and us-pricing - one
// team's pricing module, split by region - a first-segment rule yields three
// collections named "china", "eu" and "us", scattering the very module the
// user was trying to assemble. The shared word "pricing" is the category,
// and it is a suffix here and a prefix in payments-gateway, so position
// cannot be what decides.
//
// A token has to appear in at least two repos to name a group, so a word
// unique to one repo never invents a category. Repos sharing nothing fall
// back to their own first meaningful word, which keeps them addressable
// instead of dumping them in a "misc" pile.
func groupByToken(repos []api.RepoInfo) []scanGroup {
	freq := make(map[string]int)
	for _, r := range repos {
		// Count each token once per repo, so "pricing-pricing" cannot
		// out-vote a token genuinely shared across two repos.
		seen := make(map[string]bool)
		for _, t := range tokenize(r.Name) {
			if len(t) < 2 || genericToken[t] || seen[t] {
				continue
			}
			seen[t] = true
			freq[t]++
		}
	}

	buckets := make(map[string][]api.RepoInfo)
	for _, r := range repos {
		buckets[bestToken(r.Name, freq)] = append(buckets[bestToken(r.Name, freq)], r)
	}

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

// bestToken picks the group name for one repo: the token it shares with the
// most other repos. Ties break on the longer token, then alphabetically, so
// the same input always produces the same collections.
func bestToken(name string, freq map[string]int) string {
	best, bestN := "", 0
	fallback := ""
	for _, t := range tokenize(name) {
		if len(t) < 2 || genericToken[t] {
			continue
		}
		if fallback == "" {
			fallback = t
		}
		n := freq[t]
		if n < 2 {
			continue // unique to this repo: not a category
		}
		if n > bestN || (n == bestN && (len(t) > len(best) || (len(t) == len(best) && t < best))) {
			best, bestN = t, n
		}
	}
	if best != "" {
		return best
	}
	if fallback != "" {
		return fallback
	}
	return strings.ToLower(name)
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

// reviewGroups walks the proposed categories and lets the operator confirm
// each repo before anything is written.
//
// Grouping by name is a guess. It is a good guess when repos are named
// consistently and a poor one when they are not, and the command had no way
// to say "this one does not belong here" short of editing the YAML
// afterwards. This is that step: keep, drop, or file the repo under a
// different category.
//
// Input is read through output.Prompt, so a non-interactive
// stdin (a pipe, CI) answers with the default and keeps the repo where the
// grouping put it - the same result as not passing --interactive at all.
func reviewGroups(groups []scanGroup) []scanGroup {
	fmt.Println()
	output.Info("Reviewing %d propose%s categor%s. Enter keeps the suggestion.",
		len(groups), plural(len(groups), "d", "d"), plural(len(groups), "y", "ies"))
	output.Dim("  [Enter] keep   d) drop   <name>) file under that category instead")
	fmt.Println()

	reassigned := make(map[string][]api.RepoInfo)
	for _, g := range groups {
		fmt.Printf("%s\n", strings.ToUpper(g.name))
		for _, r := range g.repos {
			answer := output.Prompt(fmt.Sprintf("  %s [%s]: ", r.Name, g.name))
			switch {
			case answer == "":
				reassigned[g.name] = append(reassigned[g.name], r)
			case answer == "d" || answer == "drop":
				output.Dim("    dropped %s", r.Name)
			default:
				dest := sanitizeCollectionName(strings.ToLower(answer))
				if dest == "" {
					// Unusable name: keeping the repo beats silently
					// discarding it over a typo.
					output.Warn("    %q is not a usable category name; keeping %s in %s", answer, r.Name, g.name)
					reassigned[g.name] = append(reassigned[g.name], r)
					continue
				}
				reassigned[dest] = append(reassigned[dest], r)
				output.Dim("    %s → %s", r.Name, dest)
			}
		}
	}

	names := make([]string, 0, len(reassigned))
	for n := range reassigned {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]scanGroup, 0, len(names))
	for _, n := range names {
		out = append(out, scanGroup{name: n, repos: reassigned[n]})
	}
	return out
}
