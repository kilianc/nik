package brain

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
)

type stubSensor struct{}

func (stubSensor) Check(context.Context) ([]Stimulus, error) { return nil, nil }
func (stubSensor) Read(context.Context, string) string       { return "" }

func TestSetSensorPanicsOnNil(t *testing.T) {
	b := New(&config.Config{}, nil)

	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for nil sensor")
		}
	}()

	b.SetSensor(nil)
}

func TestRegisterReflex(t *testing.T) {
	t.Run("throttles by interval", func(t *testing.T) {
		b := New(&config.Config{}, nil)

		calls := 0
		b.RegisterReflex(50*time.Millisecond, func(ctx context.Context) {
			calls++
		})

		ctx := context.Background()
		for range 5 {
			b.reflexes[0](ctx)
		}

		if calls != 1 {
			t.Fatalf("expected 1 call during throttle window, got %d", calls)
		}

		time.Sleep(60 * time.Millisecond)
		b.reflexes[0](ctx)

		if calls != 2 {
			t.Fatalf("expected 2 calls after interval elapsed, got %d", calls)
		}
	})

	t.Run("zero interval runs every tick", func(t *testing.T) {
		b := New(&config.Config{}, nil)

		calls := 0
		b.RegisterReflex(0, func(ctx context.Context) {
			calls++
		})

		ctx := context.Background()
		for range 5 {
			b.reflexes[0](ctx)
		}

		if calls != 5 {
			t.Fatalf("expected 5 calls with zero interval, got %d", calls)
		}
	})
}
