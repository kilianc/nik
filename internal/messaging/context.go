package messaging

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/kciuffolo/nik/internal/db"
)

type SessionContext struct {
	Lines []string
}

func BuildConversationInput(conv db.Conversation, msgs []db.Message, senderLabels map[string]string, session SessionContext) []string {
	lines := []string{"## Session", ""}

	if len(session.Lines) == 0 {
		lines = append(lines,
			fmt.Sprintf("Platform: %s", conv.Platform),
			fmt.Sprintf("Type: %s", conv.Kind),
		)
	} else {
		lines = append(lines, session.Lines...)
	}

	contextMsgs, newMsgs := splitAtReadBoundary(msgs, conv.LastReadAt)

	if len(contextMsgs) > 0 {
		lines = append(lines, "", "### Context (already handled, do NOT act on these)", "")
		lines = append(lines, formatMessagesWithDateSeparators(contextMsgs, senderLabels)...)
	}

	lines = append(lines, "", "### New messages", "")
	lines = append(lines, formatMessagesWithDateSeparators(newMsgs, senderLabels)...)

	return lines
}

func formatMessagesWithDateSeparators(msgs []db.Message, senderLabels map[string]string) []string {
	var lines []string
	lastDate := ""

	for _, msg := range msgs {
		date := msg.SentAt.Format("Jan 2, 2006")
		if date != lastDate {
			lines = append(lines, fmt.Sprintf("--- %s ---", date))
			lastDate = date
		}

		line := FormatMessageLine(msg, senderLabel(msg, senderLabels))
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
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
