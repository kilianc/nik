package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestNewAppSetup(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	cfg := &config.Config{Home: t.TempDir()}
	app := NewApp(cfg, conn, nil, true, Options{})

	if app.view != viewSetup {
		t.Errorf("expected setup view, got %d", app.view)
	}
}

func TestNewAppChat(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	cfg := &config.Config{Home: t.TempDir()}
	app := NewApp(cfg, conn, nil, false, Options{})

	if app.view != viewChat {
		t.Errorf("expected chat view, got %d", app.view)
	}
}

func TestAppViewRendersSetup(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	cfg := &config.Config{Home: t.TempDir()}
	app := NewApp(cfg, conn, nil, true, Options{})

	output := app.View()
	if output == "" {
		t.Error("expected non-empty view output")
	}
}

func TestAppWindowSizeMsg(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	cfg := &config.Config{Home: t.TempDir()}
	app := NewApp(cfg, conn, nil, true, Options{})

	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated := model.(App)
	if updated.width != 80 {
		t.Errorf("expected width 80, got %d", updated.width)
	}
	if updated.height != 24 {
		t.Errorf("expected height 24, got %d", updated.height)
	}
}
