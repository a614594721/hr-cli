package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/capability/approval"
)

func newApprovalCommand() *cobra.Command {
	root := &cobra.Command{Use: "approval", Short: "approval query commands"}
	var assignee string
	var limit int
	tasks := &cobra.Command{
		Use: "+tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := approval.Tasks(assignee, limit)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": items})
		},
	}
	tasks.Flags().StringVar(&assignee, "assignee", "", "task assignee")
	tasks.Flags().IntVar(&limit, "limit", 50, "maximum rows")

	var taskID int
	task := &cobra.Command{
		Use: "+task",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := approval.Task(taskID)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	task.Flags().IntVar(&taskID, "task-id", 0, "task id")
	_ = task.MarkFlagRequired("task-id")

	var employee, status string
	var instanceLimit int
	instances := &cobra.Command{
		Use: "+instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := approval.Instances(employee, status, instanceLimit)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": items})
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
		Use: name,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := approval.WritePlan(name, taskID, comment, reason, toBadge, dryRun, yes)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	cmd.Flags().IntVar(&taskID, "task-id", 0, "task id")
	cmd.Flags().StringVar(&comment, "comment", "", "comment")
	cmd.Flags().StringVar(&reason, "reason", "", "reason")
	cmd.Flags().StringVar(&toBadge, "to-badge", "", "target badge")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "inspect approval write plan without writing")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm approval write")
	_ = cmd.MarkFlagRequired("task-id")
	return cmd
}
