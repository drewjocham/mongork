package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/drewjocham/mongork/internal/jsonutil"
	"github.com/drewjocham/mongork/internal/schema"
	"github.com/drewjocham/mongork/internal/schema/diff"
	"github.com/spf13/cobra"
)

var (
	ErrUnsupportedOutputFormat = errors.New("unsupported output format")
)

func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Schema utilities",
	}

	cmd.AddCommand(newSchemaIndexesCmd(), newSchemaDiffCmd())
	return cmd
}

func newSchemaIndexesCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:         "indexes",
		Short:       "List expected indexes registered in code",
		Annotations: map[string]string{annotationOffline: "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch strings.ToLower(output) {
			case "json":
				return renderIndexesJSON(cmd.OutOrStdout())
			case "table", "":
				renderIndexesTable(cmd.OutOrStdout())
				return nil
			default:
				return fmt.Errorf("%w: %s", ErrUnsupportedOutputFormat, output)
			}
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table or json")
	return cmd
}

func newSchemaDiffCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare registered schema/index specs against live MongoDB",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := getServices(cmd.Context())
			if err != nil || s.MongoClient == nil {
				return fmt.Errorf("mongo client unavailable")
			}

			live, err := diff.InspectLive(cmd.Context(), s.MongoClient.Database(s.Config.Mongo.Database))
			if err != nil {
				return err
			}
			target := diff.FromRegistry()
			if len(live.Indexes) == 0 && len(live.Validators) == 0 &&
				len(target.Indexes) == 0 && len(target.Validators) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to compare: no collections and no registered schema metadata.")
				return nil
			}
			if len(target.Indexes) == 0 && len(target.Validators) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No registered schema metadata to compare.")
				return nil
			}
			diffs := diff.Compare(live, target)

			switch strings.ToLower(output) {
			case "json":
				return renderDiffJSON(cmd.OutOrStdout(), diffs)
			case "table", "":
				renderDiffTable(cmd.OutOrStdout(), diffs)
				return nil
			default:
				return fmt.Errorf("%w: %s", ErrUnsupportedOutputFormat, output)
			}
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table or json")
	return cmd
}

func renderIndexesJSON(w io.Writer) error {
	encoder := jsonutil.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(schema.Indexes())
}

func renderIndexesTable(w io.Writer) {
	indexes := schema.Indexes()
	if len(indexes) == 0 {
		fmt.Fprintln(w, "No index specifications registered.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "COLLECTION\tINDEX\tKEYS\tUNIQUE\tSPARSE\tPARTIAL FILTER")
	fmt.Fprintln(tw, "----------\t-----\t----\t------\t------\t--------------")

	for _, spec := range indexes {
		unique := "no"
		if spec.Unique {
			unique = "yes"
		}
		sparse := "no"
		if spec.Sparse {
			sparse = "yes"
		}
		partial := spec.PartialFilterString()
		if partial == "" {
			partial = "-"
		}

		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			spec.Collection,
			spec.Name,
			spec.KeyString(),
			unique,
			sparse,
			partial,
		)
	}

	tw.Flush()
}

func renderDiffJSON(w io.Writer, diffs []diff.Diff) error {
	encoder := jsonutil.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(diffs)
}

func renderDiffTable(w io.Writer, diffs []diff.Diff) {
	if len(diffs) == 0 {
		fmt.Fprintln(w, "No schema drift detected.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "COMPONENT\tACTION\tTARGET\tCURRENT\tPROPOSED\tRISK")
	fmt.Fprintln(tw, "---------\t------\t------\t-------\t--------\t----")

	for _, d := range diffs {
		action := d.Action
		if d.Risk == "HIGH" || d.Risk == "CRITICAL" {
			action = "!! " + action
		}
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			d.Component,
			action,
			d.Target,
			d.Current,
			d.Proposed,
			d.Risk,
		)
	}

	tw.Flush()
}
