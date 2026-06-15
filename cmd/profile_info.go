package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/gateway"
)

func newProfileInfoCommand() *cobra.Command {
	root := &cobra.Command{Use: "profile-info", Short: "profile info preview commands"}
	var userID int
	var setValues []string
	var sensitive bool
	prev := &cobra.Command{
		Use: "+preview",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/profile-info/preview",
				map[string]any{"user_id": userID, "set": setValues, "sensitive": sensitive}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	prev.Flags().IntVar(&userID, "user-id", 0, "personal info user id")
	prev.Flags().StringArrayVar(&setValues, "set", nil, "field=value")
	prev.Flags().BoolVar(&sensitive, "sensitive", false, "allow sensitive whitelist fields")
	_ = prev.MarkFlagRequired("user-id")

	var yes bool
	apply := &cobra.Command{
		Use:  "+apply <preview-id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/profile-info/apply",
				map[string]any{"preview_id": args[0], "yes": yes}, yes)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	apply.Flags().BoolVar(&yes, "yes", false, "confirm apply")
	root.AddCommand(prev, apply)
	return root
}
