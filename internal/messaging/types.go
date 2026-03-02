package messaging

import "time"

type Conversation struct {
	Platform               string
	ExternalConversationID string
	Kind                   string
	Title                  string
	Topic                  *string
	IsAnnounce             *bool
	IsLocked               *bool
	OwnerExternalID        *string
	ParticipantExternalIDs []string
	LastMessageAt          time.Time
}

type InboundMessage struct {
	Platform               string
	ExternalConversationID string
	ExternalMessageID      string
	ExternalSenderID       string
	ExternalSenderIDs      []string
	Kind                   string
	Body                   string
	MimeType               string
	SentAt                 time.Time
	IsFromMe               bool
	IsGroup                bool
	IsEdit                 bool
	EditTargetMessageID    string
	ContextStanzaID        string
	ContextParticipant     string
	ContextIsForwarded     bool
	ContextForwardingScore *int32
	ContextMentionedIDs    []string
	IsEphemeral            bool
	IsViewOnce             bool
	LocalPath              string
	MediaHash              string
	MediaSizeBytes         int64
}

type OutboundMessage struct {
	ExternalMessageID string
	ExternalSenderID  string
	SentAt            time.Time
	Kind              string
	Body              string
	MimeType          string
	LocalPath         string
}
