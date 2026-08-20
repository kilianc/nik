package api

import (
	"context"
	"net/http"
	"time"
)

// Onboarding is what a client needs to render a nik that is still being born.
//
// Facts only. Whether an input box is locked and what its placeholder says is
// UX, and it belongs in whatever is drawing the box — a terminal and a browser
// will not agree about that for long, and neither should have to ask nikd.
type Onboarding interface {
	State(ctx context.Context) (OnboardingState, error)
}

// OnboardingState is genesis, from outside.
type OnboardingState struct {
	// BornAt is when this nik first started becoming itself. A TUI prints it;
	// it is also the only "how old is this nik" anything has.
	BornAt time.Time `json:"born_at,omitzero"`
	// Completed is monotonic: once genesis is done it stays done.
	Completed bool `json:"completed"`
	// Seed names the genesis step currently playing, empty once it is over.
	// A client uses it to decide whether it is this scene's turn to ask the
	// person something.
	Seed string `json:"seed,omitempty"`
}

func (s *Server) SetOnboarding(onboarding Onboarding) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.onboarding = onboarding
}

func (s *Server) currentOnboarding() Onboarding {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.onboarding
}

func (s *Server) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	onboarding := s.currentOnboarding()
	if onboarding == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	state, err := onboarding.State(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, state)
}
