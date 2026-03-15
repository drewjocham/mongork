package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/drewjocham/mongork/internal/jsonutil"
	"github.com/drewjocham/mongork/internal/observability"
	"github.com/spf13/cobra"
)

func newDBCollectionsCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "collections",
		Short: "List database collections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			rows, err := observability.ListCollections(cmd.Context(), s.MongoClient.Database(s.Config.Mongo.Database))
			if err != nil {
				return err
			}
			return renderJSONOrTable(cmd, output, rows, func() {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "NAME\tTYPE")
				for _, r := range rows {
					fmt.Fprintf(w, "%s\t%s\n", r.Name, r.Type)
				}
				_ = w.Flush()
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table, json)")
	return cmd
}

func newDBIndexesCmd() *cobra.Command {
	var output string
	var collection string
	cmd := &cobra.Command{
		Use:   "indexes",
		Short: "List indexes (all collections or one collection)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			rows, err := observability.ListIndexes(cmd.Context(), s.MongoClient.Database(s.Config.Mongo.Database), collection)
			if err != nil {
				return err
			}
			return renderJSONOrTable(cmd, output, rows, func() {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "COLLECTION\tINDEX\tKEYS\tUNIQUE")
				for _, r := range rows {
					fmt.Fprintf(w, "%s\t%s\t%s\t%t\n", r.Collection, r.Name, r.Keys, r.Unique)
				}
				_ = w.Flush()
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table, json)")
	cmd.Flags().StringVar(&collection, "collection", "", "Collection name filter")
	return cmd
}

func newDBStatsCmd() *cobra.Command {
	var output string
	var collection string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show collection stats",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			rows, err := observability.CollectionStatistics(
				cmd.Context(),
				s.MongoClient.Database(s.Config.Mongo.Database),
				collection,
			)
			if err != nil {
				return err
			}
			return renderJSONOrTable(cmd, output, rows, func() {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "COLLECTION\tCOUNT\tSIZE(B)\tSTORAGE(B)\tINDEX(B)\tAVG_OBJ(B)")
				for _, r := range rows {
					fmt.Fprintf(
						w,
						"%s\t%d\t%d\t%d\t%d\t%.2f\n",
						r.Collection,
						r.Count,
						r.SizeBytes,
						r.StorageBytes,
						r.IndexBytes,
						r.AvgObjectBytes,
					)
				}
				_ = w.Flush()
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table, json)")
	cmd.Flags().StringVar(&collection, "collection", "", "Collection name filter")
	return cmd
}

func newDBCurrentOpsCmd() *cobra.Command {
	var output string
	var limit int
	cmd := &cobra.Command{
		Use:   "current-ops",
		Short: "Show currently running Mongo operations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			rows, err := observability.CurrentOperations(cmd.Context(), s.MongoClient, limit)
			if err != nil {
				return err
			}
			return renderJSONOrTable(cmd, output, rows, func() {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "OPID\tOP\tNAMESPACE\tCLIENT\tSECS\tDESC")
				for _, r := range rows {
					fmt.Fprintf(
						w,
						"%s\t%s\t%s\t%s\t%d\t%s\n",
						r.OpID,
						r.Operation,
						r.Namespace,
						r.Client,
						r.RunningSecs,
						r.Description,
					)
				}
				_ = w.Flush()
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table, json)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum running operations to show")
	return cmd
}

func newDBUsersCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List Mongo users and roles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			rows, err := observability.Users(cmd.Context(), s.MongoClient)
			if err != nil {
				return err
			}
			return renderJSONOrTable(cmd, output, rows, func() {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "USER\tDB\tROLES")
				for _, r := range rows {
					fmt.Fprintf(w, "%s\t%s\t%s\n", r.User, r.DB, strings.Join(r.Roles, ", "))
				}
				_ = w.Flush()
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table, json)")
	return cmd
}

func newDBResourceCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Show resource consumption summary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			summary, err := observability.BuildResourceSummary(cmd.Context(), s.MongoClient)
			if err != nil {
				return err
			}
			return renderJSONOrTable(cmd, output, summary, func() {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "METRIC\tVALUE")
				fmt.Fprintf(w, "Connections\t%d / %d\n", summary.ConnectionsCurrent, summary.ConnectionsAvailable)
				fmt.Fprintf(w, "Resident MB\t%.2f\n", summary.ResidentMemoryMB)
				fmt.Fprintf(w, "Virtual MB\t%.2f\n", summary.VirtualMemoryMB)
				for k, v := range summary.Opcounters {
					fmt.Fprintf(w, "opcounter.%s\t%.0f\n", k, v)
				}
				_ = w.Flush()
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table, json)")
	return cmd
}

func renderJSONOrTable(cmd *cobra.Command, output string, value any, renderTable func()) error {
	if strings.EqualFold(output, "json") {
		enc := jsonutil.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(value)
	}
	renderTable()
	return nil
}
