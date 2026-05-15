package tui

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/daemonctl"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/genesis"
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
	input       textinput.Model
	viewport    viewport.Model
	cfg         *config.Config
	conn        *sql.DB
	sender      MessageSender
	messages    []db.Message
	lastID      string
	width       int
	height      int
	activity    []string
	err         error
	pulse       *pulse
	anims       animators
	convCache   string
	convDirty   bool
	ghostLabel  string
	cachedFrame int
	showSystem  bool
	inputLocked bool
	genesisSeed string
	genesisAt   time.Time
	daemonAlive bool
	vpReady     bool

	pendingAlarms int
	activeTasks   int
}

func newChatModel(cfg *config.Config, conn *sql.DB, sender MessageSender, opts Options) chatModel {
	ti := textinput.New()
	ti.Placeholder = "message..."
	ti.Prompt = "❯ "
	styles := ti.Styles()
	styles.Focused.Prompt = promptStyle
	styles.Blurred.Prompt = promptStyle
	ti.SetStyles(styles)
	ti.CharLimit = 0
	ti.Focus()

	p := newPulse(30 * time.Millisecond)
	return chatModel{
		input:      ti,
		cfg:        cfg,
		conn:       conn,
		sender:     sender,
		pulse:      p,
		anims:      animators{p},
		showSystem: opts.ShowSystem,
	}
}

type pollTickMsg time.Time
type daemonTickMsg struct{ alive bool }
type genesisLoadedMsg struct{ at time.Time }
type workloadMsg struct {
	alarms int
	tasks  int
}

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

func daemonTickCmd(home string, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		_, alive := daemonctl.CheckPID(home)
		return daemonTickMsg{alive: alive}
	})
}

func workloadCmd(conn *sql.DB, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		if conn == nil {
			return workloadMsg{}
		}
		ctx := context.Background()
		alarms, _ := db.AlarmCountActive(ctx, conn)
		tasks, _ := db.TaskCountActive(ctx, conn)
		return workloadMsg{alarms: alarms, tasks: tasks}
	})
}

func (m chatModel) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink, fetchMessagesCmd(m.conn, "")}
	if m.cfg != nil {
		cmds = append(cmds, daemonTickCmd(m.cfg.Home, 0))
	}
	if m.conn != nil {
		cmds = append(cmds, loadGenesisCmd(m.conn), workloadCmd(m.conn, 0))
	}
	return tea.Batch(cmds...)
}

func loadGenesisCmd(conn *sql.DB) tea.Cmd {
	return func() tea.Msg {
		if conn == nil {
			return genesisLoadedMsg{}
		}
		s, err := db.SettingGet(context.Background(), conn, db.GenesisStartedAtKey)
		if err != nil || s == nil {
			return genesisLoadedMsg{}
		}
		t, err := db.ParseTimeValue(s.Value)
		if err != nil {
			return genesisLoadedMsg{}
		}
		return genesisLoadedMsg{at: t}
	}
}

