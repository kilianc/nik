package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/nikapi"
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
	client *nikapi.Client
	sender MessageSender
	opts   Options
	width  int
	height int
}

type Options struct {
	ShowSystem bool
	// Home is the workspace path, for the header strip. The TUI is told it
	// rather than working it out, since which directory nik lives in is
	// nikd's business.
	Home string
	// BornAt is when the chat agent first came online. Zero value hides the
	// age chip from the header strip.
	BornAt time.Time
	// InputGate decides whether the chat input is locked and what placeholder
	// to show. Called on every poll tick. Nil means input is always editable
	// with the default placeholder.
	InputGate InputGate
}

// InputState describes how the chat input should behave. Zero value means
// editable with the default placeholder.
type InputState struct {
	Locked      bool
	Placeholder string
}

// InputGate returns the current input state given the chat's message tail and
// activity flags.
type InputGate func(messages []nikapi.Message, activity []string) InputState

func NewApp(cfg *config.Config, client *nikapi.Client, sender MessageSender, setup bool, opts Options) App {
	a := App{
		cfg:    cfg,
		client: client,
		sender: sender,
		opts:   opts,
	}

	if setup {
		a.view = viewSetup
		a.setup = newSetupModel(cfg, client)
	} else {
		a.view = viewChat
		a.chat = newChatModel(client, sender, opts)
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
			a.chat = newChatModel(a.client, a.sender, a.opts)
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

func Run(cfg *config.Config, client *nikapi.Client, sender MessageSender, setup bool, opts Options) error {
	app := NewApp(cfg, client, sender, setup, opts)
	p := tea.NewProgram(app)
	_, err := p.Run()
	return err
}
