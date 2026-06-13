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
			return emit(cmd, map[string]any{"status": "stub", "operator": auth.CurrentOperator(), "message": "environment identity is active"})
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "+logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			return emit(cmd, map[string]any{"status": "stub", "message": "environment identity has no local session to clear"})
		},
	})
	return root
}
