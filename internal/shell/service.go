package shell

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

const staleThreshold = 30 * time.Minute

type Service struct {
	conn *sql.DB
	home string
}

func NewService(conn *sql.DB, home string) *Service {
	return &Service{conn: conn, home: home}
}

func (s *Service) CheckSessions(ctx context.Context) {
	if s.conn == nil {
		return
	}

	ids, err := db.ShellOutputAliveIDs(ctx, s.conn)
	if err != nil {
		slog.Warn("check sessions", "pkg", "shell", "error", err)
		return
	}

	now := time.Now().UTC()

	for _, sid := range ids {
		alive := isAlive(sid)

		if !alive {
			out, _ := capturePane(sid)
			code, _ := getExitCode(sid)
			killSession(sid)

			err = db.ShellOutputUpsert(ctx, s.conn, db.ShellOutputUpsertParams{
				SessionID: sid,
				Output:    out,
				ExitCode:  &code,
				Alive:     false,
			})
			if err != nil {
				slog.Warn("reap dead session", "pkg", "shell", "session_id", sid, "error", err)
			}

			slog.Info("reaped dead session", "pkg", "shell", "session_id", sid, "exit_code", code)
			continue
		}

		meta, metaErr := loadMeta(sid)
		if metaErr != nil {
			slog.Warn("load meta for stale check", "pkg", "shell", "session_id", sid, "error", metaErr)
			continue
		}

		if !meta.StartedAt.IsZero() && now.Sub(meta.StartedAt) > staleThreshold {
			out, _ := capturePane(sid)
			killSession(sid)

			err = db.ShellOutputUpsert(ctx, s.conn, db.ShellOutputUpsertParams{
				SessionID: sid,
				Output:    out,
				Alive:     false,
			})
			if err != nil {
				slog.Warn("kill stale session", "pkg", "shell", "session_id", sid, "error", err)
			}

			slog.Info("killed stale session", "pkg", "shell", "session_id", sid,
				"started_at", meta.StartedAt, "age", now.Sub(meta.StartedAt))
		}
	}
}
