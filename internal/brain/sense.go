package brain

import "context"

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

func (b *Brain) RegisterReflex(r Reflex) {
	b.reflexes = append(b.reflexes, r)
}
