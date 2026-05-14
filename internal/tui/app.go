package tui

import (
	"database/sql"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kciuffolo/nik/internal/config"
)

type view int

const (
	viewSetup view = iota
	viewChat
)

type App struct {
	view   view
	setup  setupModel
	chat   chatModel
	cfg    *config.Config
	conn   *sql.DB
	sender MessageSender
	opts   Options
	width  int
	height int
}

type Options struct {
	ShowSystem bool
}

func NewApp(cfg *config.Config, conn *sql.DB, sender MessageSender, setup bool, opts Options) App {
	a := App{
		cfg:    cfg,
		conn:   conn,
		sender: sender,
		opts:   opts,
	}

	if setup {
		a.view = viewSetup
		a.setup = newSetupModel(cfg, conn)
	} else {
		a.view = viewChat
		a.chat = newChatModel(cfg, conn, sender, opts)
	}

	return a
}

func (a App) Init() tea.Cmd {
	switch a.view {
	case viewSetup:
		return a.setup.Init()
	case viewChat:
		return a.chat.Init()
	}
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wmsg, ok := msg.(tea.WindowSizeMsg); ok {
		a.width = wmsg.Width
		a.height = wmsg.Height
	}

	switch a.view {
	case viewSetup:
		var cmd tea.Cmd
		a.setup, cmd = a.setup.Update(msg)

		if a.setup.isDone() {
			a.view = viewChat
			a.chat = newChatModel(a.cfg, a.conn, a.sender, a.opts)
			a.chat, _ = a.chat.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			return a, a.chat.Init()
		}

		return a, cmd

	case viewChat:
		var cmd tea.Cmd
		a.chat, cmd = a.chat.Update(msg)
		return a, cmd
	}

	return a, nil
}

func (a App) View() string {
	switch a.view {
	case viewSetup:
		return appStyle.Render(a.setup.View())
	case viewChat:
		return a.chat.View()
	}
	return ""
}

func Run(cfg *config.Config, conn *sql.DB, sender MessageSender, setup bool, opts Options) error {
	app := NewApp(cfg, conn, sender, setup, opts)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
