package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/capability/attendance"
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
			items, err := attendance.Records(badge, eid, from, to, limit)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": items})
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
			items, err := attendance.Summary(badge, dept, date, limit)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": items})
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
			items, err := attendance.Exceptions(badge, dept, from, to, limit)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": items})
		},
	}
	cmd.Flags().StringVar(&badge, "badge", "", "employee badge")
	cmd.Flags().IntVar(&dept, "dept", 0, "department id")
	cmd.Flags().StringVar(&from, "from", "", "start date")
	cmd.Flags().StringVar(&to, "to", "", "end date")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum rows")
	return cmd
}
