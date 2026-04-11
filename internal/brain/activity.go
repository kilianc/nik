package brain

import "context"

type Activity interface {
	Busy(ctx context.Context, convID string)
	Done(ctx context.Context, convID string)
}

func (b *Brain) SetActivity(a Activity) {
	b.activity = a
}
