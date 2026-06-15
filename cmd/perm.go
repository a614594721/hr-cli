package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/gateway"
)

func newPermCommand() *cobra.Command {
	root := &cobra.Command{Use: "perm", Short: "permission explanation commands"}
	var action string
	var targetEID string
	explain := &cobra.Command{
		Use: "explain",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/perm/explain",
				map[string]any{"action": action, "target_eid": targetEID}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	explain.Flags().StringVar(&action, "action", "", "permission action")
	explain.Flags().StringVar(&targetEID, "target-eid", "", "target employee EID")
	_ = explain.MarkFlagRequired("action")
	root.AddCommand(explain)
	return root
}
