package timeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/task"
)

type Timeline struct {
	cfg      *config.Config
	msgSvc   *messaging.Service
	taskSvc  *task.Service
	alarmSvc *alarms.Service
}

func New(cfg *config.Config, msgSvc *messaging.Service, taskSvc *task.Service, alarmSvc *alarms.Service) *Timeline {
	return &Timeline{
		cfg:      cfg,
		msgSvc:   msgSvc,
		taskSvc:  taskSvc,
		alarmSvc: alarmSvc,
	}
}

func (t *Timeline) Check(ctx context.Context) ([]brain.Stimulus, error) {
	var stimuli []brain.Stimulus

	for _, convID := range t.cfg.AllowConversationIDs {
		s, ok, err := t.check(ctx, convID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			slog.Warn("check stimulus", "pkg", "timeline", "conversation_id", convID, "error", err)
			continue
		}
		if ok {
			t.markRead(ctx, convID)
			stimuli = append(stimuli, s)
		}
	}

	return stimuli, nil
}

func (t *Timeline) Get(ctx context.Context, convID string) string {
	maxHistory := t.cfg.MaxHistory
	if maxHistory == 0 {
		maxHistory = 20
	}

	conv, msgs, err := t.msgSvc.ConversationWithMessages(ctx, convID, maxHistory)
	if err != nil {
		slog.Warn("timeline get", "pkg", "timeline", "conversation_id", convID, "error", err)
		return ""
	}

	var readLine time.Time
	if conv.LastReadAt.Valid {
		readLine = conv.LastReadAt.Time
	}

	var since time.Time
	if len(msgs) > 0 {
		since = msgs[0].SentAt
	}

	senderLabels := t.msgSvc.SenderLabels(ctx, msgs)
	session := t.msgSvc.ConversationHeader(ctx, conv)
	entries := t.buildEntries(ctx, convID, since, msgs, senderLabels)

	var lines []string
	lines = append(lines, "## Session", "")
	lines = append(lines, session.Lines...)
	lines = append(lines, renderTimeline(entries, readLine)...)

	return strings.Join(lines, "\n")
}

// Render returns the session header and rendered timeline for a conversation,
// suitable for debug/CLI output.
func (t *Timeline) Render(ctx context.Context, convID string) (session []string, rendered []string, err error) {
	conv, msgs, err := t.msgSvc.ConversationWithMessages(ctx, convID, t.cfg.MaxHistory)
	if err != nil {
		return nil, nil, err
	}

	var readLine time.Time
	if conv.LastReadAt.Valid {
		readLine = conv.LastReadAt.Time
	}

	var since time.Time
	if len(msgs) > 0 {
		since = msgs[0].SentAt
	}

	senderLabels := t.msgSvc.SenderLabels(ctx, msgs)
	header := t.msgSvc.ConversationHeader(ctx, conv)
	entries := t.buildEntries(ctx, convID, since, msgs, senderLabels)

	return header.Lines, renderTimeline(entries, readLine), nil
}

// check determines whether a conversation has new events worth activating on.
func (t *Timeline) check(ctx context.Context, convID string) (brain.Stimulus, bool, error) {
	maxHistory := t.cfg.MaxHistory
	if maxHistory == 0 {
		maxHistory = 20
	}

	conv, msgs, err := t.msgSvc.ConversationWithMessages(ctx, convID, maxHistory)
	if err != nil {
		return brain.Stimulus{}, false, err
	}

	var readLine time.Time
	if conv.LastReadAt.Valid {
		readLine = conv.LastReadAt.Time
	}

	var since time.Time
	if len(msgs) > 0 {
		since = msgs[0].SentAt
	}

	senderLabels := t.msgSvc.SenderLabels(ctx, msgs)
	entries := t.buildEntries(ctx, convID, since, msgs, senderLabels)

	hasNew := false
	for _, e := range entries {
		if readLine.IsZero() || e.at.After(readLine) {
			hasNew = true
			break
		}
	}
	if !hasNew {
		return brain.Stimulus{}, false, nil
	}

	var reactToID string
	for i := len(msgs) - 1; i >= 0; i-- {
		if !msgs[i].IsFromMe {
			reactToID = msgs[i].ID
			break
		}
	}

	hasTasks := false
	hasAlarms := false
	for _, e := range entries {
		switch e.from {
		case "task":
			hasTasks = true
		case "alarm":
			hasAlarms = true
		}
	}
	sources := buildSources(len(msgs) > 0, hasTasks, hasAlarms)

	meta := map[string]string{
		"conversation_id": convID,
		"sources":         sources,
	}
	if reactToID != "" {
		meta["react_to_message_id"] = reactToID
	}

	return brain.Stimulus{Meta: meta}, true, nil
}

