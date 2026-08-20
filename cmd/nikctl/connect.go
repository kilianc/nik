package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/gateway"
	"github.com/kciuffolo/nik/internal/home"
	"github.com/kciuffolo/nik/internal/secrets"
)

// runConnect binds this install to a nik account: it stores the gateway
// token, writes gateway.url into config.yaml (creating a minimal config when
// none exists yet), and proves the pair by connecting before returning. The
// installer runs it with the token from the dashboard's one-liner, so by the
// time the service starts the daemon already has a working gateway — the
// one thing boot refuses to run without.
//
//	nik connect [--home dir] [--url wss://...] <token>
//	echo -n TOKEN | nik connect
func runConnect(args []string) {
	flagSet := flag.NewFlagSet("connect", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	url := flagSet.String("url", "", "gateway websocket URL (default: config, else production)")
	flagSet.Parse(args)

	token := strings.TrimSpace(strings.Join(flagSet.Args(), ""))
	if token == "" {
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
			var b [4096]byte
			n, _ := os.Stdin.Read(b[:])
			token = strings.TrimSpace(string(b[:n]))
		}
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "usage: nik connect [--home dir] [--url wss://...] <token>")
		os.Exit(1)
	}

	h, err := home.Resolve(*homeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(h, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: create %s: %v\n", h, err)
		os.Exit(1)
	}

	// Existing config keeps everything it has; a fresh home gets defaults
	// with only the gateway filled in — setup writes the rest later.
	cfg, err := config.Read(h)
	if err != nil {
		cfg = config.Default(h)
	}
	switch {
	case *url != "":
		cfg.Gateway.URL = *url
	case cfg.Gateway.URL == "":
		cfg.Gateway.URL = gateway.DefaultURL
	}

	// The probe rotates: what gets stored is the token the gateway handed
	// back, never the one that was typed — the install code in your shell
	// history is dead the moment this returns.
	fmt.Printf("connecting to %s...\n", cfg.Gateway.URL)
	err = gateway.ProbeWithStore(context.Background(), cfg.Gateway.URL, token, secrets.New(h))
	switch {
	case errors.Is(err, gateway.ErrAuthRejected):
		fmt.Fprintln(os.Stderr, "error: the gateway rejected that token — it may have expired (they last 15 minutes); make a new agent on your dashboard")
		os.Exit(1)
	case err != nil:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	cfg.Normalize()
	if err := cfg.Save(cfg.ConfigPath()); err != nil {
		fmt.Fprintf(os.Stderr, "error: write config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("connected — this nik is linked to your account")
}
