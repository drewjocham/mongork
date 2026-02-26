package diff

import (
	"bytes"
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func formatBsonD(doc bson.D) string {
	if len(doc) == 0 {
		return ""
	}
	parts := make([]string, 0, len(doc))
	for _, elem := range doc {
		parts = append(parts, fmt.Sprintf("%s:%v", elem.Key, elem.Value))
	}
	return join(parts, ", ")
}

func ttlString(ttl *int32) string {
	if ttl == nil || *ttl <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", *ttl)
}

func validatorSummary(spec ValidatorSpec) string {
	if spec.Schema == nil {
		return "none"
	}
	return fmt.Sprintf("level=%s schema=%s", spec.Level, canonicalJSON(spec.Schema))
}

func canonicalJSON(doc bson.M) string {
	if doc == nil {
		return ""
	}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, key := range keys {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(fmt.Sprintf("%s:%v", key, doc[key]))
	}
	buf.WriteString("}")
	return buf.String()
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	var buf bytes.Buffer
	for i, part := range parts {
		if i > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(part)
	}
	return buf.String()
}
