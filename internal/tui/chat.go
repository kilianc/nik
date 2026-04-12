package tui

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"time"

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

const (
	easeOutRate = 0.03
	easeInRate  = 0.06
)

type chatModel struct {
	input       textinput.Model
	conn        *sql.DB
	sender      MessageSender
	messages    []db.Message
	lastID      string
	width       int
	activity    []string
	err         error
	thinkTick   int
	thinkEnergy float64
	convCache   string
	convDirty   bool
	wasTyping   bool
}

func newChatModel(conn *sql.DB, sender MessageSender) chatModel {
	ti := textinput.New()
	ti.Placeholder = "message..."
	ti.Prompt = "❯ "
	ti.PromptStyle = promptStyle
	ti.CharLimit = 0
	ti.Focus()

	return chatModel{
		input:  ti,
		conn:   conn,
		sender: sender,
	}
}

type pollTickMsg time.Time
type thinkTickMsg time.Time
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

func thinkTickCmd() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(t time.Time) tea.Msg {
		return thinkTickMsg(t)
	})
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, fetchMessagesCmd(m.conn, ""))
}

func (m chatModel) Update(msg tea.Msg) (chatModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		const maxChatWidth = 120
		m.width = msg.Width
		if m.width > maxChatWidth {
			m.width = maxChatWidth
		}
		m.input.Width = m.width - 4
		m.convDirty = true
		m.refreshConvCache()
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
		const maxMessages = 500
		var cmds []tea.Cmd

		if len(msg.messages) > 0 {
			m.messages = append(m.messages, msg.messages...)
			m.lastID = msg.messages[len(msg.messages)-1].ID
			m.convDirty = true
			if len(m.messages) > maxMessages {
				m.messages = m.messages[len(m.messages)-maxMessages:]
			}
		}

		wasIdle := len(m.activity) == 0
		m.activity = msg.activity

		typing := slices.Contains(m.activity, "typing")
		if typing != m.wasTyping {
			m.convDirty = true
			m.wasTyping = typing
		}

		if wasIdle && len(m.activity) > 0 {
			cmds = append(cmds, thinkTickCmd())
		}

		if m.convDirty {
			m.refreshConvCache()
		}

		cmds = append(cmds, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return pollTickMsg(t)
		}))
		return m, tea.Batch(cmds...)

	case messageSentMsg:
		if msg.err != nil {
			m.err = msg.err
			m.refreshConvCache()
		}
		return m, nil

	case thinkTickMsg:
		m.thinkTick++
		active := len(m.activity) > 0

		if active {
			if m.thinkEnergy < 1.0 {
				m.thinkEnergy += easeInRate * (1.0 - m.thinkEnergy + 0.05)
				if m.thinkEnergy > 1.0 {
					m.thinkEnergy = 1.0
				}
			}
			return m, thinkTickCmd()
		}

		if m.thinkEnergy > 0.0 {
			m.thinkEnergy *= (1.0 - easeOutRate)
			if m.thinkEnergy < 0.005 {
				m.thinkEnergy = 0.0
				return m, nil
			}
			return m, thinkTickCmd()
		}

		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m chatModel) viewWidth() int {
	const pad = 1
	w := m.width - 2*pad
	if w <= 0 {
		w = 80
	}
	return w
}

func (m *chatModel) refreshConvCache() {
	w := m.viewWidth()

	typingTick := -1
	if slices.Contains(m.activity, "typing") {
		typingTick = m.thinkTick
	}

	var b strings.Builder
	b.WriteString(renderConversation(m.messages, w, m.thinkTick, typingTick))

	if m.err != nil {
		b.WriteString(errorStyle.Render(m.err.Error()) + "\n")
	}

	m.convCache = b.String()
	m.convDirty = false
}

func (m chatModel) View() string {
	const pad = 1
	w := m.viewWidth()

	var b strings.Builder
	b.WriteString(m.convCache)

	b.WriteString(strings.Repeat("\n", 4))
	b.WriteString(m.input.View() + "\n")
	b.WriteString(thinkMorph(m.thinkTick, m.thinkEnergy, w) + "\n")

	return lipgloss.NewStyle().PaddingLeft(pad).PaddingRight(pad).Render(b.String())
}

