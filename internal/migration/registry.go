package migration

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
)

var (
	registryMu sync.RWMutex
	registered = make(map[string]Migration)

	versionPattern = regexp.MustCompile(`^\d{8}(?:_\d{3,6})?(?:_[a-z0-9_]+)?$`)

	ErrMigrationNil        = errors.New("migration must not be nil")
	ErrInvalidVersionFmt   = errors.New("invalid version format")
	ErrMigrationRegistered = errors.New("migration already registered")
)

func Register(m Migration) error {
	if m == nil {
		return ErrMigrationNil
	}

	version := m.Version()
	if !isValidVersionFormat(version) {
		return fmt.Errorf("%w: %s (expected YYYYMMDD[_HHMMSS][_slug])", ErrInvalidVersionFmt, version)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registered[version]; exists {
		return fmt.Errorf("%w: %s", ErrMigrationRegistered, version)
	}

	registered[version] = m
	return nil
}

func MustRegister(ms ...Migration) {
	for _, m := range ms {
		if err := Register(m); err != nil {
			panic(err)
		}
	}
}

func RegisteredMigrations() map[string]Migration {
	registryMu.RLock()
	defer registryMu.RUnlock()

	copy := make(map[string]Migration, len(registered))
	for k, v := range registered {
		copy[k] = v
	}
	return copy
}

type MigrationFilter func(version string, m Migration) bool

func GetMigrations(filters ...MigrationFilter) map[string]Migration {
	registryMu.RLock()
	defer registryMu.RUnlock()

	results := make(map[string]Migration)
	for v, m := range registered {
		keep := true
		for _, filter := range filters {
			if !filter(v, m) {
				keep = false
				break
			}
		}
		if keep {
			results[v] = m
		}
	}
	return results
}

func isValidVersionFormat(version string) bool {
	return versionPattern.MatchString(version)
}
