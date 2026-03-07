// migrate-dreams exports dream passes and soul versions from the DB
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
	home := flag.String("home", "workspace", "home directory for output")
	flag.Parse()

	conn, err := sql.Open("sqlite3", *dbPath+"?_foreign_keys=1&_journal_mode=WAL")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	dreamsDir := filepath.Join(*home, "dreams")
	soulDir := filepath.Join(*home, "soul")

	n, err := migrateDreams(conn, dreamsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate dreams: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d dream file(s)\n", n)

	n, err = migrateSouls(conn, soulDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate souls: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d soul file(s)\n", n)
}

func migrateDreams(conn *sql.DB, outDir string) (int, error) {
	err := os.MkdirAll(outDir, 0o755)
	if err != nil {
		return 0, fmt.Errorf("create dreams dir: %w", err)
	}

	rows, err := conn.Query(`
		SELECT date, pass, content
		FROM dream
		WHERE content != ''
		ORDER BY date, pass
	`)
	if err != nil {
		return 0, fmt.Errorf("query dreams: %w", err)
	}
	defer rows.Close()

	passNames := map[int]string{
		1: "Drift",
		2: "Weave",
		3: "Depths",
		4: "Crystallize",
		5: "Wake",
	}

	files := map[string][]string{}
	for rows.Next() {
		var date, content string
		var pass int

		err = rows.Scan(&date, &pass, &content)
		if err != nil {
			return 0, fmt.Errorf("scan dream: %w", err)
		}

		name := passNames[pass]
		if name == "" {
			name = fmt.Sprintf("Pass %d", pass)
		}

		header := fmt.Sprintf("## Pass %d — %s", pass, name)
		if pass == 5 {
			header = "## Wake"
		}

		files[date] = append(files[date], header+"\n\n"+content)
	}

	if err = rows.Err(); err != nil {
		return 0, err
	}

	for date, sections := range files {
		path := filepath.Join(outDir, date+".md")
		err = os.WriteFile(path, []byte(strings.Join(sections, "\n\n")+"\n"), 0o644)
		if err != nil {
			return 0, fmt.Errorf("write %s: %w", path, err)
		}
	}

	return len(files), nil
}

func migrateSouls(conn *sql.DB, outDir string) (int, error) {
	err := os.MkdirAll(outDir, 0o755)
	if err != nil {
		return 0, fmt.Errorf("create soul dir: %w", err)
	}

	rows, err := conn.Query(`
		SELECT version, content, dream_date
		FROM soul
		ORDER BY version ASC
	`)
	if err != nil {
		return 0, fmt.Errorf("query souls: %w", err)
	}
	defer rows.Close()

	var latestContent string
	count := 0

	for rows.Next() {
		var version int
		var content, dreamDate string

		err = rows.Scan(&version, &content, &dreamDate)
		if err != nil {
			return count, fmt.Errorf("scan soul: %w", err)
		}

		path := filepath.Join(outDir, dreamDate+".md")
		err = os.WriteFile(path, []byte(content+"\n"), 0o644)
		if err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}

		latestContent = content
		count++
	}

	if err = rows.Err(); err != nil {
		return count, err
	}

	if latestContent != "" {
		path := filepath.Join(outDir, "latest.md")
		err = os.WriteFile(path, []byte(latestContent+"\n"), 0o644)
		if err != nil {
			return count, fmt.Errorf("write latest.md: %w", err)
		}
	}

	return count, nil
}
