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
	root := &cobra.Command{Use: "profile", Short: "database and operator profile commands"}
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
	add.Flags().StringVar(&profile.DBEnv, "db-env", "", "database environment")
	add.Flags().StringVar(&profile.DBHost, "db-host", "", "database host")
	add.Flags().StringVar(&profile.DBPort, "db-port", "", "database port")
	add.Flags().StringVar(&profile.DBName, "db-name", "", "database name")
	add.Flags().StringVar(&profile.DBUser, "db-user", "", "database user")
	add.Flags().StringVar(&profile.CredentialTarget, "credential-target", "", "external credential target name")
	add.Flags().StringVar(&profile.AuthBaseURL, "auth-base-url", "", "hr-cli auth broker base URL")
	add.Flags().StringVar(&profile.OperatorEID, "operator-eid", "", "operator EID")
	add.Flags().StringVar(&profile.OperatorURID, "operator-urid", "", "operator URID")
	add.Flags().StringVar(&profile.OperatorBadge, "operator-badge", "", "operator badge")
	add.Flags().StringVar(&profile.OperatorName, "operator-name", "", "operator name")
	add.Flags().StringVar(&profile.OperatorRole, "operator-role", "", "operator role")

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

func newCredentialCommand() *cobra.Command {
	root := &cobra.Command{Use: "credential", Short: "credential reference commands"}
	root.AddCommand(&cobra.Command{
		Use: "status",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := runtime.CredentialStatus()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	})
	return root
}
