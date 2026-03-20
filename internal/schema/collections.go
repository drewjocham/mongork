package schema

import (
	"errors"
	"sort"
	"sync"
)

var (
	ErrCollectionNameRequired = errors.New("collection name is required")
)

var (
	collectionsMu sync.RWMutex
	collections   = make(map[string]struct{})
)

func RegisterCollection(name string) error {
	if name == "" {
		return ErrCollectionNameRequired
	}

	collectionsMu.Lock()
	defer collectionsMu.Unlock()
	collections[name] = struct{}{}
	return nil
}

func MustRegisterCollections(names ...string) {
	for _, name := range names {
		if err := RegisterCollection(name); err != nil {
			panic(err)
		}
	}
}

func Collections() []string {
	collectionsMu.RLock()
	defer collectionsMu.RUnlock()

	out := make([]string, 0, len(collections))
	for name := range collections {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
