package migration

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

type SchemaBuilder struct {
	data   bson.M
	isRoot bool
}

func Schema() *SchemaBuilder {
	return &SchemaBuilder{
		data:   bson.M{"bsonType": "object"},
		isRoot: true,
	}
}

func (s *SchemaBuilder) BsonType(t string) *SchemaBuilder {
	s.data["bsonType"] = t
	return s
}

func (s *SchemaBuilder) Required(fields ...string) *SchemaBuilder {
	if len(fields) > 0 {
		s.data["required"] = fields
	}
	return s
}

func (s *SchemaBuilder) Properties(props bson.M) *SchemaBuilder {
	s.data["properties"] = props
	return s
}

func (s *SchemaBuilder) Field(name string, props any) *SchemaBuilder {
	if name == "" {
		return s
	}
	pMap, ok := s.data["properties"].(bson.M)
	if !ok {
		pMap = bson.M{}
		s.data["properties"] = pMap
	}
	pMap[name] = props
	return s
}

func (s *SchemaBuilder) String() bson.M { return bson.M{"bsonType": "string"} }
func (s *SchemaBuilder) Int() bson.M    { return bson.M{"bsonType": "int"} }
func (s *SchemaBuilder) Long() bson.M   { return bson.M{"bsonType": "long"} }
func (s *SchemaBuilder) Bool() bson.M   { return bson.M{"bsonType": "bool"} }
func (s *SchemaBuilder) Date() bson.M   { return bson.M{"bsonType": "date"} }
func (s *SchemaBuilder) Array(items any) bson.M {
	return bson.M{"bsonType": "array", "items": items}
}
func (s *SchemaBuilder) Object(props bson.M) bson.M {
	return bson.M{"bsonType": "object", "properties": props}
}

func (s *SchemaBuilder) Build() bson.M {
	if s.isRoot {
		return bson.M{"$jsonSchema": s.data}
	}
	return s.data
}
