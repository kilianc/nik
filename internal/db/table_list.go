package db

import (
	"strings"
	"sync"
)

var (
	tableListOnce sync.Once
	tableListStr  string
)

func TableList() string {
	tableListOnce.Do(func() {
		tableListStr = parseTableList(schema)
	})

	return tableListStr
}

func parseTableList(ddl string) string {
	lines := strings.Split(ddl, "\n")

	var b strings.Builder
	var comment string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "-- ") {
			comment = strings.TrimPrefix(trimmed, "-- ")
			continue
		}

		if strings.HasPrefix(trimmed, "CREATE TABLE IF NOT EXISTS ") {
			name := strings.TrimPrefix(trimmed, "CREATE TABLE IF NOT EXISTS ")
			name = strings.TrimSuffix(name, " (")

			if b.Len() > 0 {
				b.WriteByte('\n')
			}

			b.WriteString("- ")
			b.WriteString(name)
			if comment != "" {
				b.WriteString(" -- ")
				b.WriteString(comment)
			}

			comment = ""
			continue
		}

		comment = ""
	}

	return b.String()
}