func bubble(body string, isNik bool, width int) string {
	maxW := width*3/4 - 4
	if maxW < 20 {
		maxW = 20
	}

	border := youBorder
	text := msgText
	if isNik {
		border = nikBorder
	}

	wrapped := lipgloss.NewStyle().Width(maxW).Render(body)
	raw := strings.Split(wrapped, "\n")

	var lines []string
	for _, l := range raw {
		lines = append(lines, strings.TrimRight(l, " "))
	}

	boxW := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > boxW {
			boxW = w
		}
	}

	top := border.Render("╭" + strings.Repeat("─", boxW+2) + "╮")
	bot := border.Render("╰" + strings.Repeat("─", boxW+2) + "╯")

	var out []string
	out = append(out, top)
	for _, l := range lines {
		pad := boxW - lipgloss.Width(l)
		inner := text.Render(" " + l + strings.Repeat(" ", pad) + " ")
		out = append(out, border.Render("│")+inner+border.Render("│"))
	}
	out = append(out, bot)

	joined := strings.Join(out, "\n")
	if isNik {
		return joined
	}

	return lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(joined)
}

func groupLabel(t time.Time, isNik bool, width int) string {
	ts := dimText.Render(t.Format("15:04"))
	if isNik {
		return " " + ts + " " + nikLabel.Render("nik")
	}
	label := youLabel.Render("you") + " " + ts
	return lipgloss.NewStyle().Width(width - 1).Align(lipgloss.Right).Render(label)
}

func dateSep(s string, width int) string {
	rule := strings.Repeat("─", 3)
	text := rule + " " + s + " " + rule
	w := lipgloss.Width(text)
	pad := (width - w) / 2
	if pad < 0 {
		pad = 0
	}
	return sepText.Render(strings.Repeat(" ", pad) + text)
}

func ghostBubble(tick int) string {
	frame := spinnerFrames[(tick/4)%len(spinnerFrames)]
	ndots := (tick / 8) % 4
	dots := strings.Repeat(".", ndots) + strings.Repeat(" ", 3-ndots)
	content := " " + dimText.Render(frame+" typing"+dots) + " "
	border := nikBorder
	w := lipgloss.Width(content)
	top := border.Render("╭" + strings.Repeat("─", w) + "╮")
	bot := border.Render("╰" + strings.Repeat("─", w) + "╯")
	mid := border.Render("│") + content + border.Render("│")
	return top + "\n" + mid + "\n" + bot
}

func renderToolLine(tick int, toolName string, state toolState, reason string, width int) string {
	pipe := toolRailStyle.Render(" │ ")

	var indicator string
	switch state {
	case toolDone:
		indicator = checkStyle.Render("✓")
	case toolError:
		indicator = errorIndicatorStyle.Render("✗")
	default:
		frame := spinnerFrames[(tick/4)%len(spinnerFrames)]
		indicator = spinnerColor.Render(frame)
	}

	name := toolNameStyle.Render(toolName)
	base := pipe + indicator + " " + name
	if reason == "" {
		return base
	}

	sep := " — "
	avail := width - lipgloss.Width(base) - lipgloss.Width(sep)
	if avail <= 0 {
		return base
	}
	if len(reason) > avail {
		reason = reason[:avail-1] + "…"
	}
	return base + toolDimStyle.Render(sep+reason)
}

