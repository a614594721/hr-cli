package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/capability/employee"
)

func newEmployeeCommand() *cobra.Command {
	root := &cobra.Command{Use: "employee", Short: "employee query commands"}
	var name, badge, phone string
	var limit int
	find := &cobra.Command{
		Use: "+find",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, truncated, err := employee.Find(name, badge, phone, limit)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": items, "truncated": truncated})
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
			data, err := employee.Get(eid)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	get.Flags().IntVar(&eid, "eid", 0, "employee EID")
	_ = get.MarkFlagRequired("eid")

	root.AddCommand(find, get)
	return root
}
