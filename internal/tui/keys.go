package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) feedKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyEnter:
		m.session.HandleInput([]byte{'\n'})
	case tea.KeyTab:
		// Pass Tab through for shell completion
		m.session.HandleInput([]byte{'\t'})
	case tea.KeyBackspace:
		m.session.HandleInput([]byte{0x7f})
	case tea.KeyCtrlD:
		m.session.HandleInput([]byte{0x04})
	case tea.KeyCtrlU:
		m.session.HandleInput([]byte{0x15})
	case tea.KeyCtrlL:
		// clear: bash/readline emits CSI sequences that vt10x handles
		m.session.HandleInput([]byte{0x0c})
		m.scrollOffset = 0
	case tea.KeySpace:
		m.session.HandleInput([]byte{' '})
	case tea.KeyRunes:
		m.session.HandleInput([]byte(string(msg.Runes)))
	case tea.KeyUp:
		m.session.HandleInput([]byte{0x1b, '[', 'A'})
	case tea.KeyDown:
		m.session.HandleInput([]byte{0x1b, '[', 'B'})
	case tea.KeyRight:
		m.session.HandleInput([]byte{0x1b, '[', 'C'})
	case tea.KeyLeft:
		m.session.HandleInput([]byte{0x1b, '[', 'D'})
	case tea.KeyHome:
		m.session.HandleInput([]byte{0x1b, '[', 'H'})
	case tea.KeyEnd:
		m.session.HandleInput([]byte{0x1b, '[', 'F'})
	case tea.KeyCtrlC:
		m.session.HandleInput([]byte{0x03})
	default:
		s := msg.String()
		if len(s) == 1 {
			m.session.HandleInput([]byte(s))
		}
	}
}
