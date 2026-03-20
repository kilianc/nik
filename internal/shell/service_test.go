package shell

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestCheckSessionsNilConnNoops(t *testing.T) {
	svc := NewService(&config.Config{Home: "/tmp/test"}, nil)
	svc.CheckSessions(context.Background())
}
