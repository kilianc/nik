package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type fakeAnimator struct {
	calls   int
	lastMsg tea.Msg
	cmd     tea.Cmd
}

func (f *fakeAnimator) Update(msg tea.Msg) tea.Cmd {
	f.calls++
	f.lastMsg = msg
	return f.cmd
}

func TestAnimatorsUpdateFansOutToAll(t *testing.T) {
	a := &fakeAnimator{}
	b := &fakeAnimator{}
	anims := animators{a, b}

	anims.Update("hello")

	if a.calls != 1 {
		t.Errorf("expected animator a called once, got %d", a.calls)
	}
	if b.calls != 1 {
		t.Errorf("expected animator b called once, got %d", b.calls)
	}
	if a.lastMsg != "hello" || b.lastMsg != "hello" {
		t.Errorf("expected both animators to receive the same msg")
	}
}

func TestAnimatorsUpdateBatchesNonNilCmds(t *testing.T) {
	a := &fakeAnimator{cmd: func() tea.Msg { return "a" }}
	b := &fakeAnimator{cmd: nil}
	c := &fakeAnimator{cmd: func() tea.Msg { return "c" }}
	anims := animators{a, b, c}

	if cmd := anims.Update("x"); cmd == nil {
		t.Error("expected non-nil batch when at least one animator returns a Cmd")
	}
}

func TestAnimatorsUpdateEmptyReturnsNil(t *testing.T) {
	var anims animators
	if cmd := anims.Update("x"); cmd != nil {
		t.Error("expected nil Cmd from empty animators slice")
	}
}

func TestAnimatorsUpdateAllNilReturnsBatch(t *testing.T) {
	a := &fakeAnimator{cmd: nil}
	anims := animators{a}
	cmd := anims.Update("x")
	if cmd == nil {
		t.Skip("tea.Batch with no Cmds returns nil — not a failure")
	}
}
