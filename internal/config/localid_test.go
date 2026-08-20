package config

import (
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

// The production code copies this constant instead of importing db, so that
// nikctl does not link SQLite for the sake of one UUID. This test is the other
// half of that bargain: importing db here is free, since a test binary's
// dependencies are nobody's shipping binary.
func TestLocalConversationIDMatchesTheDatabase(t *testing.T) {
	if localConversationID != db.LocalConversationID {
		t.Fatalf("config says %q, db says %q — normalize would stop making the local conversation privileged",
			localConversationID, db.LocalConversationID)
	}
}
