package timeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

type Timeline struct {
	cfg    *config.Config
	msgSvc *messaging.Service
}

func New(cfg *config.Config, msgSvc *messaging.Service) *Timeline {
	return &Timeline{
		cfg:    cfg,
		msgSvc: msgSvc,
	}
}

func (t *Timeline) Check(ctx context.Context) ([]brain.Stimulus, error) {
	var stimuli []brain.Stimulus

	for _, convID := range t.cfg.AllowedIDs() {
		s, ok, err := t.check(ctx, convID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			slog.Warn("check stimulus", "pkg", "timeline", "conversation_id", convID, "error", err)
			continue
		}
		if ok {
			stimuli = append(stimuli, s)
		}
	}

	return stimuli, nil
}

func (t *Timeline) Read(ctx context.Context, convID string) string {
	return t.read(ctx, convID, readOpts{markRead: true})
}

func (t *Timeline) Peek(ctx context.Context, convID string) string {
	return t.read(ctx, convID, readOpts{skipSystem: true})
}

type readOpts struct {
	skipSystem bool
	markRead   bool
}

func (t *Timeline) read(ctx context.Context, convID string, opts readOpts) string {
	maxHistory := t.cfg.MaxHistory
	if maxHistory == 0 {
		maxHistory = 20
	}

	conv, msgs, err := t.msgSvc.ConversationWithMessages(ctx, convID, maxHistory)
	if err != nil {
		slog.Warn("timeline read", "pkg", "timeline", "conversation_id", convID, "error", err)
		return ""
	}

	if opts.skipSystem {
		filtered := msgs[:0]
		for _, m := range msgs {
			if m.Platform != "system" {
				filtered = append(filtered, m)
			}
		}
		msgs = filtered
	}

	cutoff := time.Now().UTC().Add(-t.cfg.SystemMessageMaxAgeOrDefault())
	filtered := msgs[:0]
	for _, m := range msgs {
		if m.Platform == "system" && m.SentAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, m)
	}
	msgs = filtered

	senderLabels := t.msgSvc.SenderLabels(ctx, msgs)
	session := t.msgSvc.ConversationHeader(ctx, conv)
	entries := t.buildEntries(msgs, senderLabels)

	var lines []string
	lines = append(lines, "## Conversation", "")
	lines = append(lines, session.Lines...)
	lines = append(lines, "")
	lines = append(lines, renderTimeline(entries)...)

	if opts.markRead {
		t.markRead(ctx, convID, msgs)
	}

	return strings.Join(lines, "\n")
}

// Render returns the session header and rendered timeline for a conversation,
// suitable for debug/CLI output.
func (t *Timeline) Render(ctx context.Context, convID string) (session []string, rendered []string, err error) {
	conv, msgs, err := t.msgSvc.ConversationWithMessages(ctx, convID, t.cfg.MaxHistory)
	if err != nil {
		return nil, nil, err
	}

	senderLabels := t.msgSvc.SenderLabels(ctx, msgs)
	header := t.msgSvc.ConversationHeader(ctx, conv)
	entries := t.buildEntries(msgs, senderLabels)

	return header.Lines, renderTimeline(entries), nil
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

	hasActionable := false
	for _, m := range msgs {
		if !readLine.IsZero() && !m.SentAt.After(readLine) {
			continue
		}
		if isNikAuthored(m) {
			continue
		}
		hasActionable = true
		break
	}
	if !hasActionable {
		return brain.Stimulus{}, false, nil
	}

	meta := map[string]string{
		"conversation_id": convID,
		"sources":         buildSourcesFromMessages(msgs),
	}

	return brain.Stimulus{Meta: meta}, true, nil
}

// isNikAuthored reports whether msg was produced by nik itself during an
// activation (outbound message echoes and tool_call records). These entries
// carry nothing new for the model -- it already saw the output when it chose
// to act. Skipping activation on nik-only new entries saves a wasted round
// trip that would only end in done.
func isNikAuthored(msg db.Message) bool {
	if msg.Platform == "system" {
		return msg.Kind == "tool_call" || msg.Kind == "tool_call_start"
	}
	return msg.IsFromMe
}

func (t *Timeline) markRead(ctx context.Context, convID string, msgs []db.Message) {
	if len(msgs) == 0 {
		return
	}

	err := t.msgSvc.MarkRead(ctx, convID, msgs[len(msgs)-1].SentAt)
	if err != nil {
		slog.Warn("mark conversation read", "pkg", "timeline", "conversation_id", convID, "error", err)
	}
}

