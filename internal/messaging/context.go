package messaging

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

type SessionContext struct {
	Lines []string
}

func BuildConversationInput(conversationID string, conv db.Conversation, msgs []db.Message, senderLabels map[string]string, session SessionContext, tasks []db.ActiveTask) []string {
	var currentTasks, otherTasks []db.ActiveTask
	for _, t := range tasks {
		if t.ConversationID == conversationID {
			currentTasks = append(currentTasks, t)
		} else {
			otherTasks = append(otherTasks, t)
		}
	}

	lines := []string{"## Session", ""}

	if len(session.Lines) == 0 {
		lines = append(lines,
			fmt.Sprintf("Platform: %s", conv.Platform),
			fmt.Sprintf("Type: %s", conv.Kind),
		)
	} else {
		lines = append(lines, session.Lines...)
	}

	if len(otherTasks) > 0 {
		lines = append(lines, "", "## Other active tasks", "")
		for _, t := range otherTasks {
			lines = append(lines, formatTaskLine(t))
		}
	}

	contextMsgs, newMsgs := splitAtReadBoundary(msgs, conv.LastReadAt)

	if len(contextMsgs) > 0 {
		lines = append(lines, "", "### Context (already handled, do NOT act on these)", "")
		lines = append(lines, formatTimelineWithDateSeparators(contextMsgs, senderLabels, currentTasks)...)
	}

	lines = append(lines, "", "### New messages", "")
	lines = append(lines, formatTimelineWithDateSeparators(newMsgs, senderLabels, currentTasks)...)

	return lines
}

// timelineEntry is a message or task, sorted by timestamp.
type timelineEntry struct {
	at   time.Time
	line string
}

func formatTimelineWithDateSeparators(msgs []db.Message, senderLabels map[string]string, tasks []db.ActiveTask) []string {
	if len(msgs) == 0 {
		return nil
	}

	start := msgs[0].SentAt
	var end time.Time
	if len(msgs) > 0 {
		end = msgs[len(msgs)-1].SentAt
	}

	var entries []timelineEntry

	for _, msg := range msgs {
		line := FormatMessageLine(msg, senderLabel(msg, senderLabels))
		if line != "" {
			entries = append(entries, timelineEntry{at: msg.SentAt, line: line})
		}
	}

	for _, t := range tasks {
		if t.CreatedAt.Before(start) || t.CreatedAt.After(end) {
			continue
		}
		entries = append(entries, timelineEntry{at: t.CreatedAt, line: formatTaskLine(t)})
	}

	sortTimeline(entries)

	var lines []string
	lastDate := ""
	for _, e := range entries {
		date := e.at.Format("Jan 2, 2006")
		if date != lastDate {
			lines = append(lines, fmt.Sprintf("--- %s ---", date))
			lastDate = date
		}
		lines = append(lines, e.line)
	}

	// active tasks that fall after the last message still need to show
	for _, t := range tasks {
		if !t.CreatedAt.After(end) {
			continue
		}
		lines = append(lines, formatTaskLine(t))
	}

	return lines
}

func formatTaskLine(t db.ActiveTask) string {
	ts := t.CreatedAt.Format("15:04:05")
	retry := ""
	if t.RetryNumber > 0 {
		retry = fmt.Sprintf(" (retry %d)", t.RetryNumber)
	}
	return fmt.Sprintf("[%s] ⚙️ task %s — %s (%s%s)", ts, t.Goal, t.ID, t.Status, retry)
}

func sortTimeline(entries []timelineEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].at.Before(entries[j-1].at); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func FormatMessageLine(msg db.Message, sender string) string {
	if sender == "" {
		sender = "[missing-contact]"
	}

	text := strings.TrimSpace(msg.Body)
	if text == "" {
		text = "(" + msg.Kind + ")"
	}

	if msg.Kind == "reaction" {
		text = fmt.Sprintf("reaction %s", text)
	}

	if msg.IsEdit {
		text = fmt.Sprintf("edited: %s", text)
	}

	var extras []string
	if msg.MediaLocalPath.Valid && msg.MediaLocalPath.String != "" {
		extras = append(extras, "media="+msg.MediaLocalPath.String)
	}
	if msg.MediaDescribeText.Valid && msg.MediaDescribeText.String != "" {
		extras = append(extras, "media_description="+msg.MediaDescribeText.String)
	}
	if msg.MediaTranscriptText.Valid && msg.MediaTranscriptText.String != "" {
		extras = append(extras, "media_transcript="+msg.MediaTranscriptText.String)
	}

	if isMediaKind(msg.Kind) {
		hasPath := msg.MediaLocalPath.Valid && msg.MediaLocalPath.String != ""
		hasDesc := (msg.MediaDescribeText.Valid && msg.MediaDescribeText.String != "") ||
			(msg.MediaTranscriptText.Valid && msg.MediaTranscriptText.String != "")
		if !hasPath && !hasDesc {
			extras = append(extras, "media_unavailable")
		}
	}

	if len(extras) > 0 {
		text = text + " | " + strings.Join(extras, " | ")
	}

	ts := msg.SentAt.Format("15:04:05")
	return fmt.Sprintf("[%s] %s: %s", ts, sender, text)
}

func senderLabel(msg db.Message, labels map[string]string) string {
	if labels == nil {
		return "[missing-contact]"
	}

	label, ok := labels[msg.ID]
	if ok && label != "" {
		return label
	}

	return "[missing-contact]"
}

func isMediaKind(kind string) bool {
	switch kind {
	case "audio", "image", "video", "ptv", "document", "sticker":
		return true
	}
	return false
}

func splitAtReadBoundary(msgs []db.Message, lastReadAt sql.NullTime) (contextMsgs, unreadMsgs []db.Message) {
	if !lastReadAt.Valid {
		return nil, msgs
	}

	for i, msg := range msgs {
		if msg.SentAt.After(lastReadAt.Time) {
			return msgs[:i], msgs[i:]
		}
	}

	return msgs, nil
}
