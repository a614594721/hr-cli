package cmd

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"hr-cli/internal/build"
	"hr-cli/internal/errs"
	"hr-cli/internal/output"
)

var format string
var currentCommand string

func Execute() int {
	root := NewRoot()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	if err := root.Execute(); err != nil {
		var cliErr *errs.Error
		if !errors.As(err, &cliErr) {
			cliErr = errs.New("internal", "unhandled", err.Error(), 5)
		}
		output.Failure(cliErr, meta(root))
		return cliErr.ExitCode
	}
	return 0
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:              "hr",
		Short:            "HR capability CLI (thin client to hr-gateway)",
		Version:          build.Version,
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true,
	}
	root.PersistentFlags().StringVar(&format, "format", "json", "output format: json or table")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		currentCommand = commandName(cmd)
		if format != "json" && format != "table" {
			err := errs.Validation("invalid_format", "--format must be json or table")
			err.Param = "--format"
			return err
		}
		return nil
	}
	root.AddCommand(
		newDoctorCommand(),
		newConfigCommand(),
		newProfileCommand(),
		newAuthCommand(),
		newPermCommand(),
		newEmployeeCommand(),
		newAttendanceCommand(),
		newApprovalCommand(),
		newTransferCommand(),
		newProfileInfoCommand(),
	)
	return root
}

func emit(cmd *cobra.Command, data any) error {
	return output.Success(cmd.OutOrStdout(), data, meta(cmd), format)
}

func meta(cmd *cobra.Command) output.Meta {
	command := commandName(cmd)
	if command == "" {
		command = currentCommand
	}
	return output.Meta{
		"command": command,
		"version": build.Version,
		"format":  format,
	}
}

func commandName(cmd *cobra.Command) string {
	var parts []string
	for current := cmd; current != nil && current.Name() != "hr"; current = current.Parent() {
		parts = append([]string{current.Name()}, parts...)
	}
	return strings.Join(parts, ".")
}

