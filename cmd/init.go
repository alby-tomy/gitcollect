package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/internal/audit"
	"github.com/alby-tomy/gitcollect/internal/collection"
	"github.com/alby-tomy/gitcollect/internal/config"
	"github.com/alby-tomy/gitcollect/internal/output"
)

var (
	initHost        string
	initDescription string
	initNamespace   string
	initPublic      bool
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Create a new collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVar(&initHost, "host", config.DefaultHost, "platform host the collection's repos live on")
	initCmd.Flags().StringVar(&initDescription, "description", "", "human-readable description of the collection")
	initCmd.Flags().StringVar(&initNamespace, "namespace", "", "org or username under which the repos live (defaults to your own login)")
	initCmd.Flags().BoolVar(&initPublic, "public", false, "create the collection as public instead of the default, private")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := collection.ValidateCollectionName(name); err != nil {
		return NewUsageError(fmt.Errorf("init: %w", err))
	}

	exists, err := collection.Exists(name)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if exists {
		return fmt.Errorf("init: collection %q already exists", name)
	}

	client, err := currentClient(initHost)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	owner, err := currentUserInfo(client)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	visibility := collection.VisibilityPrivate
	if initPublic {
		visibility = collection.VisibilityPublic
	}

	col, err := collection.New(name, initHost, owner, visibility)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	col.Description = initDescription
	col.Namespace = initNamespace

	if err := col.Save(); err != nil {
		recordAudit(audit.AuditEntry{
			Collection: name,
			Actor:      owner.Login,
			Action:     "init",
			Target:     name,
			Detail:     fmt.Sprintf("Collection creation failed (%s)", visibility),
			Result:     "error: " + err.Error(),
		})
		return fmt.Errorf("init: could not save collection: %w", err)
	}

	recordAudit(audit.AuditEntry{
		Collection: name,
		Actor:      owner.Login,
		Action:     "init",
		Target:     name,
		Detail:     fmt.Sprintf("Collection created (%s)", visibility),
		Result:     "ok",
	})

	output.Success("Created collection %q (%s) on %s", name, visibility, initHost)
	output.Suggestion(fmt.Sprintf("gitcollect add %s <repo>", name))
	return nil
}
