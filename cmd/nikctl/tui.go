package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/home"
	"github.com/kciuffolo/nik/internal/nikapi"
	"github.com/kciuffolo/nik/internal/tui"
)

func runTUI(args []string) {
	flagSet := flag.NewFlagSet("nik", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	forceSetup := flagSet.Bool("force-setup", false, "run setup even if config exists")
	showSystem := flagSet.Bool("show-system", false, "show system messages in chat")
	parseFlags(flagSet, args)

	h, err := home.Resolve(*homeFlag)
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

	// The chat half of the TUI is a client now: it opens no database, ensures
	// no rows, and constructs no messaging service of its own. It used to do
	// all three, which made it a second writer on a file nikd owns — in a
	// process that gets closed by closing a terminal window.
	client := nikapi.New(h)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := tui.Options{
		ShowSystem: *showSystem,
		Home:       h,
	}

	if !setup {
		state, err := client.Onboarding(ctx)

		switch {
		case errors.Is(err, nikapi.ErrNoDaemon):
			fmt.Fprintf(os.Stderr, "no nik running at %s\n\n", h)
			fmt.Fprintln(os.Stderr, "  start it:      nikd --home "+h)
			fmt.Fprintln(os.Stderr, "  or install it: nikctl install --home "+h)
			os.Exit(1)

		case err != nil:
			// nikd is there but not ready — no gateway yet, or still opening
			// the database. Open the chat anyway: this is exactly when
			// somebody needs to see what nik says about itself, and refusing
			// to start would leave them with nothing but a log file.
			opts.InputGate = nil

		default:
			opts.BornAt = state.BornAt
			opts.InputGate = onboardingInputGate(client, state)
		}
	}

	err = tui.Run(cfg, client, tui.NewAPISender(client), setup, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// onboardingInputGate gates the chat input while genesis is in progress.
// Returns nil (= TUI default: always editable) when genesis is already done,
// the common case for returning users. The seed-to-input mapping lives here
// because it is onboarding UX, not something nikd should have an opinion
// about — a browser console will want to draw this differently.
func onboardingInputGate(client *nikapi.Client, initial nikapi.OnboardingState) tui.InputGate {
	if initial.Completed {
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

	return func(messages []nikapi.Message, activity []string) tui.InputState {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		state, err := client.Onboarding(ctx)
		if err != nil {
			// Can't tell where genesis is, so don't take the input away —
			// a locked box a person cannot explain is worse than an early one.
			return tui.InputState{}
		}
		if state.Completed {
			return tui.InputState{}
		}

		if state.Seed == "" || !interactiveSeeds[state.Seed] || len(activity) > 0 {
			return tui.InputState{Locked: true, Placeholder: "waiting for nik to finish..."}
		}

		return tui.InputState{Placeholder: placeholders[state.Seed]}
	}
}