func (t *Timeline) markRead(ctx context.Context, convID string) {
	now := time.Now().UTC()

	err := t.msgSvc.MarkRead(ctx, convID, now)
	if err != nil {
		slog.Warn("mark conversation read", "pkg", "timeline", "conversation_id", convID, "error", err)
	}
}

// entry is a timeline event with a timestamp, sender label, and formatted text.
type entry struct {
	at   time.Time
	from string
	text string
}

func (t *Timeline) buildEntries(ctx context.Context, convID string, since time.Time, msgs []db.Message, senderLabels map[string]string) []entry {
	var entries []entry

	extIDToMsg := make(map[string]db.Message, len(msgs))
	for _, msg := range msgs {
		extIDToMsg[msg.ExternalMessageID] = msg
	}

	for _, msg := range msgs {
		entries = append(entries, messageEntry(msg, senderLabels[msg.ID], extIDToMsg))
	}

	reports, err := t.taskSvc.ListReports(ctx, convID, since)
	if err != nil {
		slog.Warn("task reports", "pkg", "timeline", "conversation_id", convID, "error", err)
	}
	for _, r := range reports {
		entries = append(entries, reportEntry(r))
	}

	spawned, err := t.taskSvc.ListSpawned(ctx, convID, since)
	if err != nil {
		slog.Warn("task spawned", "pkg", "timeline", "conversation_id", convID, "error", err)
	}
	for _, s := range spawned {
		if s.RetryForTaskID.Valid && s.RetryNumber > 0 {
			entries = append(entries, taskRetryEntry(s))
		} else {
			entries = append(entries, taskSpawnedEntry(s))
		}
	}

	cancelled, err := t.taskSvc.ListCancelled(ctx, convID, since)
	if err != nil {
		slog.Warn("task cancelled", "pkg", "timeline", "conversation_id", convID, "error", err)
	}
	for _, c := range cancelled {
		entries = append(entries, taskCancelledEntry(c))
	}

	occurrences, err := t.alarmSvc.ListOccurrences(ctx, convID, since)
	if err != nil {
		slog.Warn("alarm occurrences", "pkg", "timeline", "conversation_id", convID, "error", err)
	}
	for _, o := range occurrences {
		entries = append(entries, occurrenceEntry(o))
	}

	createdAlarms, err := t.alarmSvc.ListCreated(ctx, convID, since)
	if err != nil {
		slog.Warn("alarm created", "pkg", "timeline", "conversation_id", convID, "error", err)
	}
	for _, a := range createdAlarms {
		entries = append(entries, alarmCreatedEntry(a))
	}

	return entries
}

func renderTimeline(entries []entry, readLine time.Time) []string {
	if len(entries) == 0 {
		return nil
	}

	sorted := make([]entry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].at.Before(sorted[j].at)
	})

	var handled, fresh []entry
	if readLine.IsZero() {
		fresh = sorted
	} else {
		for i, e := range sorted {
			if e.at.After(readLine) {
				handled = sorted[:i]
				fresh = sorted[i:]
				break
			}
		}
		if len(fresh) == 0 && len(handled) == 0 {
			handled = sorted
		}
	}

	var lines []string

	if len(handled) > 0 {
		lines = append(lines, "### Already handled", "")
		lines = append(lines, renderEntries(handled)...)
		lines = append(lines, "")
	}

	if len(fresh) > 0 {
		lines = append(lines, "### New", "")
		lines = append(lines, renderEntries(fresh)...)
	}

	return lines
}

func renderEntries(entries []entry) []string {
	var lines []string
	lastDate := ""

	for _, e := range entries {
		date := e.at.Format("Jan 2, 2006")
		if date != lastDate {
			lines = append(lines, fmt.Sprintf("--- %s ---", date))
			lastDate = date
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", e.at.Format("15:04:05"), e.from, e.text))
	}

	return lines
}

func messageEntry(msg db.Message, sender string, extIDToMsg map[string]db.Message) entry {
	if msg.IsFromMe {
		sender = "YOU"
	} else if sender == "" {
		panic("message " + msg.ID + " has no sender label")
	}

	text := messaging.FormatMessageText(msg)

	if msg.Kind == "reaction" && msg.ContextStanzaID.Valid {
		if target, ok := extIDToMsg[msg.ContextStanzaID.String]; ok {
			text += " to " + reactionTargetSnippet(target)
		}
	}

	return entry{
		at:   msg.SentAt,
		from: sender,
		text: text,
	}
}

const reactionTargetTruncateLen = 50

func reactionTargetSnippet(msg db.Message) string {
	body := strings.TrimSpace(msg.Body)

	label := id.Shorten(msg.ID)

	if body == "" {
		return label + " (" + msg.Kind + ")"
	}

	if len(body) > reactionTargetTruncateLen {
		body = body[:reactionTargetTruncateLen] + "…"
	}

	return label + ` "` + body + `"`
}

const (
	pad               = "           " // 11 spaces — width of [HH:MM:SS] + space
	reportTruncateLen = 200
)

func padLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	joined := strings.Join(lines, "\n")
	parts := strings.Split(joined, "\n")

	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		b.WriteByte('\n')
		b.WriteString(pad)
		b.WriteString(p)
	}
	return b.String()
}

