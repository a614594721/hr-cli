package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/gateway"
)

func newAttendanceCommand() *cobra.Command {
	root := &cobra.Command{Use: "attendance", Short: "attendance read commands"}
	root.AddCommand(attendanceRecordsCommand(), attendanceSummaryCommand(), attendanceExceptionsCommand())
	return root
}

func attendanceRecordsCommand() *cobra.Command {
	var badge, from, to string
	var eid, limit int
	cmd := &cobra.Command{
		Use: "+records",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/attendance/records",
				map[string]any{"badge": badge, "eid": eid, "from": from, "to": to, "limit": limit}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	cmd.Flags().StringVar(&badge, "badge", "", "employee badge")
	cmd.Flags().IntVar(&eid, "eid", 0, "employee EID")
	cmd.Flags().StringVar(&from, "from", "", "start date")
	cmd.Flags().StringVar(&to, "to", "", "end date")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum rows")
	return cmd
}

func attendanceSummaryCommand() *cobra.Command {
	var badge, date string
	var dept, limit int
	cmd := &cobra.Command{
		Use: "+summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/attendance/summary",
				map[string]any{"badge": badge, "dept": dept, "date": date, "limit": limit}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	cmd.Flags().StringVar(&badge, "badge", "", "employee badge")
	cmd.Flags().IntVar(&dept, "dept", 0, "department id")
	cmd.Flags().StringVar(&date, "date", "", "date")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum rows")
	return cmd
}

func attendanceExceptionsCommand() *cobra.Command {
	var badge, from, to string
	var dept, limit int
	cmd := &cobra.Command{
		Use: "+exceptions",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/attendance/exceptions",
				map[string]any{"badge": badge, "dept": dept, "from": from, "to": to, "limit": limit}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	cmd.Flags().StringVar(&badge, "badge", "", "employee badge")
	cmd.Flags().IntVar(&dept, "dept", 0, "department id")
	cmd.Flags().StringVar(&from, "from", "", "start date")
	cmd.Flags().StringVar(&to, "to", "", "end date")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum rows")
	return cmd
}
