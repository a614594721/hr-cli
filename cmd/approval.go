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

	root.AddCommand(tasks, task, approvalBlockedCommand("+approve"), approvalBlockedCommand("+reject"), approvalBlockedCommand("+transfer"))
	return root
}

func approvalBlockedCommand(name string) *cobra.Command {
	cmd := &cobra.Command{
		Use: name,
		RunE: func(cmd *cobra.Command, args []string) error {
			return approval.WriteNotVerified(name)
		},
	}
	cmd.Flags().String("task-id", "", "task id")
	cmd.Flags().String("comment", "", "comment")
	cmd.Flags().String("reason", "", "reason")
	cmd.Flags().String("to-badge", "", "target badge")
	return cmd
}
