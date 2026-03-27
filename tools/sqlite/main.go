package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	_ "github.com/kciuffolo/nik/internal/db"
)

func main() {
	dbPath := flag.String("db", ":memory:", "path to database")
	flag.Parse()

	dsn := *dbPath + "?_busy_timeout=5000&_journal_mode=WAL"
	conn, err := sql.Open("sqlite3_nik", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	conn.SetMaxOpenConns(1)
	defer conn.Close()

	if err := conn.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "ping: %v\n", err)
		os.Exit(1)
	}

	if flag.NArg() > 0 {
		execFile(conn, flag.Arg(0))
		return
	}

	if !isTerminal(os.Stdin) {
		execReader(conn, os.Stdin)
		return
	}

	interactive(conn)
}

func interactive(conn *sql.DB) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024*1024), 64*1024*1024)

	var buf strings.Builder
	prompt := "nik> "

	for {
		fmt.Fprint(os.Stderr, prompt)
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()

		if buf.Len() == 0 && strings.HasPrefix(line, ".") {
			handleDot(conn, line)
			continue
		}

		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)

		stmt := buf.String()
		if !statementComplete(stmt) {
			prompt = "...> "
			continue
		}

		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			execOne(conn, stmt)
		}
		buf.Reset()
		prompt = "nik> "
	}

	if buf.Len() > 0 {
		execOne(conn, buf.String())
	}
}

func execReader(conn *sql.DB, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024*1024), 64*1024*1024)

	var buf strings.Builder
	lineNo := 0
	errors := 0

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)

		stmt := buf.String()
		if !statementComplete(stmt) {
			continue
		}

		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			err := execSilent(conn, stmt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "line %d: %v\n", lineNo, err)
				errors++
			}
		}
		buf.Reset()
	}

	if buf.Len() > 0 {
		stmt := strings.TrimSpace(buf.String())
		if stmt != "" {
			err := execSilent(conn, stmt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "line %d: %v\n", lineNo, err)
				errors++
			}
		}
	}

	if errors > 0 {
		fmt.Fprintf(os.Stderr, "%d errors\n", errors)
		os.Exit(1)
	}
}

func execFile(conn *sql.DB, path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()

	execReader(conn, f)
}

func execSilent(conn *sql.DB, stmt string) error {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "PRAGMA") {
		return queryPrint(conn, stmt)
	}

	_, err := conn.Exec(stmt)
	return err
}

func execOne(conn *sql.DB, stmt string) {
	err := execSilent(conn, stmt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

func queryPrint(conn *sql.DB, stmt string) error {
	rows, err := conn.Query(stmt)
	if err != nil {
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	if len(cols) > 0 {
		fmt.Println(strings.Join(cols, "|"))
	}

	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}

	for rows.Next() {
		err := rows.Scan(ptrs...)
		if err != nil {
			return err
		}

		parts := make([]string, len(cols))
		for i, v := range values {
			if v == nil {
				parts[i] = ""
			} else {
				parts[i] = fmt.Sprintf("%v", v)
			}
		}
		fmt.Println(strings.Join(parts, "|"))
	}

	return rows.Err()
}

func handleDot(conn *sql.DB, line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case ".quit", ".exit":
		os.Exit(0)
	case ".tables":
		queryPrint(conn, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	case ".schema":
		if len(parts) > 1 {
			queryPrint(conn, fmt.Sprintf("SELECT sql FROM sqlite_master WHERE name='%s'", parts[1]))
		} else {
			queryPrint(conn, "SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY type DESC, name")
		}
	case ".dump":
		fmt.Fprintln(os.Stderr, ".dump not implemented — use sqlite3 CLI for dump, this tool for import")
	case ".help":
		fmt.Fprintln(os.Stderr, ".tables    list tables")
		fmt.Fprintln(os.Stderr, ".schema    show schema")
		fmt.Fprintln(os.Stderr, ".quit      exit")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s (try .help)\n", parts[0])
	}
}

func statementComplete(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return true
	}

	return strings.HasSuffix(trimmed, ";")
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
