package genesis

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func seedDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	ctx := context.Background()

	err = db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	err = db.OwnerContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure owner contact: %v", err)
	}

	err = db.LocalConversationEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure local conversation: %v", err)
	}

	_, err = conn.Exec(
		"UPDATE conversation SET last_read_at = '9999-01-01T00:00:00.000Z' WHERE id = ?",
		db.LocalConversationID,
	)
	if err != nil {
		t.Fatalf("set last_read_at: %v", err)
	}

	return conn
}

func countSeeds(t *testing.T, conn *sql.DB) int {
	t.Helper()
	var count int
	err := conn.QueryRow(
		"SELECT count(*) FROM message WHERE platform = 'system' AND external_message_id LIKE 'genesis:%'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count seeds: %v", err)
	}
	return count
}

func seedExists(conn *sql.DB, name string) bool {
	var n int
	err := conn.QueryRow(
		"SELECT count(*) FROM message WHERE platform = 'system' AND external_message_id = ?",
		"genesis:"+name,
	).Scan(&n)
	return err == nil && n > 0
}

func TestReflex(t *testing.T) {
	ctx := context.Background()

	t.Run("does not fire during active activation", func(t *testing.T) {
		conn := seedDB(t)
		defer conn.Close()

		_, err := conn.Exec(
			`UPDATE conversation SET activity = '["thinking"]' WHERE id = ?`,
			db.LocalConversationID,
		)
		if err != nil {
			t.Fatalf("set activity: %v", err)
		}

		Reflex(conn)(ctx)

		if countSeeds(t, conn) > 0 {
			t.Fatal("reflex should not fire during active activation")
		}
	})

	t.Run("birth fires on first activation", func(t *testing.T) {
		conn := seedDB(t)
		defer conn.Close()

		Reflex(conn)(ctx)

		if !seedExists(conn, "birth") {
			t.Fatal("expected birth seed to fire on first activation")
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		conn := seedDB(t)
		defer conn.Close()

		reflex := Reflex(conn)
		reflex(ctx)
		reflex(ctx)
		reflex(ctx)

		var count int
		err := conn.QueryRow(
			"SELECT count(*) FROM message WHERE external_message_id = 'genesis:birth'",
		).Scan(&count)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 row after 3 runs, got %d", count)
		}
	})

	t.Run("progression", func(t *testing.T) {
		conn := seedDB(t)
		defer conn.Close()

		reflex := Reflex(conn)

		reflex(ctx)
		if !seedExists(conn, seeds[0].name) {
			t.Fatalf("expected %s to fire on first tick", seeds[0].name)
		}
		if countSeeds(t, conn) != 1 {
			t.Fatalf("expected 1 seed after first tick, got %d", countSeeds(t, conn))
		}

		for i := 0; i < len(seeds)-1; i++ {
			current := seeds[i].name
			next := seeds[i+1].name

			reflex(ctx)
			if seedExists(conn, next) {
				t.Fatalf("%s should not fire before genesis_completed_step=%s", next, current)
			}

			err := db.SettingSet(ctx, conn, "genesis_completed_step", current)
			if err != nil {
				t.Fatalf("set setting: %v", err)
			}

			reflex(ctx)
			if !seedExists(conn, next) {
				t.Fatalf("%s should fire after genesis_completed_step=%s", next, current)
			}
			if countSeeds(t, conn) != i+2 {
				t.Fatalf("expected %d seeds after firing %s, got %d", i+2, next, countSeeds(t, conn))
			}
		}
	})
}

func TestStartedAt(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	if got, err := StartedAt(ctx, conn); err != nil || !got.IsZero() {
		t.Errorf("StartedAt unset: got (%v, %v), want (zero, nil)", got, err)
	}

	started := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	if err := db.SettingSet(ctx, conn, db.GenesisStartedAtKey, db.ISO8601MS(started)); err != nil {
		t.Fatalf("set started: %v", err)
	}
	if got, err := StartedAt(ctx, conn); err != nil || !got.Equal(started) {
		t.Errorf("StartedAt: got (%v, %v), want (%v, nil)", got, err, started)
	}
}

func TestIsCompleted(t *testing.T) {
	completionLatch.Store(false)
	t.Cleanup(func() { completionLatch.Store(false) })

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	if IsCompleted(ctx, conn) {
		t.Error("IsCompleted unset: got true, want false")
	}

	if err := db.SettingSet(ctx, conn, db.GenesisCompletedAtKey, db.ISO8601MS(time.Now())); err != nil {
		t.Fatalf("set completed: %v", err)
	}
	if !IsCompleted(ctx, conn) {
		t.Error("IsCompleted after stamp: got false, want true")
	}

	// Latched: even after blowing away the setting, IsCompleted stays true
	// without touching the DB — genesis completion is a monotonic invariant.
	if _, err := conn.Exec("DELETE FROM setting WHERE key = ?", db.GenesisCompletedAtKey); err != nil {
		t.Fatalf("delete completed setting: %v", err)
	}
	if !IsCompleted(ctx, conn) {
		t.Error("IsCompleted after latch: got false, want true (should be cached)")
	}
}

func TestCurrentSeed(t *testing.T) {
	mk := func(platform, ext string) db.Message {
		return db.Message{Platform: platform, ExternalMessageID: ext}
	}

	cases := []struct {
		name string
		msgs []db.Message
		want string
	}{
		{"empty", nil, ""},
		{"no genesis messages", []db.Message{mk("local", "abc"), mk("system", "other:1")}, ""},
		{"single seed", []db.Message{mk("system", "genesis:birth")}, "birth"},
		{
			"most recent wins",
			[]db.Message{
				mk("system", "genesis:birth"),
				mk("local", "user reply"),
				mk("system", "genesis:first_contact"),
				mk("local", "later"),
			},
			"first_contact",
		},
		{
			"ignores non-system genesis-looking messages",
			[]db.Message{mk("local", "genesis:fake"), mk("system", "genesis:birth")},
			"birth",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CurrentSeed(tc.msgs); got != tc.want {
				t.Errorf("CurrentSeed = %q, want %q", got, tc.want)
			}
		})
	}
}