func (m chatModel) Update(msg tea.Msg) (chatModel, tea.Cmd) {
	var cmds []tea.Cmd

	// Universal animator fanout. MUST stay at the top of Update so animation
	// messages (pulseTickMsg etc.) can never be swallowed by a fallthrough
	// handler — see setup spinner history.
	if cmd := m.anims.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if frame := m.pulse.Tick() / 4; frame != m.cachedFrame {
		m.cachedFrame = frame
		if len(m.activity) > 0 {
			m.refreshConvCache()
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		const maxChatWidth = 120
		m.width = msg.Width
		if m.width > maxChatWidth {
			m.width = maxChatWidth
		}
		m.height = msg.Height
		m.resyncLayout()
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.inputLocked {
				return m, tea.Batch(cmds...)
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, tea.Batch(cmds...)
			}
			m.input.Reset()
			if m.vpReady {
				m.viewport.GotoBottom()
			}
			cmds = append(cmds, sendMessageCmd(m.sender, text))
			return m, tea.Batch(cmds...)
		case "up", "down", "pgup", "pgdown", "home", "end":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}
		if m.inputLocked {
			return m, tea.Batch(cmds...)
		}

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case daemonTickMsg:
		m.daemonAlive = msg.alive
		if m.cfg != nil {
			cmds = append(cmds, daemonTickCmd(m.cfg.Home, 3*time.Second))
		}
		return m, tea.Batch(cmds...)

	case workloadMsg:
		m.pendingAlarms = msg.alarms
		m.activeTasks = msg.tasks
		if m.conn != nil {
			cmds = append(cmds, workloadCmd(m.conn, 5*time.Second))
		}
		return m, tea.Batch(cmds...)

	case genesisLoadedMsg:
		m.genesisAt = msg.at
		return m, tea.Batch(cmds...)

	case pollTickMsg:
		cmds = append(cmds, fetchMessagesCmd(m.conn, m.lastID))
		return m, tea.Batch(cmds...)

	case newMessagesMsg:
		const maxMessages = 500

		if len(msg.messages) > 0 {
			m.messages = append(m.messages, msg.messages...)
			m.lastID = msg.messages[len(msg.messages)-1].ID
			m.convDirty = true
			if len(m.messages) > maxMessages {
				m.messages = m.messages[len(m.messages)-maxMessages:]
			}
			m.genesisSeed = currentGenesisSeed(m.messages)
		}

		m.activity = msg.activity

		ghost := ""
		switch {
		case slices.Contains(m.activity, "typing"):
			ghost = "typing"
		case slices.Contains(m.activity, "thinking"):
			ghost = "thinking"
		}
		if ghost != m.ghostLabel {
			m.convDirty = true
			m.ghostLabel = ghost
		}

		locked := computeInputLocked(m.genesisSeed, m.activity)
		if locked != m.inputLocked {
			m.inputLocked = locked
			m.convDirty = true
			if locked {
				m.input.Blur()
			} else {
				cmds = append(cmds, m.input.Focus())
			}
		}
		m.input.Placeholder = placeholderFor(m.inputLocked, m.genesisSeed)

		if cmd := m.pulse.SetActive(len(m.activity) > 0); cmd != nil {
			cmds = append(cmds, cmd)
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
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m chatModel) viewWidth() int {
	w := m.width - 2*chatHorizontalPad
	if w <= 0 {
		w = 80
	}
	return w
}

const (
	chatHeaderHeight  = 3
	chatHeaderGap     = 1
	chatFooterGap     = 1
	chatFooterHeight  = 1
	chatHorizontalPad = 2
	chatVerticalPad   = 1
	chatChromeHeight  = chatHeaderHeight + chatHeaderGap + chatFooterGap + chatFooterHeight + 2*chatVerticalPad
)

func (m chatModel) viewportHeight() int {
	h := m.height - chatChromeHeight
	if h < 3 {
		h = 3
	}
	return h
}

func (m *chatModel) resyncLayout() {
	m.input.SetWidth(m.viewWidth() - 2)

	w, h := m.viewWidth(), m.viewportHeight()
	if !m.vpReady {
		m.viewport = viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))
		m.vpReady = true
	} else {
		m.viewport.SetWidth(w)
		m.viewport.SetHeight(h)
	}
	m.convDirty = true
	m.refreshConvCache()
}

func (m *chatModel) refreshConvCache() {
	w := m.viewWidth()
	tick := m.pulse.Tick()

	ghostTick := -1
	ghostLabel := ""
	switch {
	case slices.Contains(m.activity, "typing"):
		ghostTick = tick
		ghostLabel = "typing"
	case slices.Contains(m.activity, "thinking"):
		ghostTick = tick
		ghostLabel = "thinking"
	}

	var b strings.Builder
	b.WriteString(renderConversation(m.messages, w, tick, ghostTick, ghostLabel, m.showSystem))

	if m.err != nil {
		b.WriteString(errorStyle.Render(m.err.Error()) + "\n")
	}

	m.convCache = b.String()
	m.convDirty = false

	if m.vpReady {
		wasAtBottom := m.viewport.AtBottom()
		m.viewport.SetContent(m.convCache)
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
	}
}

func (m chatModel) View() string {
	var body string
	if m.vpReady {
		body = m.viewport.View()
	} else {
		body = m.convCache
	}

	var b strings.Builder
	b.WriteString(m.renderHeader() + "\n")
	b.WriteString(strings.Repeat("\n", chatHeaderGap))
	b.WriteString(body + "\n")
	b.WriteString(strings.Repeat("\n", chatFooterGap))
	b.WriteString(m.input.View())

	return lipgloss.NewStyle().
		Padding(chatVerticalPad, chatHorizontalPad).
		Render(b.String())
}

