// migrate recreates a table in nik.db using the canonical schema.sql DDL.
// Use when a CHECK constraint or table-level constraint has drifted.
//
//	go run ./tools/migrate -db workspace/nik.db -table message
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/kciuffolo/nik/internal/db"
)

const schemaPath = "internal/db/schema.sql"

func main() {
	dbPath := flag.String("db", "nik.db", "path to the live database")
	table := flag.String("table", "", "table to recreate (required)")
	flag.Parse()

	if *table == "" {
		fmt.Fprintln(os.Stderr, "-table is required")
		os.Exit(1)
	}

	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", schemaPath, err)
		os.Exit(1)
	}

	desiredDDL, err := extractDDL(string(schemaSQL), *table)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract DDL for %s: %v\n", *table, err)
		os.Exit(1)
	}

	backupPath, err := backupDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("backed up to %s\n", backupPath)

	live, err := sql.Open("sqlite3_nik", fmt.Sprintf("file:%s?_foreign_keys=0", *dbPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer live.Close()

	err = recreateTable(live, *table, desiredDDL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate %s: %v\n", *table, err)
		fmt.Fprintf(os.Stderr, "restore from backup: %s\n", backupPath)
		os.Exit(1)
	}

	fmt.Printf("migration complete for table %s\n", *table)
}

// extractDDL applies schemaSQL to an in-memory DB and reads back the DDL
// for the requested table from sqlite_master.
func extractDDL(schemaSQL, table string) (string, error) {
	mem, err := sql.Open("sqlite3_nik", ":memory:")
	if err != nil {
		return "", err
	}
	defer mem.Close()

	_, err = mem.Exec(schemaSQL)
	if err != nil {
		return "", fmt.Errorf("apply schema: %w", err)
	}

	var ddl string
	err = mem.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&ddl)
	if err != nil {
		return "", fmt.Errorf("table %s not found in schema: %w", table, err)
	}

	return ddl, nil
}

func recreateTable(db *sql.DB, table, desiredDDL string) error {
	tmp := table + "_new"

	tmpDDL := strings.Replace(desiredDDL, "CREATE TABLE "+table, "CREATE TABLE "+tmp, 1)
	if tmpDDL == desiredDDL {
		tmpDDL = strings.Replace(desiredDDL, `CREATE TABLE "`+table+`"`, "CREATE TABLE "+tmp, 1)
	}

	var beforeCount int
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&beforeCount)
	if err != nil {
		return fmt.Errorf("count before: %w", err)
	}
	fmt.Printf("%s: %d rows before\n", table, beforeCount)

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(tmpDDL)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}

	cols, err := columnNames(tx, table)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}
	colList := strings.Join(cols, ", ")

	_, err = tx.Exec(fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", tmp, colList, colList, table))
	if err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	_, err = tx.Exec(fmt.Sprintf("DROP TABLE %s", table))
	if err != nil {
		return fmt.Errorf("drop old: %w", err)
	}

	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", tmp, table))
	if err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	rows, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("fk check: %w", err)
	}
	defer rows.Close()

	var violations []string
	for rows.Next() {
		var tbl, rowid, parent, fkid string
		rows.Scan(&tbl, &rowid, &parent, &fkid)
		violations = append(violations, fmt.Sprintf("  %s rowid=%s -> %s (fk %s)", tbl, rowid, parent, fkid))
	}
	if len(violations) > 0 {
		return fmt.Errorf("foreign key violations:\n%s", strings.Join(violations, "\n"))
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	var afterCount int
	err = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&afterCount)
	if err != nil {
		return fmt.Errorf("count after: %w", err)
	}
	fmt.Printf("%s: %d rows after\n", table, afterCount)

	if beforeCount != afterCount {
		return fmt.Errorf("row count mismatch: %d before, %d after", beforeCount, afterCount)
	}

	return nil
}

func columnNames(tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull bool
		var dflt sql.NullString
		var pk int
		err = rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk)
		if err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}

	return cols, rows.Err()
}

func backupDB(dbPath string) (string, error) {
	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return "", err
	}

	ts := time.Now().Format("2006-01-02T15-04-05")
	dst := filepath.Join(dir, ts+".db")

	src, err := os.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	if err != nil {
		os.Remove(dst)
		return "", err
	}

	return dst, nil
}
