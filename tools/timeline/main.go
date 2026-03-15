package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/task"
	"github.com/kciuffolo/nik/internal/timeline"
)

func main() {
	home := flag.String("home", "workspace", "nik home directory")
	convID := flag.String("conv", "", "conversation ID")
	n := flag.Int("n", 50, "max messages to load")
	flag.Parse()

	if *convID == "" {
		fmt.Fprintln(os.Stderr, "usage: timeline -conv <conversation_id> [-n 50]")
		os.Exit(1)
	}

	cfg, err := config.Load(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	cfg.MaxHistory = *n

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx := context.Background()

	tl := timeline.New(
		cfg,
		messaging.NewService(cfg, conn, contacts.NewService(conn)),
		task.NewService(conn),
		alarms.New(conn),
		skills.NewService(conn),
	)

	session, rendered, err := tl.Render(ctx, *convID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("```")
	fmt.Println("## Session")
	fmt.Println()
	fmt.Println(strings.Join(session, "\n"))
	fmt.Println()
	fmt.Println(strings.Join(rendered, "\n"))
	fmt.Println("```")
}
