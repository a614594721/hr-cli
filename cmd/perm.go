package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/auth"
)

func newPermCommand() *cobra.Command {
	root := &cobra.Command{Use: "perm", Short: "permission explanation commands"}
	var action string
	var targetEID string
	explain := &cobra.Command{
		Use: "explain",
		RunE: func(cmd *cobra.Command, args []string) error {
			operator := auth.CurrentOperator()
			decision := "limited"
			if operator.Role == "HR_ADMIN" {
				decision = "allow"
			}
			return emit(cmd, map[string]any{
				"action": action, "target_eid": targetEID, "operator": operator,
				"decision": decision, "reason": "V1a permission engine uses environment role only",
			})
		},
	}
	explain.Flags().StringVar(&action, "action", "", "permission action")
	explain.Flags().StringVar(&targetEID, "target-eid", "", "target employee EID")
	_ = explain.MarkFlagRequired("action")
	root.AddCommand(explain)
	return root
}
