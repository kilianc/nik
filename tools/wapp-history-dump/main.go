// wapp-history-dump connects to WhatsApp, records HistorySync protobuf
// data to a file, and exits on Ctrl-C.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	flag.Parse()

	outPath := "wapp_history.pb64"
	if flag.NArg() > 0 {
		outPath = flag.Arg(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	tmpDB, err := os.CreateTemp("", "wapp-history-dump-*.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp session db: %v\n", err)
		os.Exit(1)
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	defer os.Remove(tmpDBPath)

	dsn := fmt.Sprintf("file:%s?_foreign_keys=on", tmpDBPath)
	container, err := sqlstore.New(ctx, "sqlite3", dsn, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open session db: %v\n", err)
		os.Exit(1)
	}

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get device: %v\n", err)
		os.Exit(1)
	}

	wm := whatsmeow.NewClient(device, nil)
	wm.Store.PushName = "Nik"

	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	var chunks atomic.Int64
	idleTimeout := 30 * time.Second
	idle := time.NewTimer(idleTimeout)
	idle.Stop()

	wm.AddEventHandler(func(evt any) {
		hs, ok := evt.(*events.HistorySync)
		if !ok {
			return
		}
		raw, err := proto.Marshal(hs.Data)
		if err != nil {
			slog.Error("marshal history sync", "error", err)
			return
		}
		outFile.WriteString(base64.StdEncoding.EncodeToString(raw) + "\n")
		n := chunks.Add(1)
		convs := len(hs.Data.GetConversations())
		slog.Info("recorded chunk", "chunk", n, "conversations", convs)
		idle.Reset(idleTimeout)
	})

	qrChan, err := wm.GetQRChannel(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get QR channel: %v\n", err)
		os.Exit(1)
	}
	err = wm.ConnectContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	for item := range qrChan {
		if item.Event == "code" {
			fmt.Println()
			qrterminal.Generate(item.Code, qrterminal.L, os.Stdout)
			fmt.Println()
		} else if item.Event == "success" {
			break
		} else if item.Error != nil {
			fmt.Fprintf(os.Stderr, "QR pairing: %v\n", item.Error)
			os.Exit(1)
		}
	}

	slog.Info("connected, recording history sync events", "output", outPath)
	slog.Info("will exit after 30s of no new chunks, or press Ctrl-C")
	idle.Reset(idleTimeout)

	select {
	case <-ctx.Done():
	case <-idle.C:
		slog.Info("no new chunks for 30s, finishing")
	}
	wm.Disconnect()
	slog.Info("done", "chunks_recorded", chunks.Load())
}
