//go:build include_examples

package cli

import (
	"github.com/drewjocham/mongork/examples/examplemigrations"
	"github.com/drewjocham/mongork/internal/migration"
)

func registerExampleMigrations() error {
	migration.MustRegister(
		&examplemigrations.AddUserIndexesMigration{},
		&examplemigrations.TransformUserDataMigration{},
		&examplemigrations.CreateAuditCollectionMigration{},
	)
	return nil
}
