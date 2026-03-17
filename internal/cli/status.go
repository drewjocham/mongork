package cli

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/drewjocham/mongork/internal/migration"
	"github.com/spf13/cobra"
)

var (
	ErrFailedToGetStatus = errors.New("failed to get status")
	ErrUnsupportedOutput = errors.New("unsupported output format")
)

func newStatusCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			engine, err := getEngine(cmd.Context())
			if err != nil {
				return err
			}

			status, err := engine.GetStatus(cmd.Context())
			if err != nil {
				return fmt.Errorf("%w: %w", ErrFailedToGetStatus, err)
			}

			out := cmd.OutOrStdout()
			return renderWithOutput(
				out,
				format,
				ErrUnsupportedOutput,
				func(w io.Writer) error { return renderTable(w, status) },
				func(w io.Writer) error { return renderJSON(w, status) },
			)
		},
	}

	cmd.Flags().StringVarP(&format, "output", "o", "table", "Output format (table, json)")
	return cmd
}

func renderJSON(w io.Writer, status []migration.MigrationStatus) error {
	return encodePrettyJSON(w, status)
}

func renderTable(w io.Writer, status []migration.MigrationStatus) error {
	if len(status) == 0 {
		fmt.Fprintln(w, "No migrations found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)

	const (
		iconPending = " [ ] PENDING"
		iconApplied = " \033[32m[✓] APPLIED\033[0m"
	)

	fmt.Fprintln(tw, "\033[1mSTATE\tVERSION\tAPPLIED AT\tDESCRIPTION\033[0m")

	for _, s := range status {
		state := iconPending
		appliedAt := "-"

		if s.Applied {
			state = iconApplied
			if s.AppliedAt != nil {
				appliedAt = s.AppliedAt.Format("2006-01-02 15:04")
			}
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", state, s.Version, appliedAt, s.Description)
	}

	return tw.Flush()
}
