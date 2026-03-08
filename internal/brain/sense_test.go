package brain

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

type stubSensor struct{}

func (stubSensor) Check(context.Context) ([]Stimulus, error) { return nil, nil }
func (stubSensor) Get(context.Context, string) string        { return "" }

func TestSetSensorPanicsOnNil(t *testing.T) {
	b := New(&config.Config{}, nil)

	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for nil sensor")
		}
	}()

	b.SetSensor(nil)
}

func TestSetSensorStores(t *testing.T) {
	b := New(&config.Config{}, nil)

	b.SetSensor(stubSensor{})
	if b.sensor == nil {
		t.Fatalf("expected sensor to be set")
	}
}
