package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/drewjocham/mongork/internal/jsonutil"
	"github.com/drewjocham/mongork/internal/migration"
	"github.com/drewjocham/mongork/internal/observability"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var ErrFailedToMarshalJSON = errors.New("failed to marshal json")

type HealthReport = observability.HealthReport

func NewDBCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "db", Short: "Database utilities"}
	cmd.AddCommand(
		newDBHealthCmd(),
		newDBCollectionsCmd(),
		newDBIndexesCmd(),
		newDBStatsCmd(),
		newDBCurrentOpsCmd(),
		newDBUsersCmd(),
		newDBResourceCmd(),
	)
	return cmd
}

func newDBHealthCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Show database health and metrics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			report, err := buildReport(cmd.Context(), s.MongoClient, s.Config.Mongo.Database)
			if err != nil {
				return err
			}

			if strings.ToLower(output) == "json" {
				enc := jsonutil.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return fmt.Errorf("%w: %w", ErrFailedToMarshalJSON, err)
				}
				return nil
			}

			RenderHealthTable(cmd.OutOrStdout(), report)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table, json)")
	return cmd
}

func buildReport(ctx context.Context, client *mongo.Client, dbName string) (HealthReport, error) {
	return observability.BuildHealthReport(ctx, client, dbName)
}

func RenderHealthTable(w io.Writer, r HealthReport) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "\n\033[1m--- MONGO HEALTH: %s ---\033[0m\n", strings.ToUpper(r.Database))
	fmt.Fprintln(tw, "\033[1mMETRIC\tVALUE\033[0m")

	color := "\033[34m" // Blue
	if strings.EqualFold(r.Role, "PRIMARY") {
		color = "\033[32m" // Green
	}

	fields := [][]string{
		{"Role", fmt.Sprintf("%s%s\033[0m", color, r.Role)},
		{"Connections", r.Connections},
		{"Oplog Window", r.OplogWindow},
		{"Oplog Size", r.OplogSize},
	}

	for _, f := range fields {
		fmt.Fprintf(tw, "%s\t%s\n", f[0], f[1])
	}

	for node, lag := range r.Lag {
		fmt.Fprintf(tw, "Lag (%s)\t%s\n", node, lag)
	}
	tw.Flush()

	if len(r.Warnings) > 0 {
		fmt.Fprintln(w, "\n\033[33m\033[1m⚠️  WARNINGS\033[0m")
		for _, warn := range r.Warnings {
			fmt.Fprintf(w, "  \033[33m!\033[0m %s\n", warn)
		}
	}
}

func RenderMigrationTable(w io.Writer, status []migration.MigrationStatus) {
	if len(status) == 0 {
		fmt.Fprintln(w, "No migrations found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "\033[1mSTATE\tVERSION\tAPPLIED AT\tDESCRIPTION\033[0m")

	for _, s := range status {
		state := " [ ] PENDING"
		appliedAt := "-"

		if s.Applied {
			state = " \033[32m[✓] APPLIED\033[0m"
			if s.AppliedAt != nil {
				appliedAt = s.AppliedAt.Format("2006-01-02 15:04")
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", state, s.Version, appliedAt, s.Description)
	}
	tw.Flush()
}
