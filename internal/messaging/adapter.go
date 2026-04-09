package messaging

import "context"

type MessageReceiver interface {
	MessageExists(ctx context.Context, platform, externalMessageID string) (bool, error)
	ReceiveConversation(ctx context.Context, conv Conversation) error
	ReceiveMessage(ctx context.Context, msg InboundMessage) error
	OnHistorySyncComplete(ctx context.Context, platform string) error
}

type MessagingPlatform interface {
	Platform() string
	Start(ctx context.Context, receiver MessageReceiver) error
	Stop(ctx context.Context) error
	Reply(ctx context.Context, externalConversationID string, body string, quote *QuoteTarget) (OutboundMessage, error)
	SendFile(ctx context.Context, externalConversationID string, filePath string, caption string) (OutboundMessage, error)
	SendVoiceNote(ctx context.Context, externalConversationID string, audioPath string) (OutboundMessage, error)
	React(ctx context.Context, externalConversationID string, externalMessageID string, externalSenderID string, emoji string) (OutboundMessage, error)
	SetPresence(ctx context.Context, available bool) error
	StartTyping(ctx context.Context, externalConversationID string) error
	StopTyping(ctx context.Context, externalConversationID string) error
	MarkRead(ctx context.Context, refs []InboundMessage) error
}
