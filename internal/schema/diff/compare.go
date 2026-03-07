package diff

import (
	"fmt"
	"sort"
)

func Compare(live, target SchemaSpec) []Diff {
	var diffs []Diff

	for _, coll := range unionKeys(live.Indexes, target.Indexes) {
		liveIndexes := live.Indexes[coll]
		targetIndexes := target.Indexes[coll]

		for _, name := range unionKeys(liveIndexes, targetIndexes) {
			liveIdx, liveOK := liveIndexes[name]
			targetIdx, targetOK := targetIndexes[name]

			switch {
			case !liveOK && targetOK:
				diffs = append(diffs, Diff{
					Component: "index",
					Action:    "AddIndex",
					Target:    fmt.Sprintf("%s.%s", coll, name),
					Current:   "missing",
					Proposed:  describeIndex(targetIdx),
					Risk:      indexAddRisk(targetIdx),
				})
			case liveOK && !targetOK:
				diffs = append(diffs, Diff{
					Component: "index",
					Action:    "DropIndex",
					Target:    fmt.Sprintf("%s.%s", coll, name),
					Current:   describeIndex(liveIdx),
					Proposed:  "removed",
					Risk:      "CRITICAL",
				})
			case liveOK && targetOK:
				if indexSignature(liveIdx) != indexSignature(targetIdx) {
					diffs = append(diffs, Diff{
						Component: "index",
						Action:    "UpdateIndex",
						Target:    fmt.Sprintf("%s.%s", coll, name),
						Current:   describeIndex(liveIdx),
						Proposed:  describeIndex(targetIdx),
						Risk:      "MEDIUM",
					})
				}
			}
		}
	}

	for _, coll := range unionKeys(live.Validators, target.Validators) {
		liveVal, liveOK := live.Validators[coll]
		targetVal, targetOK := target.Validators[coll]

		switch {
		case !liveOK && targetOK:
			diffs = append(diffs, Diff{
				Component: "validator",
				Action:    "AddValidator",
				Target:    coll,
				Current:   "missing",
				Proposed:  validatorSummary(targetVal),
				Risk:      "MEDIUM",
			})
		case liveOK && !targetOK:
			diffs = append(diffs, Diff{
				Component: "validator",
				Action:    "DropValidator",
				Target:    coll,
				Current:   validatorSummary(liveVal),
				Proposed:  "removed",
				Risk:      "HIGH",
			})
		case liveOK && targetOK:
			if validatorSignature(liveVal) != validatorSignature(targetVal) {
				diffs = append(diffs, Diff{
					Component: "validator",
					Action:    "UpdateValidator",
					Target:    coll,
					Current:   validatorSummary(liveVal),
					Proposed:  validatorSummary(targetVal),
					Risk:      "MEDIUM",
				})
			}
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Component != diffs[j].Component {
			return diffs[i].Component < diffs[j].Component
		}
		return diffs[i].Target < diffs[j].Target
	})

	return diffs
}

func unionKeys[T any](a, b map[string]T) []string {
	keys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func indexSignature(idx IndexSpec) string {
	return fmt.Sprintf("k=%s|u=%t|s=%t|ttl=%s|p=%s",
		formatBsonD(idx.Keys),
		idx.Unique,
		idx.Sparse,
		ttlString(idx.ExpireAfterSeconds),
		formatBsonD(idx.PartialFilter),
	)
}

func validatorSignature(v ValidatorSpec) string {
	return fmt.Sprintf("%s|%s", v.Level, canonicalJSON(v.Schema))
}

func indexAddRisk(idx IndexSpec) string {
	if idx.Unique {
		return "MEDIUM"
	}
	return "LOW"
}
