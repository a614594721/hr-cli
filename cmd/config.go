package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/runtime"
)

func newConfigCommand() *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "local configuration commands"}
	root.AddCommand(&cobra.Command{
		Use: "init",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := runtime.Init()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "show",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := runtime.Show()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	})
	return root
}

func newProfileCommand() *cobra.Command {
	root := &cobra.Command{Use: "profile", Short: "gateway profile commands"}
	var profile runtime.Profile
	add := &cobra.Command{
		Use:  "add <name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := runtime.AddProfile(args[0], profile)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	add.Flags().StringVar(&profile.AuthBaseURL, "auth-base-url", "", "hr-gateway base URL")

	use := &cobra.Command{
		Use:  "use <name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := runtime.UseProfile(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	list := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := runtime.ListProfiles()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	root.AddCommand(add, use, list)
	return root
}
