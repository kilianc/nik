package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/genesis"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/tui"
)

func runTUI(args []string) {
	flagSet := flag.NewFlagSet("nik", flag.ExitOnError)
	home := flagSet.String("home", "", "workspace directory")
	forceSetup := flagSet.Bool("force-setup", false, "run setup even if config exists")
	showSystem := flagSet.Bool("show-system", false, "show system messages in chat")
	flagSet.Parse(args)

	h, err := resolveHome(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	cfg, err := config.Read(h)
	setup := *forceSetup
	if errors.Is(err, os.ErrNotExist) {
		cfg = config.Default(h)
		setup = true
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx := context.Background()

	err = db.NikContactEnsure(ctx, conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: ensure nik contact: %v\n", err)
		os.Exit(1)
	}

	err = db.OwnerContactEnsure(ctx, conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: ensure owner contact: %v\n", err)
		os.Exit(1)
	}

	err = db.LocalConversationEnsure(ctx, conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: ensure local conversation: %v\n", err)
		os.Exit(1)
	}

	contactsSvc := contacts.NewService(conn)
	messagingSvc := messaging.NewService(cfg, conn, contactsSvc)

	messagingSvc.RegisterPlatform(messaging.NewLocalAdapter(conn))

	bornAt, _ := genesis.StartedAt(ctx, conn)

	err = tui.Run(cfg, conn, tui.NewLocalSender(messagingSvc), setup, tui.Options{
		ShowSystem: *showSystem,
		BornAt:     bornAt,
		InputGate:  onboardingInputGate(ctx, conn),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// onboardingInputGate gates the chat input while genesis is in progress.
// Returns nil (= TUI default: always editable) when genesis is already done
// at launch — the common case for returning users. The seed→input mapping
// lives here because it's onboarding UX, not generic chat or pure genesis.
func onboardingInputGate(ctx context.Context, conn *sql.DB) tui.InputGate {
	if genesis.IsCompleted(ctx, conn) {
		return nil
	}

	interactiveSeeds := map[string]bool{
		"first_contact":   true,
		"contact_card":    true,
		"read_the_manual": true,
	}
	placeholders := map[string]string{
		"first_contact": "introduce yourself to nik",
	}

	return func(messages []db.Message, activity []string) tui.InputState {
		if genesis.IsCompleted(context.Background(), conn) {
			return tui.InputState{}
		}
		seed := genesis.CurrentSeed(messages)
		if seed == "" || !interactiveSeeds[seed] || len(activity) > 0 {
			return tui.InputState{Locked: true, Placeholder: "waiting for nik to finish..."}
		}
		return tui.InputState{Placeholder: placeholders[seed]}
	}
}
