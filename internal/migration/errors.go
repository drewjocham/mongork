package migration

type ErrorMigration string

func (e ErrorMigration) Error() string {
	return string(e)
}

const (
	ErrInvalidMigrationVersion = ErrorMigration("invalid migration version")
	ErrFailedToGenerate        = ErrorMigration("failed to generate migration")
	ErrFailedToReadTemplate    = ErrorMigration("failed to read template")
	ErrFailedToConnect         = ErrorMigration("failed to connect to database")
	ErrFailedToPing            = ErrorMigration("failed to ping database")
	ErrFailedToUnlock          = ErrorMigration("failed to release lock")
)
