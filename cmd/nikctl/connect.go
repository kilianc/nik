package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/home"
	"github.com/kciuffolo/nik/internal/nikapi"
)

// runConnect binds this install to a nik account. The token goes to the
// running daemon, which probes the gateway, stores what it gets back, writes
// gateway.url, and — if it was waiting for exactly this — carries on booting.
//
// It used to write config.yaml and the secret store itself, from a process
// that was not the one that would use them. That is why the installer had to
// connect *before* installing the service: a daemon started without a gateway
// died on arrival. Now the daemon comes up first and this is what completes
// it, so a token that arrives an hour later works exactly as well.
//
//	nikctl connect [--home dir] [--url wss://...] <token>
//	echo -n TOKEN | nikctl connect
func runConnect(args []string) {
	flagSet := flag.NewFlagSet("connect", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	url := flagSet.String("url", "", "gateway websocket URL (default: config, else production)")
	parseFlags(flagSet, args)

	token := strings.TrimSpace(strings.Join(flagSet.Args(), ""))
	if token == "" {
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
			var b [4096]byte
			n, _ := os.Stdin.Read(b[:])
			token = strings.TrimSpace(string(b[:n]))
		}
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "usage: nikctl connect [--home dir] [--url wss://...] <token>")
		os.Exit(1)
	}

	h, err := home.Resolve(*homeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Connecting reaches the gateway over the network, so this is generous
	// next to the client's usual timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fmt.Println("connecting...")

	err = connectWaitingForDaemon(ctx, nikapi.New(h), *url, token)

	switch {
	case errors.Is(err, nikapi.ErrNoDaemon):
		fmt.Fprintf(os.Stderr, "error: no nik running at %s\n\n", h)
		fmt.Fprintln(os.Stderr, "  install it:  nikctl install --home "+h)
		fmt.Fprintln(os.Stderr, "  or start it: nikd --home "+h)
		os.Exit(1)

	case errors.Is(err, nikapi.ErrAuthRejected):
		fmt.Fprintln(os.Stderr, "error: the gateway rejected that token — it may have expired (they last 15 minutes); make a new nik on your dashboard")
		os.Exit(1)

	case err != nil:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("connected — this nik is linked to your account")
}

// daemonWait is how long connect will wait for a daemon that is starting.
//
// The installer runs `nikctl install` and then this, and a service manager
// takes a moment to get the process up — so the common case for "no daemon"
// is "not yet" rather than "not at all". Bounded low enough that someone who
// really has no daemon gets told so while they are still watching.
const daemonWait = 15 * time.Second

func connectWaitingForDaemon(ctx context.Context, client *nikapi.Client, url, token string) error {
	deadline := time.Now().Add(daemonWait)
	announced := false

	for {
		err := client.Connect(ctx, url, token)
		if !errors.Is(err, nikapi.ErrNoDaemon) {
			return err
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return err
		}

		if !announced {
			announced = true
			fmt.Println("waiting for nik to start...")
		}

		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return err
		}
	}
}
