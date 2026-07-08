package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var helpConceptsCmd = &cobra.Command{
	Use:   "concepts",
	Short: "Explain core gitcollect concepts and mental model",
	Long: `A reference explaining how gitcollect's main abstractions work
and how they map to the underlying GitHub/GitLab platform behaviour.`,
	RunE: runHelpConcepts,
}

func init() {
	rootCmd.AddCommand(helpConceptsCmd)
}

func runHelpConcepts(cmd *cobra.Command, args []string) error {
	fmt.Print(`
GITCOLLECT CONCEPTS

Collection
  A named group of repos stored at ~/.gitcollect/collections/<name>.yaml.
  Collections are local — share them by copying the YAML file to a teammate.
  Each collection has one owner (the person who created it).

Visibility
  private  Only listed members can use the collection. Non-members get
           the same error as "not found" — existence is never confirmed.
  public   Any gitcollect user can clone without authentication.

Access control — two independent layers
  Collection level  who is a member (can use the collection at all)
  Repo level        which groups or individual users can reach each repo
  Both checks must pass. Passing one is not enough.

Identity
  gitcollect stores your platform's permanent numeric user ID, not your
  username. Renaming your GitHub or GitLab account never breaks access.
  Cached login strings are used for display and API paths only.

Platform enforcement
  gitcollect does not maintain its own permission system.
  Every access grant drives a real GitHub/GitLab collaborator API call.
  The local YAML is a declaration of intent — the platform enforces it.
  Editing the YAML by hand grants nothing without the API call.

GitHub collaborator invites
  When you add a member, GitHub sends them a collaborator invite email.
  They must accept it before cloning works — usually within minutes.
  gitcollect detects unaccepted invites and tells you explicitly.
  GitLab access is immediate — no invite step.

Config file locations
  Tokens:      ~/.gitcollect/config           (permissions: 0600)
  Collections: ~/.gitcollect/collections/     (one YAML per collection)
  Audit logs:  ~/.gitcollect/audit/           (newline-delimited JSON)
  Activity:    ~/.gitcollect/activity/        (newline-delimited JSON)
`)
	fmt.Print("  On Windows:  $env:USERPROFILE\\.gitcollect\\\n\n")
	return nil
}
