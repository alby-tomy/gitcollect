package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"gopkg.in/yaml.v3"
)

var (
	exportAll  bool
	exportJSON bool
)

var exportCmd = &cobra.Command{
	Use:   "export [collection]",
	Short: "Print a collection (or all collections) as YAML or JSON",
	Long: `Print a collection to stdout as YAML or JSON for backup or sharing.

Output is pure data — no color, no headers. Redirect to a file or pipe
to another tool. The exported YAML round-trips cleanly back into
~/.gitcollect/collections/ for restore.

Examples:
  gitcollect export my-project
  gitcollect export my-project --json
  gitcollect export --all > all-collections.yaml
  gitcollect export --all --json
  gitcollect export my-project > my-project-backup.yaml`,
	Args: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if all && len(args) > 0 {
			return fmt.Errorf("export: --all cannot be combined with a collection name")
		}
		if !all && len(args) == 0 {
			return fmt.Errorf("export: provide a collection name or use --all")
		}
		return nil
	},
	RunE: runExport,
}

func init() {
	exportCmd.Flags().BoolVar(&exportAll, "all", false, "export all collections")
	exportCmd.Flags().BoolVar(&exportJSON, "json", false, "output as JSON instead of YAML")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	if exportAll {
		return exportAllCollections()
	}
	return exportOne(args[0])
}

func exportOne(name string) error {
	col, err := collection.Load(name)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	return marshalAndPrint([]*collection.Collection{col}, false)
}

func exportAllCollections() error {
	names, err := collection.List()
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	cols := make([]*collection.Collection, 0, len(names))
	for _, n := range names {
		col, err := collection.Load(n)
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		cols = append(cols, col)
	}
	return marshalAndPrint(cols, exportAll && len(cols) > 1)
}

func marshalAndPrint(cols []*collection.Collection, multiDoc bool) error {
	if exportJSON {
		if len(cols) == 1 {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(cols[0])
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cols)
	}

	for i, col := range cols {
		if multiDoc && i > 0 {
			fmt.Println("---")
		}
		b, err := yaml.Marshal(col)
		if err != nil {
			return fmt.Errorf("export: marshal %s: %w", col.Name, err)
		}
		fmt.Print(string(b))
	}
	return nil
}
