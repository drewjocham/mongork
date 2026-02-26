package diff

import (
	"github.com/drewjocham/mongork/internal/schema"
)

// FromRegistry builds the desired schema spec from in-code registries.
func FromRegistry() SchemaSpec {
	spec := NewSchemaSpec()

	for _, idx := range schema.Indexes() {
		if spec.Indexes[idx.Collection] == nil {
			spec.Indexes[idx.Collection] = make(map[string]IndexSpec)
		}
		spec.Indexes[idx.Collection][idx.Name] = IndexSpec{
			Collection:         idx.Collection,
			Name:               idx.Name,
			Keys:               idx.Keys,
			Unique:             idx.Unique,
			Sparse:             idx.Sparse,
			PartialFilter:      idx.PartialFilter,
			ExpireAfterSeconds: idx.ExpireAfterSeconds,
		}
	}

	for _, v := range schema.Validators() {
		spec.Validators[v.Collection] = ValidatorSpec{
			Collection: v.Collection,
			Schema:     v.Schema,
			Level:      v.Level,
		}
	}

	return spec
}
