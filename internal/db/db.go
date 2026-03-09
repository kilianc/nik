package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/mattn/go-sqlite3"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

//go:embed schema.sql
var schema string

func init() {
	sql.Register("sqlite3_nik", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.RegisterFunc("jaro_winkler_similarity", jaroWinklerSimilarity, true)
		},
	})
}

func Open(dbPath string, loc *time.Location) (*sql.DB, error) {
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := dbPath + "?_foreign_keys=1&_busy_timeout=5000&_journal_mode=WAL"
	if loc != nil {
		dsn += "&_loc=" + url.QueryEscape(loc.String())
	}

	db, err := sql.Open("sqlite3_nik", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return db, nil
}

func OpenInMemory() (*sql.DB, error) {
	db, err := Open(":memory:", nil)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	return db, nil
}
