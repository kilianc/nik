package whatsapp

import "testing"

func TestNewWaLoggerSubBuildsHierarchicalModule(t *testing.T) {
	root := newWaLogger("whatsmeow")
	rootLogger, ok := root.(*slogWaLog)
	if !ok {
		t.Fatalf("expected *slogWaLog implementation")
	}

	sub := rootLogger.Sub("db")
	subLogger, ok := sub.(*slogWaLog)
	if !ok {
		t.Fatalf("expected *slogWaLog implementation for sub logger")
	}

	if subLogger.mod != "whatsmeow/db" {
		t.Fatalf("expected module path whatsmeow/db, got %q", subLogger.mod)
	}
}
