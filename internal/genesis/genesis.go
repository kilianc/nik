package genesis

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type seed struct {
	name        string
	interactive bool
	body        string
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
	{name: "first_contact", interactive: true, body: loadSeed("01-first_contact")},
	{name: "reach", body: loadSeed("02-reach")},
	{name: "demo_time", body: loadSeed("03-demo_time")},
	{name: "contact_card", interactive: true, body: loadSeed("04-contact_card")},
	{name: "read_the_manual", interactive: true, body: loadSeed("05-read_the_manual")},
}

func IsInteractive(name string) bool {
	for _, s := range seeds {
		if s.name == name {
			return s.interactive
		}
	}
	return false
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
			return
		}

		upsertSeed(ctx, conn, seeds[i+1])
	}
}
