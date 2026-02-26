package mcp

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func formatIndexKeys(keys interface{}) string {
	var keyParts []string
	if doc, ok := keys.(bson.D); ok {
		for _, elem := range doc {
			keyParts = append(keyParts, fmt.Sprintf("%s: %v", elem.Key, elem.Value))
		}
	}
	if m, ok := keys.(bson.M); ok {
		for k, v := range m {
			keyParts = append(keyParts, fmt.Sprintf("%s: %v", k, v))
		}
	}

	if len(keyParts) == 0 {
		return "none"
	}
	return strings.Join(keyParts, ", ")
}
