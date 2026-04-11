package tui

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/messaging"
)

type MessageSender interface {
	Send(ctx context.Context, body string) error
}

type localSender struct {
	svc *messaging.Service
}

func (s *localSender) Send(ctx context.Context, body string) error {
	return s.svc.ReceiveMessage(ctx, messaging.InboundMessage{
		Platform:               "local",
		ExternalConversationID: db.LocalConversationID,
		ExternalMessageID:      id.V7(),
		ExternalSenderID:       db.OwnerContactID,
		ExternalSenderIDs:      []string{db.OwnerContactID},
		Kind:                   "text",
		Body:                   body,
		SentAt:                 time.Now(),
	})
}

func NewLocalSender(svc *messaging.Service) MessageSender {
	return &localSender{svc: svc}
}

type chatModel struct {
	input    textinput.Model
	spinner  spinner.Model
	conn     *sql.DB
	sender   MessageSender
	messages []db.Message
	lastID   string
	width    int
	activity []string
	err      error
}

func newChatModel(conn *sql.DB, sender MessageSender) chatModel {
	ti := textinput.New()
	ti.Placeholder = "message..."
	ti.Prompt = "❯ "
	ti.PromptStyle = promptStyle
	ti.CharLimit = 0
	ti.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return chatModel{
		input:   ti,
		spinner: s,
		conn:    conn,
		sender:  sender,
	}
}

type pollTickMsg time.Time
type newMessagesMsg struct {
	messages []db.Message
	activity []string
}
type messageSentMsg struct{ err error }

func fetchMessagesCmd(conn *sql.DB, afterID string) tea.Cmd {
	return func() tea.Msg {
		if conn == nil {
			return newMessagesMsg{}
		}

		ctx := context.Background()

		params := db.MessageListParams{
			ConversationID: db.LocalConversationID,
			Limit:          200,
		}
		if afterID != "" {
			params.AfterID = afterID
		}

		msgs, err := db.MessageList(ctx, conn, params)
		if err != nil {
			return newMessagesMsg{}
		}

		slices.Reverse(msgs)

		var activity []string
		conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: db.LocalConversationID})
		if err == nil {
			activity = conv.Activity
		}

		return newMessagesMsg{messages: msgs, activity: activity}
	}
}

func sendMessageCmd(sender MessageSender, body string) tea.Cmd {
	return func() tea.Msg {
		err := sender.Send(context.Background(), body)
		return messageSentMsg{err: err}
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick, fetchMessagesCmd(m.conn, ""))
}

func (m chatModel) Update(msg tea.Msg) (chatModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width / 2
		m.input.Width = m.width - 4
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			return m, sendMessageCmd(m.sender, text)
		}

	case pollTickMsg:
		return m, fetchMessagesCmd(m.conn, m.lastID)

	case newMessagesMsg:
		var cmds []tea.Cmd

		if len(msg.messages) > 0 {
			m.messages = append(m.messages, msg.messages...)
			m.lastID = msg.messages[len(msg.messages)-1].ID
		}

		wasIdle := len(m.activity) == 0
		m.activity = msg.activity
		if wasIdle && len(m.activity) > 0 {
			cmds = append(cmds, m.spinner.Tick)
		}

		cmds = append(cmds, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return pollTickMsg(t)
		}))
		return m, tea.Batch(cmds...)

	case messageSentMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nil

	case spinner.TickMsg:
		if len(m.activity) > 0 {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m chatModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	var b strings.Builder

	b.WriteString(renderMessages(m.messages, w))

	switch {
	case m.err != nil:
		b.WriteString(errorStyle.Render(m.err.Error()) + "\n")
	case slices.Contains(m.activity, "typing"):
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("nik is typing...") + "\n")
	case slices.Contains(m.activity, "thinking"):
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("nik is thinking...") + "\n")
	}

	b.WriteString(m.input.View())
	return b.String()
}

func renderMessages(msgs []db.Message, width int) string {
	var b strings.Builder
	var prevSender string
	var prevTime time.Time
	var prevDate string

	for _, msg := range msgs {
		if !isVisibleKind(msg.Kind) || strings.TrimSpace(msg.Body) == "" {
			continue
		}

		lt := msg.SentAt.Local()
		if msg.SentAt.IsZero() {
			lt = time.Now().Local()
		}

		dateStr := formatDate(lt)
		if dateStr != prevDate {
			if prevDate != "" {
				b.WriteString("\n")
			}
			rule := strings.Repeat("─", 3)
			b.WriteString(chatSepStyle.Render(centerDate(rule+" "+dateStr+" "+rule, width)))
			b.WriteString("\n\n")
			prevDate = dateStr
			prevSender = ""
		}

		if msg.Platform == "system" {
			continue
		}

		isNik := msg.IsFromMe || msg.ContactID == db.SystemContactID
		sender := "you"
		if isNik {
			sender = "nik"
		}

		senderChanged := sender != prevSender && prevSender != ""
		bigGap := lt.Sub(prevTime) > 5*time.Minute
		if senderChanged || bigGap {
			b.WriteString("\n")
			prevSender = ""
		}

		b.WriteString(renderLine(lt, isNik, msg.Body, prevSender, width))
		b.WriteString("\n")

		prevSender = sender
		prevTime = lt
	}

	return b.String()
}

func renderLine(lt time.Time, isNik bool, body string, prevSender string, width int) string {
	ts := timestampStyle.Render(lt.Format("15:04"))

	nameStyle := chatYouName
	name := "you"
	if isNik {
		nameStyle = chatNikName
		name = "nik"
	}

	prefix := ts + " " + nameStyle.Render(name) + " "
	prefixLen := lipgloss.Width(prefix)

	if name == prevSender {
		prefix = strings.Repeat(" ", prefixLen)
	}

	bodyWidth := width - prefixLen
	if bodyWidth < 20 {
		bodyWidth = 20
	}

	wrapped := lipgloss.NewStyle().Width(bodyWidth).Render(body)
	bodyLines := strings.Split(wrapped, "\n")

	var out []string
	for i, line := range bodyLines {
		if i == 0 {
			out = append(out, prefix+line)
		} else {
			out = append(out, strings.Repeat(" ", prefixLen)+line)
		}
	}

	return strings.Join(out, "\n")
}

func centerDate(s string, width int) string {
	w := lipgloss.Width(s)
	if width <= w {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}

func formatDate(t time.Time) string {
	now := time.Now().Local()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return "Today"
	}
	yesterday := now.AddDate(0, 0, -1)
	if t.Year() == yesterday.Year() && t.YearDay() == yesterday.YearDay() {
		return "Yesterday"
	}
	if t.Year() == now.Year() {
		return t.Format("Mon, Jan 2")
	}
	return t.Format("Mon, Jan 2, 2006")
}

var visibleKinds = map[string]bool{
	"text":     true,
	"image":    true,
	"audio":    true,
	"video":    true,
	"document": true,
	"location": true,
	"contact":  true,
}

func isVisibleKind(kind string) bool {
	return visibleKinds[kind]
}
