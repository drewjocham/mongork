package migration

import (
	"fmt"
	"strings"
)

func FormatStatusTable(status []MigrationStatus) string {
	var b strings.Builder
	b.WriteString("### Migration Status\n\n")
	b.WriteString("| Version | Status | Applied At | Description |\n")
	b.WriteString("| :--- | :--- | :--- | :--- |\n")

	for _, st := range status {
		applied := "⏳ Pending"
		appliedAt := "N/A"

		if st.Applied {
			applied = "✅ Applied"
			if st.AppliedAt != nil {
				appliedAt = st.AppliedAt.Format("2006-01-02 15:04")
			}
		}

		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			st.Version, applied, appliedAt, st.Description))
	}
	return b.String()
}