func reportEntry(r db.TaskReport) entry {
	content := r.Content
	if len(content) > reportTruncateLen {
		content = content[:reportTruncateLen] + " [truncated]"
	}

	lines := []string{
		"[Task report]",
		"task_id: " + id.Shorten(r.TaskID),
		"goal: " + r.Goal,
		"status: " + r.Status,
		"report: " + content,
	}

	return entry{
		at:   r.CreatedAt,
		from: "task",
		text: padLines(lines),
	}
}

func occurrenceEntry(o db.AlarmOccurrence) entry {
	recurring := o.Recurrence.Valid && o.Recurrence.String != ""

	var header string
	if recurring {
		header = "[Recurring alarm fired]"
	} else {
		header = "[One-off alarm fired]"
	}

	lines := []string{
		header,
		"alarm_id: " + id.Shorten(o.AlarmID),
		"goal: " + o.Goal,
	}
	if recurring {
		lines = append(lines, "recurrence: "+o.Recurrence.String)
	}
	if o.Note.Valid && o.Note.String != "" {
		lines = append(lines, "last_time: "+o.Note.String)
	}

	lines = append(lines, "MANDATORY: if you already handled this alarm, move on. If you are handling this alarm now, load the alarm skill and follow all instructions meticulously.")

	return entry{
		at:   o.FiredAt,
		from: "alarm",
		text: padLines(lines),
	}
}

func taskSpawnedEntry(s db.TaskSpawned) entry {
	lines := []string{
		"[Task spawned]",
		"task_id: " + id.Shorten(s.ID),
		"goal: " + s.Goal,
	}
	if s.CrewMemberName.Valid {
		lines = append(lines, "assigned: "+s.CrewMemberName.String)
	}

	return entry{
		at:   s.CreatedAt,
		from: "system",
		text: padLines(lines),
	}
}

func taskRetryEntry(s db.TaskSpawned) entry {
	lines := []string{
		"[Task retry #" + strconv.Itoa(s.RetryNumber) + " spawned]",
		"task_id: " + id.Shorten(s.ID),
		"retry_of: " + id.Shorten(s.RetryForTaskID.String),
		"goal: " + s.Goal,
	}
	if s.CrewMemberName.Valid {
		lines = append(lines, "assigned: "+s.CrewMemberName.String)
	}

	return entry{
		at:   s.CreatedAt,
		from: "system",
		text: padLines(lines),
	}
}

func taskCancelledEntry(c db.TaskCancelled) entry {
	lines := []string{
		"[Task cancelled]",
		"task_id: " + id.Shorten(c.ID),
		"goal: " + c.Goal,
	}

	return entry{
		at:   c.CompletedAt,
		from: "system",
		text: padLines(lines),
	}
}

func alarmCreatedEntry(a db.Alarm) entry {
	recurring := a.Recurrence.Valid && a.Recurrence.String != ""

	lines := []string{
		"[Alarm created]",
		"alarm_id: " + id.Shorten(a.ID),
		"goal: " + a.Goal,
	}
	if recurring {
		lines = append(lines, "recurrence: "+a.Recurrence.String)
	}
	if a.NextFireAt.Valid {
		lines = append(lines, "fires_at: "+a.NextFireAt.Time.Format("Jan 2, 2006 3:04 PM"))
	}

	return entry{
		at:   a.CreatedAt,
		from: "system",
		text: padLines(lines),
	}
}

func buildSources(hasMessages, hasReports, hasOccurrences bool) string {
	var sources []string
	if hasMessages {
		sources = append(sources, `"message"`)
	}
	if hasReports {
		sources = append(sources, `"task"`)
	}
	if hasOccurrences {
		sources = append(sources, `"alarm"`)
	}
	if len(sources) == 0 {
		return "[]"
	}
	return "[" + strings.Join(sources, ",") + "]"
}
