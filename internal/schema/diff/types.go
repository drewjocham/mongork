package diff

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

type IndexSpec struct {
	Collection         string
	Name               string
	Keys               bson.D
	Unique             bool
	Sparse             bool
	PartialFilter      bson.D
	ExpireAfterSeconds *int32
}

type ValidatorSpec struct {
	Collection string
	Schema     bson.M
	Level      string
}

type SchemaSpec struct {
	Collections map[string]struct{}
	Indexes     map[string]map[string]IndexSpec
	Validators  map[string]ValidatorSpec
}

type Diff struct {
	Component string `json:"component"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Current   string `json:"current"`
	Proposed  string `json:"proposed"`
	Risk      string `json:"risk"`
}

func NewSchemaSpec() SchemaSpec {
	return SchemaSpec{
		Collections: make(map[string]struct{}),
		Indexes:     make(map[string]map[string]IndexSpec),
		Validators:  make(map[string]ValidatorSpec),
	}
}