func bubble(body string, isNik bool, isSystem bool, width int, reactions []string) string {
	maxW := width*3/4 - 4
	if maxW < 20 {
		maxW = 20
	}

	border := youBorder
	text := msgText
	if isSystem {
		border = systemBorder
	} else if isNik {
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

	var reactStr string
	if len(reactions) > 0 {
		reactStr = " " + strings.Join(reactions, " ") + " "
		if minW := lipgloss.Width(reactStr) - 1; boxW < minW {
			boxW = minW
		}
	}

	top := border.Render("╭" + strings.Repeat("─", boxW+2) + "╮")

	var bot string
	if reactStr != "" {
		dashes := boxW + 2 - lipgloss.Width(reactStr)
		bot = border.Render("╰"+strings.Repeat("─", dashes)) + reactStr + border.Render("╯")
	} else {
		bot = border.Render("╰" + strings.Repeat("─", boxW+2) + "╯")
	}

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

func ghostBubble(tick int, label string) string {
	ndots := (tick / 8) % 4
	dots := strings.Repeat(".", ndots) + strings.Repeat(" ", 3-ndots)
	content := " " + thinkingSpinnerStyle.Render(spinnerGlyph(tick)) + thinkingStyle.Render(" "+label+dots) + " "
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
		indicator = spinnerColor.Render(spinnerGlyph(tick))
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

func renderConversation(msgs []db.Message, width int, tick int, ghostTick int, ghostLabel string, showSystem bool) string {
	var b strings.Builder
	var prevSender string
	var prevDate string

	reactionMap := collectReactions(msgs)

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
			if name == "" || name == "done" || name == "message_send" {
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
		lt := m.SentAt.Local()
		if m.SentAt.IsZero() {
			lt = time.Now().Local()
		}

		if m.Platform == "system" && !showSystem {
			continue
		}

		isNik := m.IsFromMe || m.ContactID == db.SystemContactID

		entries = append(entries, entry{
			lt:            lt,
			isNik:         isNik,
			body:          m.Body,
			platform:      m.Platform,
			externalMsgID: m.ExternalMessageID,
			reactions:     reactionMap[m.ExternalMessageID],
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
			body := e.body
			if e.platform == "system" {
				body = dimText.Render("system ›") + " " + body
			}
			b.WriteString(bubble(body, e.isNik, e.platform == "system", width, e.reactions) + "\n")
		}

		isLast := i == len(entries)-1
		endOfGroup := isLast || !sameGroup(e, entries[i+1])

		lastTool := e.toolName != "" && (isLast || entries[i+1].toolName == "" || !sameGroup(e, entries[i+1]))
		if lastTool {
			b.WriteString(toolRailStyle.Render(" ╰") + "\n")
		}

		if isLast && ghostTick >= 0 {
			if e.isNik {
				b.WriteString(ghostBubble(ghostTick, ghostLabel) + "\n")
			}
			if endOfGroup {
				b.WriteString(groupLabel(e.lt, e.isNik, width) + "\n")
			}
			if !e.isNik {
				b.WriteString("\n" + ghostBubble(ghostTick, ghostLabel) + "\n")
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

	if len(entries) == 0 && ghostTick >= 0 {
		b.WriteString(ghostBubble(ghostTick, ghostLabel) + "\n")
	}

	// Always reserve space for a ghost bubble at the bottom so a thinking/typing
	// bubble appearing or disappearing doesn't shift the conversation upward.
	// Padding matches the newline count the ghost would add in each position:
	//   empty / nik-last: ghostBubble + "\n"        → 3 newlines
	//   user-last:        "\n" + ghostBubble + "\n" → 4 newlines
	if ghostTick < 0 {
		pad := 3
		if len(entries) > 0 && !entries[len(entries)-1].isNik {
			pad = 4
		}
		b.WriteString(strings.Repeat("\n", pad))
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
	lt            time.Time
	isNik         bool
	body          string
	toolName      string
	toolState     toolState
	platform      string
	externalMsgID string
	reactions     []string
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

func collectReactions(msgs []db.Message) map[string][]string {
	extIDs := make(map[string]bool)
	for i := range msgs {
		if msgs[i].Kind != "reaction" && msgs[i].ExternalMessageID != "" {
			extIDs[msgs[i].ExternalMessageID] = true
		}
	}

	type rkey struct{ target, contact string }
	latest := make(map[rkey]string)
	var order []rkey

	for i := range msgs {
		m := &msgs[i]
		if m.Kind != "reaction" || !m.ContextStanzaID.Valid {
			continue
		}
		target := m.ContextStanzaID.String
		if !extIDs[target] {
			continue
		}
		k := rkey{target, m.ContactID}
		if _, ok := latest[k]; !ok {
			order = append(order, k)
		}
		latest[k] = m.Body
	}

	byTarget := make(map[string][]string)
	for _, k := range order {
		if emoji := latest[k]; emoji != "" {
			byTarget[k.target] = append(byTarget[k.target], emoji)
		}
	}

	for target, emojis := range byTarget {
		counts := make(map[string]int)
		var unique []string
		for _, e := range emojis {
			if counts[e] == 0 {
				unique = append(unique, e)
			}
			counts[e]++
		}

		var deduped []string
		for _, e := range unique {
			if counts[e] > 1 {
				deduped = append(deduped, e+strconv.Itoa(counts[e]))
			} else {
				deduped = append(deduped, e)
			}
		}
		byTarget[target] = deduped
	}

	return byTarget
}

func currentGenesisSeed(msgs []db.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		ext := msgs[i].ExternalMessageID
		if msgs[i].Platform != "system" || !strings.HasPrefix(ext, "genesis:") {
			continue
		}
		return strings.TrimPrefix(ext, "genesis:")
	}
	return ""
}

func computeInputLocked(seed string, activity []string) bool {
	if seed == "" || !genesis.IsInteractive(seed) {
		return true
	}
	return len(activity) > 0
}

func placeholderFor(inputLocked bool, genesisSeed string) string {
	switch {
	case inputLocked:
		return "waiting for nik to finish..."
	case genesisSeed == "first_contact":
		return "introduce yourself to nik"
	default:
		return "message..."
	}
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
