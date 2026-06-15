package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/gateway"
)

func newEmployeeCommand() *cobra.Command {
	root := &cobra.Command{Use: "employee", Short: "employee query commands"}
	var name, badge, phone string
	var limit int
	find := &cobra.Command{
		Use: "+find",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/employee/find",
				map[string]any{"name": name, "badge": badge, "phone": phone, "limit": limit}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	find.Flags().StringVar(&name, "name", "", "employee name")
	find.Flags().StringVar(&badge, "badge", "", "employee badge")
	find.Flags().StringVar(&phone, "phone", "", "employee phone")
	find.Flags().IntVar(&limit, "limit", 20, "maximum rows")

	var eid int
	get := &cobra.Command{
		Use: "get",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/employee/get",
				map[string]any{"eid": eid}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	get.Flags().IntVar(&eid, "eid", 0, "employee EID")
	_ = get.MarkFlagRequired("eid")

	root.AddCommand(find, get)
	return root
}
