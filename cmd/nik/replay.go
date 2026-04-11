package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/whatsapp"
)

func runReplay(args []string) {
	flagSet := flag.NewFlagSet("replay", flag.ExitOnError)
	home := flagSet.String("home", "", "workspace directory")
	flagSet.Parse(args)

	file := flagSet.Arg(0)
	if file == "" {
		fmt.Fprintln(os.Stderr, "usage: nik replay [--home dir] <file>")
		os.Exit(1)
	}

	h, err := resolveHome(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	messagingSvc := messaging.NewService(cfg, conn, contactsSvc)

	wc, err := whatsapp.NewClient(cfg.WappSessionDBPath(), cfg.MediaPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create whatsapp client: %v\n", err)
		os.Exit(1)
	}
	defer wc.Close()

	wa := whatsapp.NewAdapter(wc)
	messagingSvc.RegisterPlatform(wa)
	err = wa.Start(ctx, messagingSvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: start whatsapp adapter: %v\n", err)
		os.Exit(1)
	}

	err = wc.ReplayHistorySync(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: replay: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("replay finished")
}
