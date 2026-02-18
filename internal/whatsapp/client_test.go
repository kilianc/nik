package whatsapp

import "testing"

func TestSelfJIDReturnsEmptyWithoutClientState(t *testing.T) {
	c := &Client{}
	if got := c.SelfJID(); got != "" {
		t.Fatalf("expected empty self jid, got %q", got)
	}
}
