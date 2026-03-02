package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/briefing"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/dream"
	"github.com/kciuffolo/nik/internal/journal"
	"github.com/kciuffolo/nik/internal/llm"
	niklog "github.com/kciuffolo/nik/internal/log"
	"github.com/kciuffolo/nik/internal/memory"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/search"
	"github.com/kciuffolo/nik/internal/shell"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/websearch"
	"github.com/kciuffolo/nik/internal/whatsapp"
)

const version = "0.0.1"

func main() {
	home := flag.String("home", "", "workspace directory (default: current directory)")
	wappLink := flag.Bool("force-wapp-link", false, "force WhatsApp QR pairing")
	replay := flag.String("wapp-replay-history", "", "replay recorded history sync from file")
	flag.Parse()

	cfg, err := config.Load(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logFile, err := os.OpenFile(cfg.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	logWriter := io.MultiWriter(os.Stderr, logFile)
	logger := slog.New(&niklog.TruncHandler{Inner: slog.NewTextHandler(logWriter, nil)})
	slog.SetDefault(logger)

	ascii := []string{
		"oooo   oooo ooooo oooo   oooo",
		" 8888o  88   888   888  o88",
		" 88 888o88   888   888888",
		" 88   8888   888   888  88o",
		"o88o    88  o888o o888o o888o",
		"",
		"Noetic Intelligence Kernel v" + version,
		"",
	}

	motd := strings.Join(ascii, "\n")
	fmt.Println()
	fmt.Println(motd)
	fmt.Println()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	conn, err := db.Open(cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	slog.Info("database ready", "path", cfg.DBPath())

	mediaPath := cfg.MediaPath()
	err = os.MkdirAll(mediaPath, 0o755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating media dir: %v\n", err)
		os.Exit(1)
	}

	whatsappClient, err := whatsapp.NewClient(cfg.WappSessionDBPath(), mediaPath, cfg.MediaDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer whatsappClient.Close()

	contactsSvc := contacts.NewService(conn)
	messagingSvc := messaging.NewService(cfg, conn, contactsSvc)
	whatsappAdapter := whatsapp.NewAdapter(whatsappClient)

	messagingSvc.RegisterPlatform(whatsappAdapter)
	whatsappAdapter.Start(ctx, messagingSvc)

	if *replay != "" {
		if err := whatsappClient.ReplayHistorySync(*replay); err != nil {
			fmt.Fprintf(os.Stderr, "replay error: %v\n", err)
			os.Exit(1)
		}
		slog.Info("replay finished, exiting")
		return
	}

	var llmOpts []llm.ClientOption
	if cfg.OpenAIKey != "" {
		llmOpts = append(llmOpts, llm.WithAPIKey(cfg.OpenAIKey))
	}
	if cfg.UseCodex {
		auth, err := codex.LoadOrLogin("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "codex auth error: %v\n", err)
			os.Exit(1)
		}
		llmOpts = append(llmOpts, llm.WithCodex(auth))
		slog.Info("codex auth ready", "account_id", auth.AccountID)
	}
	llmOpts = append(llmOpts, llm.WithReasoningEffort(&cfg.ReasoningEffort))
	llmClient := llm.NewClient(cfg.Model, llmOpts...)

	alarmSvc := alarms.New(conn)
	searchSvc := search.NewService(conn)
	memorySvc := memory.NewService(conn, llmClient)
	journalSvc := journal.NewService(conn, cfg)
	dreamSvc := dream.NewService(conn, cfg)
	briefingSvc := briefing.NewService(conn, cfg)
	b := brain.New(cfg, llmClient)

	b.SetSoulReader(dreamSvc.CurrentSoul)
	b.RegisterDataSource(messaging.NewDataSource(cfg, messagingSvc))
	b.RegisterDataSource(alarms.NewDataSource(alarmSvc, messagingSvc))
	b.RegisterDataSource(shell.NewDataSource(messagingSvc, b.IsActive))
	b.RegisterDataSource(journal.NewDataSource(journalSvc, conn, messagingSvc, cfg))
	b.RegisterDataSource(dream.NewDataSource(dreamSvc, conn, memorySvc, cfg))
	b.RegisterDataSource(briefing.NewDataSource(briefingSvc, cfg))

	b.RegisterTools(config.BuildTools(cfg, conn)...)
	b.RegisterTools(contacts.BuildTools(conn)...)
	b.RegisterTools(llm.BuildTools(llmClient, cfg.Home)...)
	b.RegisterTools(memory.BuildTools(memorySvc)...)
	b.RegisterTools(messaging.BuildTools(messagingSvc)...)
	b.RegisterTools(alarms.BuildTools(alarmSvc)...)
	b.RegisterTools(search.BuildTools(conn, searchSvc)...)
	b.RegisterTools(shell.BuildTools(cfg)...)
	b.RegisterTools(websearch.BuildTools(cfg)...)
	b.RegisterTools(skills.BuildTools(cfg)...)
	b.RegisterTools(journal.BuildTools(journalSvc)...)
	b.RegisterTools(dream.BuildTools(dreamSvc)...)
	b.RegisterTools(briefing.BuildTools(briefingSvc)...)

	go b.Awake(ctx, 2*time.Second)

	if err := whatsappClient.Connect(ctx, *wappLink); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	<-ctx.Done()
	slog.Info("shutting down")
}
