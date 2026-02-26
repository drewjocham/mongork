package schema

import (
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type ValidatorSpec struct {
	Collection  string
	Description string
	Schema      bson.M
	Level       string // off, moderate, strict
}

var validatorRegistry = struct {
	specs map[string]ValidatorSpec
}{
	specs: make(map[string]ValidatorSpec),
}

func RegisterValidator(spec ValidatorSpec) error {
	if spec.Collection == "" {
		return fmt.Errorf("validator collection is required")
	}
	if spec.Schema == nil {
		return fmt.Errorf("validator schema is required")
	}

	key := spec.Collection
	if _, exists := validatorRegistry.specs[key]; exists {
		return fmt.Errorf("validator for %s already registered", spec.Collection)
	}
	validatorRegistry.specs[key] = spec
	return nil
}

func MustRegisterValidator(specs ...ValidatorSpec) {
	for _, spec := range specs {
		if err := RegisterValidator(spec); err != nil {
			panic(err)
		}
	}
}

func Validators() []ValidatorSpec {
	out := make([]ValidatorSpec, 0, len(validatorRegistry.specs))
	for _, spec := range validatorRegistry.specs {
		out = append(out, spec)
	}
	return out
}
