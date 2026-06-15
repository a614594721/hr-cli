package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/gateway"
)

func newApprovalCommand() *cobra.Command {
	root := &cobra.Command{Use: "approval", Short: "approval query commands"}
	var assignee string
	var limit int
	tasks := &cobra.Command{
		Use: "+tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/approval/tasks",
				map[string]any{"assignee": assignee, "limit": limit}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	tasks.Flags().StringVar(&assignee, "assignee", "", "task assignee")
	tasks.Flags().IntVar(&limit, "limit", 50, "maximum rows")

	var taskID int
	task := &cobra.Command{
		Use: "+task",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/approval/task",
				map[string]any{"task_id": taskID}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	task.Flags().IntVar(&taskID, "task-id", 0, "task id")
	_ = task.MarkFlagRequired("task-id")

	var employee, status string
	var instanceLimit int
	instances := &cobra.Command{
		Use: "+instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/approval/instances",
				map[string]any{"employee": employee, "status": status, "limit": instanceLimit}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	instances.Flags().StringVar(&employee, "employee", "", "employee EID/URID")
	instances.Flags().StringVar(&status, "status", "", "workflow status: pending, closed, approved, rejected, or numeric FORMSTATE")
	instances.Flags().IntVar(&instanceLimit, "limit", 50, "maximum rows")

	root.AddCommand(tasks, task, instances, approvalWriteCommand("+approve"), approvalWriteCommand("+reject"), approvalWriteCommand("+transfer"))
	return root
}

func approvalWriteCommand(name string) *cobra.Command {
	var taskID int
	var comment, reason, toBadge string
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   name,
		Short: "approval write (1.0: dry-run only; --yes is disabled)",
		Long: name + ` plans an approval write but does NOT execute it.

hr-cli 1.0 ships approval queries (+tasks/+task/+instances) and dry-run for
approve/reject/transfer. The --yes path is intentionally disabled because the
native state-machine entrypoint (approve, reject, transfer node permissions,
logs, callbacks) has not been verified end to end. A future release will
enable --yes once the native chain is confirmed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/approval/write",
				map[string]any{
					"action": name, "task_id": taskID, "comment": comment,
					"reason": reason, "to_badge": toBadge, "dry_run": dryRun, "yes": yes,
				}, yes)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	cmd.Flags().IntVar(&taskID, "task-id", 0, "task id")
	cmd.Flags().StringVar(&comment, "comment", "", "comment")
	cmd.Flags().StringVar(&reason, "reason", "", "reason")
	cmd.Flags().StringVar(&toBadge, "to-badge", "", "target badge")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "inspect approval write plan without writing")
	cmd.Flags().BoolVar(&yes, "yes", false, "(disabled in 1.0) confirm approval write")
	_ = cmd.MarkFlagRequired("task-id")
	return cmd
}
