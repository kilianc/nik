package shell

import (
	"context"
	"testing"
)

func TestCheckSessionsNilConnNoops(t *testing.T) {
	svc := NewService(nil, "/tmp/test")
	svc.CheckSessions(context.Background())
}
