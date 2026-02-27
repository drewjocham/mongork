package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/drewjocham/mongork/internal/schema/diff"
)

func renderSchemaDiff(ctx context.Context, out io.Writer) error {
	s, err := getServices(ctx)
	if err != nil || s.MongoClient == nil {
		return fmt.Errorf("mongo client unavailable")
	}

	live, err := diff.InspectLive(ctx, s.MongoClient.Database(s.Config.Mongo.Database))
	if err != nil {
		return err
	}
	target := diff.FromRegistry()
	diffs := diff.Compare(live, target)
	renderDiffTable(out, diffs)
	return nil
}
