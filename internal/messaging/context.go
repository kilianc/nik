package messaging

import (
	"fmt"
	"strings"

	"github.com/kciuffolo/nik/internal/db"
)

type ConversationHeader struct {
	Lines []string
}

func FormatMessageText(msg db.Message) string {
	text := strings.TrimSpace(msg.Body)

	if msg.Kind == "reaction" {
		if text == "" {
			text = "(removed reaction)"
		} else {
			text = "(" + text + ")"
		}
	} else if text == "" {
		text = "(" + msg.Kind + ")"
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

	return text
}

func formatMessageLine(msg db.Message, sender string) string {
	if sender == "" {
		sender = "[missing-contact]"
	}

	return fmt.Sprintf("[%s] %s: %s", msg.SentAt.Format("15:04:05"), sender, FormatMessageText(msg))
}

func isMediaKind(kind string) bool {
	switch kind {
	case "audio", "image", "video", "ptv", "document", "sticker":
		return true
	}
	return false
}
