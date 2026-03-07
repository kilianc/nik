// migrate-briefings exports briefing and briefing_topic rows from the DB
// to the file system. one-shot tool; safe to re-run (overwrites existing files).
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "workspace/nik.db", "path to nik.db")
	outDir := flag.String("out", "workspace/briefings", "output directory for briefing files")
	flag.Parse()

	conn, err := sql.Open("sqlite3", *dbPath+"?_foreign_keys=1&_journal_mode=WAL")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	err = os.MkdirAll(*outDir, 0o755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}

	n, err := migrateBriefings(conn, *outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate briefings: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d briefing file(s)\n", n)

	n, err = migrateTopics(conn, *outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate topics: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote topics.md (%d topic(s))\n", n)
}

func migrateBriefings(conn *sql.DB, outDir string) (int, error) {
	rows, err := conn.Query("SELECT date, content FROM briefing WHERE content != ''")
	if err != nil {
		return 0, fmt.Errorf("query briefings: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var date, content string

		err = rows.Scan(&date, &content)
		if err != nil {
			return count, fmt.Errorf("scan briefing: %w", err)
		}

		path := filepath.Join(outDir, date+".md")
		err = os.WriteFile(path, []byte(content+"\n"), 0o644)
		if err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}

		count++
	}

	return count, rows.Err()
}

func migrateTopics(conn *sql.DB, outDir string) (int, error) {
	rows, err := conn.Query("SELECT query, reason, contact_id FROM briefing_topic ORDER BY created_at")
	if err != nil {
		return 0, fmt.Errorf("query topics: %w", err)
	}
	defer rows.Close()

	var lines []string
	lines = append(lines, "# Briefing Topics", "")

	count := 0
	for rows.Next() {
		var query, reason string
		var contactID sql.NullString

		err = rows.Scan(&query, &reason, &contactID)
		if err != nil {
			return count, fmt.Errorf("scan topic: %w", err)
		}

		entry := fmt.Sprintf("- **%s**", query)
		if reason != "" {
			entry += " — " + reason
		}
		if contactID.Valid && contactID.String != "" {
			entry += fmt.Sprintf(" (contact: %s)", contactID.String)
		}

		lines = append(lines, entry)
		count++
	}

	if err := rows.Err(); err != nil {
		return count, err
	}

	lines = append(lines, "")
	path := filepath.Join(outDir, "topics.md")
	err = os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	if err != nil {
		return count, fmt.Errorf("write topics.md: %w", err)
	}

	return count, nil
}
