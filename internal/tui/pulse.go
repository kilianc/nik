package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	pulseEaseOutRate = 0.03
	pulseEaseInRate  = 0.06
	pulseEnergyFloor = 0.005
)

// pulse is the single animation primitive used across the TUI. It owns a tick
// counter, an energy value (eased in while active, out when idle), and a
// tag-protected tick chain. Concurrent forks self-reject via the tag.
//
// Visual elements (spinner glyph, breathing border, dots) are render-readers
// that consume Tick() and Energy(); they are not separate animators.
type pulse struct {
	tag     int
	tick    int
	energy  float64
	active  bool
	running bool
	cadence time.Duration
}

type pulseTickMsg struct{ tag int }

func newPulse(cadence time.Duration) *pulse {
	return &pulse{cadence: cadence}
}

// SetActive declares whether the animation should be ticking. Returns a Cmd
// that kicks the chain if it wasn't already running, nil otherwise. Safe to
// call every Update — calling it while running is a no-op.
func (p *pulse) SetActive(active bool) tea.Cmd {
	p.active = active
	if active && !p.running {
		p.running = true
		return p.cmd()
	}
	return nil
}

// Update is the animator contract. Filters for pulseTickMsg; on stale tag
// drops silently; on match advances tick/energy and either re-arms or lets
// the chain die (idle + energy floor reached).
func (p *pulse) Update(msg tea.Msg) tea.Cmd {
	tm, ok := msg.(pulseTickMsg)
	if !ok {
		return nil
	}
	if tm.tag != p.tag {
		return nil
	}
	p.tag++
	p.tick++

	if p.active {
		if p.energy < 1.0 {
			p.energy += pulseEaseInRate * (1.0 - p.energy + 0.05)
			if p.energy > 1.0 {
				p.energy = 1.0
			}
		}
		return p.cmd()
	}

	if p.energy > 0.0 {
		p.energy *= 1.0 - pulseEaseOutRate
		if p.energy < pulseEnergyFloor {
			p.energy = 0.0
			p.running = false
			return nil
		}
		return p.cmd()
	}

	p.running = false
	return nil
}

func (p *pulse) cmd() tea.Cmd {
	tag := p.tag
	return tea.Tick(p.cadence, func(time.Time) tea.Msg {
		return pulseTickMsg{tag: tag}
	})
}

func (p pulse) Tick() int       { return p.tick }
func (p pulse) Energy() float64 { return p.energy }
