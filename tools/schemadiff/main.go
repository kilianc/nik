// schemadiff compares the live nik.db schema against the desired schema.sql
// and prints column-level diffs. read-only -- never modifies the database.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	_ "github.com/kciuffolo/nik/internal/db"
)

const schemaPath = "internal/db/schema.sql"

type column struct {
	name    string
	typ     string
	notNull bool
	dfltVal sql.NullString
	pk      int
}

func (c column) signature() string {
	var parts []string
	parts = append(parts, c.typ)
	if c.notNull {
		parts = append(parts, "NOT NULL")
	}
	if c.pk > 0 {
		parts = append(parts, "PRIMARY KEY")
	}
	if c.dfltVal.Valid {
		parts = append(parts, fmt.Sprintf("DEFAULT %s", c.dfltVal.String))
	}
	return strings.Join(parts, " ")
}

func main() {
	dbPath := flag.String("db", "nik.db", "path to the live database")
	flag.Parse()

	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", schemaPath, err)
		os.Exit(1)
	}

	live, err := sql.Open("sqlite3_nik", fmt.Sprintf("file:%s?mode=ro", *dbPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer live.Close()

	desired, err := sql.Open("sqlite3_nik", ":memory:")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open in-memory db: %v\n", err)
		os.Exit(1)
	}
	defer desired.Close()

	_, err = desired.Exec(stripVirtualTables(string(schemaSQL)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "apply desired schema: %v\n", err)
		os.Exit(1)
	}

	tables, err := listTables(desired)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list desired tables: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("schema diff: %s vs schema.sql\n", *dbPath)
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println()

	hasDiffs := false

	desiredSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		desiredSet[t] = true
	}

	for _, table := range tables {
		desiredCols, err := tableInfo(desired, table)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read desired table_info(%s): %v\n", table, err)
			continue
		}

		liveCols, err := tableInfo(live, table)
		if err != nil {
			fmt.Printf("%s: MISSING TABLE\n", table)
			hasDiffs = true
			continue
		}

		diffs := diffColumns(desiredCols, liveCols)
		if len(diffs) == 0 {
			fmt.Printf("%s: ok\n", table)
			continue
		}

		hasDiffs = true
		fmt.Printf("%s:\n", table)
		for _, d := range diffs {
			fmt.Printf("  %s\n", d)
		}
	}

	liveTables, err := listTables(live)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list live tables: %v\n", err)
		os.Exit(1)
	}

	for _, table := range liveTables {
		if desiredSet[table] {
			continue
		}

		hasDiffs = true
		fmt.Printf("%s: EXTRA TABLE (not in schema.sql)\n", table)
	}

	fmt.Println()
	if hasDiffs {
		fmt.Println("diffs found")
		os.Exit(1)
	}
	fmt.Println("schemas match")
}

func listTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}

	return tables, rows.Err()
}

func tableInfo(db *sql.DB, table string) (map[string]column, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]column)
	for rows.Next() {
		var cid int
		var c column
		err = rows.Scan(&cid, &c.name, &c.typ, &c.notNull, &c.dfltVal, &c.pk)
		if err != nil {
			return nil, err
		}
		cols[c.name] = c
	}

	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns (virtual table?)")
	}

	return cols, rows.Err()
}

// stripVirtualTables removes CREATE VIRTUAL TABLE statements so the schema
// can be applied to a plain SQLite connection without extensions.
func stripVirtualTables(schema string) string {
	var out []string
	for _, stmt := range strings.Split(schema, ";") {
		trimmed := strings.TrimSpace(stmt)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "CREATE VIRTUAL TABLE") {
			continue
		}
		out = append(out, stmt)
	}
	return strings.Join(out, ";")
}

func diffColumns(desired, live map[string]column) []string {
	var diffs []string

	// sort for stable output
	var desiredNames []string
	for name := range desired {
		desiredNames = append(desiredNames, name)
	}
	sort.Strings(desiredNames)

	for _, name := range desiredNames {
		dc := desired[name]
		lc, exists := live[name]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("+ %s %s", name, dc.signature()))
			continue
		}

		if dc.signature() != lc.signature() {
			diffs = append(diffs, fmt.Sprintf("~ %s: %s (desired) vs %s (live)", name, dc.signature(), lc.signature()))
		}
	}

	// extra columns in live that aren't in desired
	var liveNames []string
	for name := range live {
		liveNames = append(liveNames, name)
	}
	sort.Strings(liveNames)

	for _, name := range liveNames {
		_, exists := desired[name]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("- %s (extra in live)", name))
		}
	}

	return diffs
}
