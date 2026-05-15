package tui

import (
	"database/sql"

	tea "charm.land/bubbletea/v2"
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

func (a App) View() tea.View {
	var content string
	var mouseMode tea.MouseMode
	switch a.view {
	case viewSetup:
		content = appStyle.Render(a.setup.View())
		// No mouse capture during setup — the terminal needs it so users can
		// drag-select the sign-in URL.
		mouseMode = tea.MouseModeNone
	case viewChat:
		content = a.chat.View()
		// Chat needs mouse events for viewport wheel-scrolling.
		mouseMode = tea.MouseModeCellMotion
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = mouseMode
	return v
}

func Run(cfg *config.Config, conn *sql.DB, sender MessageSender, setup bool, opts Options) error {
	app := NewApp(cfg, conn, sender, setup, opts)
	p := tea.NewProgram(app)
	_, err := p.Run()
	return err
}
