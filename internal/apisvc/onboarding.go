package apisvc

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/genesis"
)

// Onboarding reports how far along genesis is.
type Onboarding struct {
	conn *sql.DB
}

func NewOnboarding(conn *sql.DB) *Onboarding {
	return &Onboarding{conn: conn}
}

// seedWindow is how far back to look for the seed currently playing. Genesis
// scenes are minutes apart at most, so anything older than a page of messages
// is a scene that has already finished.
const seedWindow = 200

func (o *Onboarding) State(ctx context.Context) (api.OnboardingState, error) {
	bornAt, err := genesis.StartedAt(ctx, o.conn)
	if err != nil {
		return api.OnboardingState{}, err
	}

	state := api.OnboardingState{
		BornAt:    bornAt,
		Completed: genesis.IsCompleted(ctx, o.conn),
	}
	if state.Completed {
		// No seed is playing once it is over, and reading the messages to
		// confirm that would be work for an answer that cannot change.
		return state, nil
	}

	messages, err := db.MessageList(ctx, o.conn, db.MessageListParams{
		ConversationID: db.LocalConversationID,
		Limit:          seedWindow,
	})
	if err != nil {
		return state, err
	}

	// MessageList is newest-first and CurrentSeed scans backwards for the
	// latest seed, so it wants oldest-first.
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	state.Seed = genesis.CurrentSeed(messages)

	return state, nil
}
