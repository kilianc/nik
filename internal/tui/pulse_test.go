package tui

import (
	"testing"
	"time"
)

func TestPulseSetActiveKicksOnce(t *testing.T) {
	p := newPulse(30 * time.Millisecond)

	cmd := p.SetActive(true)
	if cmd == nil {
		t.Fatal("expected non-nil Cmd on first SetActive(true)")
	}
	if !p.running {
		t.Error("expected running=true after SetActive(true)")
	}

	if cmd := p.SetActive(true); cmd != nil {
		t.Error("expected nil Cmd when SetActive(true) called while running")
	}
}

func TestPulseUpdateRejectsStaleTag(t *testing.T) {
	p := newPulse(30 * time.Millisecond)
	p.SetActive(true)

	if cmd := p.Update(pulseTickMsg{tag: 99}); cmd != nil {
		t.Error("expected nil Cmd on stale tag")
	}
	if p.tick != 0 {
		t.Errorf("expected tick unchanged on stale tag, got %d", p.tick)
	}
	if p.tag != 0 {
		t.Errorf("expected tag unchanged on stale tag, got %d", p.tag)
	}
}

func TestPulseUpdateActiveAdvances(t *testing.T) {
	p := newPulse(30 * time.Millisecond)
	p.SetActive(true)

	cmd := p.Update(pulseTickMsg{tag: 0})
	if cmd == nil {
		t.Fatal("expected non-nil Cmd while active")
	}
	if p.tick != 1 {
		t.Errorf("expected tick=1, got %d", p.tick)
	}
	if p.tag != 1 {
		t.Errorf("expected tag=1, got %d", p.tag)
	}
	if p.energy <= 0 {
		t.Errorf("expected energy > 0 while active, got %f", p.energy)
	}
}

func TestPulseUpdateDecayThenDie(t *testing.T) {
	p := newPulse(30 * time.Millisecond)
	p.SetActive(true)
	p.energy = 0.5
	p.SetActive(false)

	cmd := p.Update(pulseTickMsg{tag: p.tag})
	if cmd == nil {
		t.Fatal("expected non-nil Cmd while decaying")
	}
	if p.energy >= 0.5 {
		t.Errorf("expected energy to decay below 0.5, got %f", p.energy)
	}

	p.energy = pulseEnergyFloor / 2
	tag := p.tag
	cmd = p.Update(pulseTickMsg{tag: tag})
	if cmd != nil {
		t.Error("expected nil Cmd when crossing floor")
	}
	if p.running {
		t.Error("expected running=false after chain dies")
	}
	if p.energy != 0 {
		t.Errorf("expected energy snapped to 0, got %f", p.energy)
	}
}

func TestPulseUpdateIdleDies(t *testing.T) {
	p := newPulse(30 * time.Millisecond)
	p.SetActive(true)
	p.SetActive(false)

	cmd := p.Update(pulseTickMsg{tag: 0})
	if cmd != nil {
		t.Error("expected nil Cmd when idle and energy=0")
	}
	if p.running {
		t.Error("expected running=false after idle tick")
	}
}

func TestPulseUpdateIgnoresUnknownMsg(t *testing.T) {
	p := newPulse(30 * time.Millisecond)
	p.SetActive(true)

	if cmd := p.Update("string msg"); cmd != nil {
		t.Error("expected nil Cmd on unknown msg type")
	}
	if p.tick != 0 {
		t.Errorf("expected tick unchanged on unknown msg, got %d", p.tick)
	}
}
