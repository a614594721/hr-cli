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
	var statusVerify bool
	statusCmd := &cobra.Command{
		Use: "status",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := auth.Status(statusVerify)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	statusCmd.Flags().BoolVar(&statusVerify, "verify", false, "verify token against the auth broker")
	root.AddCommand(statusCmd)
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
	loginCmd.Flags().BoolVar(&login.NoWait, "no-wait", false, "start DingTalk login and return without polling")
	loginCmd.Flags().StringVar(&login.LoginID, "login-id", "", "resume polling a previous DingTalk login id")
	loginCmd.Flags().StringVar(&login.LoginSecret, "login-secret", "", "resume polling a previous DingTalk login secret")
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
