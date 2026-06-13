package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/capability/transfer"
	"hr-cli/internal/preview"
)

func newTransferCommand() *cobra.Command {
	root := &cobra.Command{Use: "transfer", Short: "employee transfer preview commands"}
	var badge, effectDate, reason string
	var dept, job int
	prev := &cobra.Command{
		Use: "+preview",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := transfer.Preview(badge, dept, job, effectDate, reason)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	prev.Flags().StringVar(&badge, "badge", "", "employee badge")
	prev.Flags().IntVar(&dept, "dept", 0, "new department id")
	prev.Flags().IntVar(&job, "job", 0, "new job id")
	prev.Flags().StringVar(&effectDate, "effect-date", "", "effect date")
	prev.Flags().StringVar(&reason, "reason", "", "change reason")
	_ = prev.MarkFlagRequired("badge")

	var yes bool
	var dryRun bool
	apply := &cobra.Command{
		Use:  "+apply <preview-id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := transfer.Apply(args[0], yes, dryRun)
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	apply.Flags().BoolVar(&yes, "yes", false, "confirm apply")
	apply.Flags().BoolVar(&dryRun, "dry-run", false, "run apply preflight without writing")

	show := &cobra.Command{Use: "preview", Short: "preview helpers"}
	showShow := &cobra.Command{
		Use:  "show <preview-id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := preview.Load(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
	show.AddCommand(showShow)

	root.AddCommand(prev, apply, show)
	return root
}
