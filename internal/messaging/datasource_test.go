package messaging

import (
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestNewDataSourceStoresDependencies(t *testing.T) {
	cfg := &config.Config{MaxHistory: 42}
	svc := &Service{}

	ds := NewDataSource(cfg, svc)
	if ds == nil {
		t.Fatalf("expected non-nil data source")
	}
	if ds.cfg != cfg {
		t.Fatalf("expected config pointer to be preserved")
	}
	if ds.svc != svc {
		t.Fatalf("expected service pointer to be preserved")
	}
}
