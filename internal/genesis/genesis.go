package genesis

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type seed struct {
	name string
	body string
}

//go:embed *.md
var seedBodiesFS embed.FS

func loadSeed(filename string) string {
	b, err := seedBodiesFS.ReadFile(filename + ".md")
	if err != nil {
		panic(fmt.Sprintf("read genesis seed %s: %v", filename, err))
	}
	return strings.TrimSpace(string(b))
}

var seeds = []seed{
	{name: "birth", body: loadSeed("00-birth")},
	{name: "first_contact", body: loadSeed("01-first_contact")},
	{name: "reach", body: loadSeed("02-reach")},
	{name: "demo_time", body: loadSeed("03-demo_time")},
	{name: "contact_card", body: loadSeed("04-contact_card")},
	{name: "read_the_manual", body: loadSeed("05-read_the_manual")},
}

// StartedAt returns when genesis started, or zero time if not yet stamped.
func StartedAt(ctx context.Context, conn *sql.DB) (time.Time, error) {
	if conn == nil {
		return time.Time{}, nil
	}
	v, err := db.SettingGet(ctx, conn, db.GenesisStartedAtKey)
	if err != nil {
		return time.Time{}, fmt.Errorf("get %s: %w", db.GenesisStartedAtKey, err)
	}
	if v == nil {
		return time.Time{}, nil
	}
	return db.ParseTimeValue(v.Value)
}

// completionLatch caches the terminal "genesis is done" invariant. Genesis
// completion is monotonic — once stamped, it stays stamped — so once we've
// observed true we never need to ask the DB again.
var completionLatch atomic.Bool

// IsCompleted reports whether genesis has finished. Cheap after the first
// observation of completion: subsequent calls return true without touching
// the DB.
func IsCompleted(ctx context.Context, conn *sql.DB) bool {
	if completionLatch.Load() {
		return true
	}
	if conn == nil {
		return false
	}
	v, err := db.SettingGet(ctx, conn, db.GenesisCompletedAtKey)
	if err != nil || v == nil {
		return false
	}
	completionLatch.Store(true)
	return true
}

// CurrentSeed returns the name of the most recent genesis seed in messages,
// or "" if no seed has been posted yet (or if all seeds have scrolled out of
// the slice).
func CurrentSeed(msgs []db.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Platform != "system" || !strings.HasPrefix(m.ExternalMessageID, "genesis:") {
			continue
		}
		return strings.TrimPrefix(m.ExternalMessageID, "genesis:")
	}
	return ""
}

func upsertSeed(ctx context.Context, conn *sql.DB, s seed) {
	extID := "genesis:" + s.name
	_, err := conn.ExecContext(ctx, queries.GenesisNudgeEnsure,
		id.V7(),
		db.LocalConversationID,
		db.SystemContactID,
		extID,
		s.body,
	)
	if err != nil {
		slog.Warn("upsert genesis seed", "pkg", "genesis", "seed", s.name, "error", err)
	}
}

func Reflex(conn *sql.DB) func(ctx context.Context) {
	return func(ctx context.Context) {
		conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: db.LocalConversationID})
		if err != nil || len(conv.Activity) > 0 {
			return
		}

		completed, err := db.SettingGet(ctx, conn, "genesis_completed_step")
		if err != nil {
			slog.Warn("genesis reflex: get completed step", "pkg", "genesis", "error", err)
			return
		}

		i := -1
		if completed != nil {
			for idx, s := range seeds {
				if s.name == completed.Value {
					i = idx
					break
				}
			}
		}

		if i == len(seeds)-1 {
			s, err := db.SettingGet(ctx, conn, "genesis_completed_at")
			if err == nil && s == nil {
				err := db.SettingSet(ctx, conn, "genesis_completed_at", db.ISO8601MS(time.Now()))
				if err != nil {
					slog.Warn("genesis reflex: stamp genesis_completed_at", "pkg", "genesis", "error", err)
				}
			}
			return
		}

		upsertSeed(ctx, conn, seeds[i+1])
	}
}
