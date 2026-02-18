package brain

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

type stubDataSource struct{}

func (stubDataSource) Check(context.Context) ([]DataSourceOutput, error) {
	return nil, nil
}

func TestRegisterDataSourcePanicsOnNil(t *testing.T) {
	b := New(&config.Config{}, nil)

	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for nil data source")
		}
	}()

	b.RegisterDataSource(nil)
}

func TestRegisterDataSourcesAppendsAll(t *testing.T) {
	b := New(&config.Config{}, nil)

	b.RegisterDataSources(stubDataSource{}, stubDataSource{})
	if len(b.dataSources) != 2 {
		t.Fatalf("expected 2 data sources, got %d", len(b.dataSources))
	}
}
