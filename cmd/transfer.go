package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/gateway"
)

func newTransferCommand() *cobra.Command {
	root := &cobra.Command{Use: "transfer", Short: "employee transfer preview commands"}
	var badge, effectDate, reason string
	var dept, job int
	prev := &cobra.Command{
		Use: "+preview",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/transfer/preview",
				map[string]any{"badge": badge, "dept": dept, "job": job, "effect_date": effectDate, "reason": reason}, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	prev.Flags().StringVar(&badge, "badge", "", "employee badge")
	prev.Flags().IntVar(&dept, "dept", 0, "new department id")
	prev.Flags().IntVar(&job, "job", 0, "new job id")
	prev.Flags().StringVar(&effectDate, "effect-date", "", "effect date")
	prev.Flags().StringVar(&reason, "reason", "", "change reason")
	_ = prev.MarkFlagRequired("badge")

	var yes, dryRun bool
	apply := &cobra.Command{
		Use:  "+apply <preview-id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "POST", "/api/hr-cli/v1/transfer/apply",
				map[string]any{"preview_id": args[0], "yes": yes, "dry_run": dryRun}, yes)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	apply.Flags().BoolVar(&yes, "yes", false, "confirm apply")
	apply.Flags().BoolVar(&dryRun, "dry-run", false, "run apply preflight without writing")

	root.AddCommand(prev, apply)
	return root
}
