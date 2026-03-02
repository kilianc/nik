package messaging

import (
	"context"
	"testing"
	"time"
)

type fakeReceiver struct{}

func (fakeReceiver) ReceiveConversation(context.Context, Conversation) error { return nil }
func (fakeReceiver) ReceiveMessage(context.Context, InboundMessage) error    { return nil }
func (fakeReceiver) OnHistorySyncComplete(context.Context, string) error     { return nil }

type fakePlatformAdapter struct {
	name string
}

func (f *fakePlatformAdapter) Platform() string { return f.name }
func (f *fakePlatformAdapter) Start(context.Context, MessageReceiver) error {
	return nil
}
func (f *fakePlatformAdapter) Stop(context.Context) error { return nil }
func (f *fakePlatformAdapter) Reply(context.Context, string, string) (OutboundMessage, error) {
	return OutboundMessage{}, nil
}
func (f *fakePlatformAdapter) SendImage(context.Context, string, string, string) (OutboundMessage, error) {
	return OutboundMessage{}, nil
}
func (f *fakePlatformAdapter) React(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakePlatformAdapter) SetPresence(context.Context, bool) error { return nil }
func (f *fakePlatformAdapter) StartTyping(context.Context, string) error {
	return nil
}
func (f *fakePlatformAdapter) StopTyping(context.Context, string) error { return nil }
func (f *fakePlatformAdapter) MarkRead(context.Context, []InboundMessage) error {
	return nil
}

func TestAdapterContractsCompileAndExposePlatformName(t *testing.T) {
	var _ MessageReceiver = (*fakeReceiver)(nil)
	var _ MessagingPlatform = (*fakePlatformAdapter)(nil)

	p := &fakePlatformAdapter{name: "whatsapp"}
	if p.Platform() != "whatsapp" {
		t.Fatalf("expected platform name whatsapp, got %q", p.Platform())
	}
}

func TestOutboundMessageCarriesTimestamp(t *testing.T) {
	now := time.Now()
	outbound := OutboundMessage{SentAt: now}
	if outbound.SentAt != now {
		t.Fatalf("expected outbound timestamp to round-trip")
	}
}
