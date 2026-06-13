package cmd

import (
	"github.com/spf13/cobra"

	"hr-cli/internal/db"
)

func newDBCommand() *cobra.Command {
	root := &cobra.Command{Use: "db", Short: "restricted raw diagnostics"}
	var sqlText string
	var args []string
	var limit int
	query := &cobra.Command{
		Use: "query",
		RunE: func(cmd *cobra.Command, cobraArgs []string) error {
			items, err := db.RawQuery(sqlText, args, limit)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"items": items})
		},
	}
	query.Flags().StringVar(&sqlText, "sql", "", "read-only SQL")
	query.Flags().StringArrayVar(&args, "arg", nil, "SQL argument")
	query.Flags().IntVar(&limit, "limit", 100, "maximum rows")
	_ = query.MarkFlagRequired("sql")
	root.AddCommand(query)
	return root
}
