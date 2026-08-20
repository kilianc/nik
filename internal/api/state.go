package api

import (
	"sync"
	"time"
)

// State is what /v1/health reports, and nikd fills it in as it converges.
//
// It exists because liveness has been the wrong question. Today the only
// signal anything has is that a process is up and a socket is open — a nik
// with no model credentials, a locked database or a dead sandbox looks
// exactly like a healthy one from outside. Every field here is something that
// can be true or false independently, and Degraded is the list a dashboard
// renders instead of guessing.
//
// Zero value is usable: everything false, which is what a daemon that has
// just started actually knows about itself.
type State struct {
	mu sync.RWMutex

	startedAt time.Time
	subsystem map[string]Subsystem
	order     []string
}

// Subsystem is one thing that can be independently broken.
type Subsystem struct {
	OK bool `json:"ok"`
	// Detail says what is wrong, or what is right and worth naming — a
	// database path, a gateway URL, the model that failed to authenticate.
	Detail string `json:"detail,omitempty"`
	// Since is when this subsystem last changed answer.
	Since time.Time `json:"since,omitzero"`
}

// Health is the /v1/health body.
type Health struct {
	Version   string               `json:"version"`
	Commit    string               `json:"commit,omitempty"`
	UptimeS   int64                `json:"uptime_s"`
	Ready     bool                 `json:"ready"`
	Subsystem map[string]Subsystem `json:"subsystems"`
	// Degraded names the subsystems that are not OK, in the order they were
	// registered, so the first entry is the earliest thing to go wrong in
	// boot order rather than whichever the map iterated to first.
	Degraded []string `json:"degraded"`
}

func NewState() *State {
	return &State{
		startedAt: time.Now(),
		subsystem: map[string]Subsystem{},
	}
}

// Set records a subsystem's answer. Registration order is remembered, which
// is boot order, which is the order a person wants to read failures in.
func (s *State) Set(name string, ok bool, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, seen := s.subsystem[name]
	if !seen {
		s.order = append(s.order, name)
	}

	since := time.Now()
	if seen && prev.OK == ok {
		since = prev.Since
	}

	s.subsystem[name] = Subsystem{OK: ok, Detail: detail, Since: since}
}

// Snapshot is the whole answer, consistent at one instant.
func (s *State) Snapshot(version, commit string) Health {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subsystems := make(map[string]Subsystem, len(s.subsystem))
	degraded := []string{}

	for _, name := range s.order {
		sub := s.subsystem[name]
		subsystems[name] = sub
		if !sub.OK {
			degraded = append(degraded, name)
		}
	}

	return Health{
		Version:   version,
		Commit:    commit,
		UptimeS:   int64(time.Since(s.startedAt).Seconds()),
		Ready:     len(degraded) == 0 && len(s.order) > 0,
		Subsystem: subsystems,
		Degraded:  degraded,
	}
}
