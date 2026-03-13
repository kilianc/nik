package messaging

import "context"

type MessageReceiver interface {
	ReceiveConversation(ctx context.Context, conv Conversation) error
	ReceiveMessage(ctx context.Context, msg InboundMessage) error
	OnHistorySyncComplete(ctx context.Context, platform string) error
}

type MessagingPlatform interface {
	Platform() string
	Start(ctx context.Context, receiver MessageReceiver) error
	Stop(ctx context.Context) error
	Reply(ctx context.Context, externalConversationID string, body string) (OutboundMessage, error)
	SendImage(ctx context.Context, externalConversationID string, imagePath string, caption string) (OutboundMessage, error)
	SendAudio(ctx context.Context, externalConversationID string, audioPath string, voiceNote bool) (OutboundMessage, error)
	React(ctx context.Context, externalConversationID string, externalMessageID string, externalSenderID string, emoji string) (OutboundMessage, error)
	SetPresence(ctx context.Context, available bool) error
	StartTyping(ctx context.Context, externalConversationID string) error
	StopTyping(ctx context.Context, externalConversationID string) error
	MarkRead(ctx context.Context, refs []InboundMessage) error
}
