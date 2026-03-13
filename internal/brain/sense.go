package brain

import (
	"context"
	"time"
)

type Stimulus struct {
	Meta map[string]string
}

type Sensor interface {
	Check(ctx context.Context) ([]Stimulus, error)
	Get(ctx context.Context, convID string) string
}

func (b *Brain) SetSensor(s Sensor) {
	if s == nil {
		panic("set sensor: nil sensor")
	}
	b.sensor = s
}

type Reflex func(ctx context.Context)

func (b *Brain) RegisterReflex(every time.Duration, r Reflex) {
	if every > 0 {
		var last time.Time
		orig := r
		r = func(ctx context.Context) {
			if time.Since(last) < every {
				return
			}
			last = time.Now()
			orig(ctx)
		}
	}

	b.reflexes = append(b.reflexes, r)
}
