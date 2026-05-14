package tui

import tea "github.com/charmbracelet/bubbletea"

// animator is a self-contained UI component with its own tick loop. Update
// accepts any tea.Msg and filters internally for the messages it cares about.
// Implementers must use tag-based fork rejection so concurrent forks
// self-cancel by tag mismatch.
type animator interface {
	Update(msg tea.Msg) tea.Cmd
}

// animators is the parent's registry. One call to Update at the top of the
// parent's Update fans every msg out to every registered animator. New
// animations are added in the parent's constructor; the routing call never
// changes.
type animators []animator

func (a animators) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for _, anim := range a {
		if cmd := anim.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}
