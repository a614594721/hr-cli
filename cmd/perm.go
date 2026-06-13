package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/perm"
)

func newPermCommand() *cobra.Command {
	root := &cobra.Command{Use: "perm", Short: "permission explanation commands"}
	var action string
	var targetEID string
	explain := &cobra.Command{
		Use: "explain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return emit(cmd, perm.Explain(action, targetEID))
		},
	}
	explain.Flags().StringVar(&action, "action", "", "permission action")
	explain.Flags().StringVar(&targetEID, "target-eid", "", "target employee EID")
	_ = explain.MarkFlagRequired("action")
	root.AddCommand(explain)
	return root
}