func renderConversation(msgs []db.Message, width int, tick int, nikTypingTick int) string {
	var b strings.Builder
	var prevSender string
	var prevDate string

	// build pairing map: tool_call_start ID → tool_call message
	pairedToolCalls := make(map[string]*db.Message)
	for i := range msgs {
		m := &msgs[i]
		if m.Kind == "tool_call" && m.ContextStanzaID.Valid {
			pairedToolCalls[m.ContextStanzaID.String] = m
		}
	}

	var entries []entry
	for _, m := range msgs {
		if !isVisibleKind(m.Kind) {
			continue
		}

		if m.Kind == "tool_call" {
			continue
		}

		if m.Kind == "tool_call_start" {
			name, reason := parseToolCallStart(m.Body)
			if name == "" || name == "done" {
				continue
			}

			state := resolveToolCallState(pairedToolCalls[m.ID])

			lt := m.SentAt.Local()
			if m.SentAt.IsZero() {
				lt = time.Now().Local()
			}

			entries = append(entries, entry{
				lt:        lt,
				isNik:     true,
				body:      reason,
				toolName:  name,
				toolState: state,
			})
			continue
		}

		if strings.TrimSpace(m.Body) == "" {
			continue
		}
		if m.Platform == "system" {
			continue
		}

		lt := m.SentAt.Local()
		if m.SentAt.IsZero() {
			lt = time.Now().Local()
		}

		isNik := m.IsFromMe || m.ContactID == db.SystemContactID

		entries = append(entries, entry{
			lt:       lt,
			isNik:    isNik,
			body:     m.Body,
			platform: m.Platform,
		})
	}

	for i, e := range entries {
		d := e.lt.Format("2006-01-02")
		if d != prevDate {
			if prevDate != "" {
				b.WriteString("\n")
			}
			b.WriteString(dateSep(formatDate(e.lt), width) + "\n\n")
			prevDate = d
			prevSender = ""
		}

		if i > 0 && prevSender != "" && !sameGroup(entries[i-1], e) {
			b.WriteString("\n")
		}

		if e.toolName != "" {
			firstTool := i == 0 || entries[i-1].toolName == "" || !sameGroup(entries[i-1], e)
			if firstTool {
				b.WriteString(toolRailStyle.Render(" ╭") + "\n")
			}
			b.WriteString(renderToolLine(tick, e.toolName, e.toolState, e.body, width) + "\n")
		} else {
			b.WriteString(bubble(e.body, e.isNik, width) + "\n")
		}

		isLast := i == len(entries)-1
		endOfGroup := isLast || !sameGroup(e, entries[i+1])

		lastTool := e.toolName != "" && (isLast || entries[i+1].toolName == "" || !sameGroup(e, entries[i+1]))
		if lastTool {
			b.WriteString(toolRailStyle.Render(" ╰") + "\n")
		}

		if isLast && nikTypingTick >= 0 {
			if e.isNik {
				b.WriteString(ghostBubble(nikTypingTick) + "\n")
			}
			if endOfGroup {
				b.WriteString(groupLabel(e.lt, e.isNik, width) + "\n")
			}
			if !e.isNik {
				b.WriteString("\n" + ghostBubble(nikTypingTick) + "\n")
			}
		} else if endOfGroup {
			b.WriteString(groupLabel(e.lt, e.isNik, width) + "\n")
		}

		if e.isNik {
			prevSender = "nik"
		} else {
			prevSender = "you"
		}
	}

	if len(entries) == 0 && nikTypingTick >= 0 {
		b.WriteString(ghostBubble(nikTypingTick) + "\n")
	}

	return b.String()
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
	"text":            true,
	"image":           true,
	"audio":           true,
	"video":           true,
	"document":        true,
	"location":        true,
	"contact":         true,
	"tool_call_start": true,
	"tool_call":       true,
}

func isVisibleKind(kind string) bool {
	return visibleKinds[kind]
}

type entry struct {
	lt        time.Time
	isNik     bool
	body      string
	toolName  string
	toolState toolState
	platform  string
}

func sameGroup(a, b entry) bool {
	if a.isNik != b.isNik {
		return false
	}
	if b.lt.Sub(a.lt) > 5*time.Minute {
		return false
	}
	return a.lt.Format("2006-01-02") == b.lt.Format("2006-01-02")
}

type toolState int

const (
	toolRunning toolState = iota
	toolDone
	toolError
)

type toolCallStartBody struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

type toolCallStartInput struct {
	Reason string `json:"reason"`
}

type toolCallBody struct {
	Output string `json:"output"`
}

type toolCallOutput struct {
	Error string `json:"error"`
}

func parseToolCallStart(body string) (name, reason string) {
	var tc toolCallStartBody
	json.Unmarshal([]byte(body), &tc)

	var inp toolCallStartInput
	json.Unmarshal([]byte(tc.Input), &inp)

	return tc.Name, inp.Reason
}

func resolveToolCallState(paired *db.Message) toolState {
	if paired == nil {
		return toolRunning
	}

	var tc toolCallBody
	json.Unmarshal([]byte(paired.Body), &tc)

	var out toolCallOutput
	json.Unmarshal([]byte(tc.Output), &out)
	if out.Error != "" {
		return toolError
	}

	return toolDone
}