func buildSourcesFromMessages(msgs []db.Message) string {
	set := map[string]bool{}

	for _, msg := range msgs {
		if msg.Platform != "system" {
			set["message"] = true
			continue
		}

		switch {
		case msg.Kind == "task_report", strings.HasPrefix(msg.Kind, "task_"):
			set["task"] = true
		case strings.HasPrefix(msg.Kind, "alarm_"):
			set["alarm"] = true
		case strings.HasPrefix(msg.Kind, "skill_"):
			set["skill"] = true
		default:
			set["system"] = true
		}
	}

	var sources []string
	for k := range set {
		sources = append(sources, `"`+k+`"`)
	}
	sort.Strings(sources)

	if len(sources) == 0 {
		return "[]"
	}

	return "[" + strings.Join(sources, ",") + "]"
}

type entry struct {
	at   time.Time
	from string
	text string
}

func (t *Timeline) buildEntries(msgs []db.Message, senderLabels map[string]string) []entry {
	var entries []entry

	for _, msg := range msgs {
		e := messageEntry(msg, senderLabels[msg.ID], t.msgSvc.DB())
		if e.text == "" && e.from == "" {
			continue
		}
		entries = append(entries, e)
	}

	return entries
}

func renderTimeline(entries []entry) []string {
	if len(entries) == 0 {
		return nil
	}

	sorted := make([]entry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].at.Before(sorted[j].at)
	})

	return renderEntries(sorted)
}

func renderEntries(entries []entry) []string {
	var lines []string
	lastDate := ""

	for _, e := range entries {
		date := e.at.Format("Jan 2, 2006")
		if date != lastDate {
			lines = append(lines, fmt.Sprintf("--- %s ---", date), "")
			lastDate = date
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", e.at.Format("15:04:05"), e.from, e.text))
		lines = append(lines, "")
	}

	return lines
}

func messageEntry(msg db.Message, sender string, database *sql.DB) entry {
	if msg.IsRedacted {
		return entry{
			at:   msg.SentAt,
			from: "system",
			text: "[message redacted]",
		}
	}

	if msg.Platform == "system" {
		if msg.Kind == "tool_call_start" {
			return entry{}
		}
		if msg.Kind == "media_processed" {
			return renderMediaProcessed(msg, database)
		}
		return renderSystemMessage(msg)
	}

	if msg.IsFromMe {
		sender = "YOU"
	} else if sender == "" {
		// should be impossible — senderLabels is built from the same msg slice
		panic("message " + msg.ID + " has no sender label")
	}

	text := messaging.FormatMessageText(msg)

	if len(msg.ContextMentionedIDs) > 0 {
		names := resolveMentionNames(msg.ContextMentionedIDs, database)
		if len(names) > 0 {
			text += " (mentioning " + strings.Join(names, ", ") + ")"
		}
	}

	if msg.ContextStanzaID.Valid && database != nil {
		target, err := db.MessageGet(context.Background(), database, db.MessageGetParams{
			Platform:          msg.Platform,
			ExternalMessageID: msg.ContextStanzaID.String,
		})
		if err == nil {
			verb := "quote replying to "
			if msg.Kind == "reaction" {
				verb = "reacting to "
			}
			targetSender := resolveContactName(context.Background(), database, target)
			text += " (" + verb + targetSnippet(target, targetSender) + ")"
		}
	}

	if msg.IsEdit && msg.EditTargetMessageID.Valid && database != nil {
		target, err := db.MessageGet(context.Background(), database, db.MessageGetParams{
			Platform:          msg.Platform,
			ExternalMessageID: msg.EditTargetMessageID.String,
		})
		if err == nil {
			targetSender := resolveContactName(context.Background(), database, target)
			text += " (edit of " + targetSnippet(target, targetSender) + ")"
		}
	}

	return entry{
		at:   msg.SentAt,
		from: sender,
		text: text,
	}
}

const targetSnippetTruncateLen = 200

func targetSnippet(msg db.Message, sender string) string {
	if msg.IsRedacted {
		return "[message redacted]"
	}

	ts := msg.SentAt.Format("15:04:05")
	body := strings.TrimSpace(msg.Body)

	prefix := "[" + ts + "] " + sender + ": "

	if body == "" {
		return prefix + "(" + msg.Kind + ")"
	}

	if len(body) > targetSnippetTruncateLen {
		body = body[:targetSnippetTruncateLen] + "…"
	}

	return prefix + body
}

func resolveContactName(ctx context.Context, database *sql.DB, msg db.Message) string {
	if msg.IsFromMe {
		return "YOU"
	}

	if msg.ContactID == "" {
		return "unknown"
	}

	contact, err := db.ContactGet(ctx, database, msg.ContactID)
	if err != nil {
		return "unknown"
	}

	name := strings.TrimSpace(contact.Name)
	if name != "" {
		return name
	}

	return "unknown"
}

func resolveMentionNames(mentionedIDs []string, database *sql.DB) []string {
	if database == nil {
		return nil
	}

	var names []string

	for _, jid := range mentionedIDs {
		contact, err := db.ContactGet(context.Background(), database, jid)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(contact.Name)
		if name == "" {
			continue
		}

		names = append(names, name)
	}

	return names
}
