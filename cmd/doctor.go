package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/gateway"
)

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "ping hr-gateway /health",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := gateway.Call(cmd.Context(), "GET", "/api/hr-cli/v1/health", nil, false)
			if err != nil {
				return err
			}
			return emit(cmd, out)
		},
	}
	return cmd
}
