package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/auth"
)

func newAuthCommand() *cobra.Command {
	root := &cobra.Command{Use: "auth", Short: "operator identity commands"}
	root.AddCommand(&cobra.Command{
		Use: "+me",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := auth.Me()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "status",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := auth.Status()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	})
	var login auth.LoginRequest
	loginCmd := &cobra.Command{
		Use: "+login",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := auth.Login(login)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	loginCmd.Flags().IntVar(&login.EID, "eid", 0, "employee EID")
	loginCmd.Flags().StringVar(&login.Badge, "badge", "", "employee badge")
	loginCmd.Flags().StringVar(&login.Email, "email", "", "employee email")
	loginCmd.Flags().StringVar(&login.Phone, "phone", "", "employee phone")
	loginCmd.Flags().StringVar(&login.Name, "name", "", "employee name")
	loginCmd.Flags().StringVar(&login.DingUserID, "ding-userid", "", "DingTalk userid")
	loginCmd.Flags().StringVar(&login.Role, "role", "", "operator role: SELF, HRBP, MANAGER, or HR_ADMIN")
	loginCmd.Flags().BoolVar(&login.DingTalk, "dingtalk", false, "login through DingTalk OAuth broker")
	loginCmd.Flags().StringVar(&login.AuthBaseURL, "auth-base-url", "", "hr-cli auth broker base URL")
	loginCmd.Flags().BoolVar(&login.NoBrowser, "no-browser", false, "print the login URL without opening a browser")
	loginCmd.Flags().IntVar(&login.TimeoutSeconds, "timeout", 180, "DingTalk login timeout in seconds")
	root.AddCommand(loginCmd)
	root.AddCommand(&cobra.Command{
		Use: "+logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := auth.Logout()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	})
	return root
}
