package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/capability/doctor"
)

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "check database and runtime health",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := doctor.Run()
			if err != nil {
				return err
			}
			return emit(cmd, data)
		},
	}
}
