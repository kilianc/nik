package shell

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

const staleThreshold = 30 * time.Minute

const containerNikBin = "/usr/local/bin/nik"

type Service struct {
	cfg       *config.Config
	conn      *sql.DB
	container string
	nikBin    string
}

func NewService(cfg *config.Config, conn *sql.DB, nikBin string) *Service {
	return &Service{cfg: cfg, conn: conn, nikBin: nikBin}
}

func (s *Service) nikBinDir() string {
	if s.container != "" {
		return filepath.Dir(containerNikBin)
	}
	return filepath.Dir(s.nikBin)
}

func (s *Service) nikBinLinux() string {
	if runtime.GOOS == "linux" {
		return s.nikBin
	}
	return filepath.Join(filepath.Dir(s.nikBin),
		fmt.Sprintf("nik-linux-%s", runtime.GOARCH))
}

func (s *Service) containerName() string { return s.cfg.Shell.DockerImage }

func (s *Service) dockerImage() string { return s.cfg.Shell.DockerImage }

func (s *Service) workdir() string {
	return s.cfg.ShellHome()
}

func (s *Service) CheckSessions(ctx context.Context) {
	if s.conn == nil {
		return
	}

	ids, err := db.ShellSessionAliveIDs(ctx, s.conn)
	if err != nil {
		slog.Warn("check sessions", "pkg", "shell", "error", err)
		return
	}

	now := time.Now().UTC()

	for _, sid := range ids {
		alive := s.isAlive(sid)

		if !alive {
			out, _ := s.capturePane(sid)
			code, _ := s.getExitCode(sid)
			s.killSession(sid)

			err = db.ShellSessionUpdate(ctx, s.conn, db.ShellSessionUpdateParams{
				ID:       sid,
				Output:   out,
				ExitCode: &code,
				Alive:    false,
			})
			if err != nil {
				slog.Warn("reap dead session", "pkg", "shell", "session_id", sid, "error", err)
			}

			slog.Info("reaped dead session", "pkg", "shell", "session_id", sid, "exit_code", code)
			continue
		}

		meta, metaErr := s.loadMeta(sid)
		if metaErr != nil {
			slog.Warn("load meta for stale check", "pkg", "shell", "session_id", sid, "error", metaErr)
			continue
		}

		if !meta.StartedAt.IsZero() && now.Sub(meta.StartedAt) > staleThreshold {
			out, _ := s.capturePane(sid)
			s.killSession(sid)

			err = db.ShellSessionUpdate(ctx, s.conn, db.ShellSessionUpdateParams{
				ID:     sid,
				Output: out,
				Alive:  false,
			})
			if err != nil {
				slog.Warn("kill stale session", "pkg", "shell", "session_id", sid, "error", err)
			}

			slog.Info("killed stale session", "pkg", "shell", "session_id", sid,
				"started_at", meta.StartedAt, "age", now.Sub(meta.StartedAt))
		}
	}
}
