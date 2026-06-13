package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/capability/profileinfo"
)

func newProfileInfoCommand() *cobra.Command {
	root := &cobra.Command{Use: "profile-info", Short: "profile info preview commands"}
	var userID int
	var setValues []string
	var sensitive bool
	prev := &cobra.Command{
		Use: "+preview",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := profileinfo.Preview(userID, setValues, sensitive)
			if err != nil {
				return err
			}
			return emit(cmd, data)
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
			data, err := profileinfo.Apply(args[0], yes)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	apply.Flags().BoolVar(&yes, "yes", false, "confirm apply")
	root.AddCommand(prev, apply)
	return root
}
