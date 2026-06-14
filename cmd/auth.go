package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/auth"
)

func newAuthCommand() *cobra.Command {
	root := &cobra.Command{Use: "auth", Short: "operator identity commands"}
	for _, name := range []string{"+me", "status"} {
		root.AddCommand(&cobra.Command{
			Use: name,
			RunE: func(cmd *cobra.Command, args []string) error {
				return emit(cmd, auth.CurrentOperator())
			},
		})
	}
	root.AddCommand(&cobra.Command{
		Use: "+login",
		RunE: func(cmd *cobra.Command, args []string) error {
			return emit(cmd, map[string]any{
				"status":   "active",
				"operator": auth.CurrentOperator(),
				"mode":     "environment_or_profile",
				"message":  "operator identity resolved from environment variables or active profile",
			})
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "+logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			return emit(cmd, map[string]any{
				"status":  "no_session",
				"mode":    "environment_or_profile",
				"message": "no local auth session is stored; clear HR_OPERATOR_* environment variables or switch profile to change identity",
			})
		},
	})
	return root
}
