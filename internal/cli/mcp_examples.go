//go:build include_examples

package cli

import (
	"github.com/drewjocham/mongork/examples/examplemigrations"
	addcolumn "github.com/drewjocham/mongork/examples/practical/add_column_expand_contract"
	databackfill "github.com/drewjocham/mongork/examples/practical/data_backfill"
	renamefield "github.com/drewjocham/mongork/examples/practical/rename_field"
	splitcollection "github.com/drewjocham/mongork/examples/practical/split_collection"
	"github.com/drewjocham/mongork/internal/migration"
)

type ExampleMigration struct {
	version     string
	description string
}

func (m *ExampleMigration) Version() string     { return m.version }
func (m *ExampleMigration) Description() string { return m.description }

func (m *ExampleMigration) Up(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("examples").InsertOne(ctx, map[string]string{"status": "initialized"})
	return err
}

func (m *ExampleMigration) Down(ctx context.Context, db *mongo.Database) error {
	return db.Collection("examples").Drop(ctx)
}

func registerExampleMigrations() error {
	migration.MustRegister(
		&examplemigrations.AddUserIndexesMigration{},
		&examplemigrations.TransformUserDataMigration{},
		&examplemigrations.CreateAuditCollectionMigration{},
	)
	for _, scenario := range [][]migration.Migration{
		addcolumn.Examples(),
		renamefield.Examples(),
		splitcollection.Examples(),
		databackfill.Examples(),
	} {
		migration.MustRegister(scenario...)
	}
	return nil
}
