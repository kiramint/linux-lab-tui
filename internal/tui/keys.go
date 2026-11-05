package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// feedKey forwards a Bubble Tea key to the PTY when the shell pane is focused.
// Esc / Ctrl+[ must reach the PTY as 0x1b so programs like vim can leave insert
// mode; bare Esc is never used to blur focus or quit (use Ctrl+Q to quit).
func (m *Model) feedKey(msg tea.KeyMsg) {
	b := keyToPTYBytes(msg)
	if len(b) == 0 {
		return
	}
	if msg.Type == tea.KeyCtrlL {
		m.scrollOffset = 0
	}
	m.session.HandleInput(b)
}

// keyToPTYBytes maps a Bubble Tea key message to bytes for the PTY.
// Returns nil when the key should be ignored (not forwarded).
func keyToPTYBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyEnter:
		return []byte{'\n'}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyEsc:
		// KeyEscape and KeyCtrlOpenBracket are aliases of KeyEsc (ASCII 27)
		// in bubbletea, so one case covers Esc and Ctrl+[.
		return []byte{0x1b}
	case tea.KeyCtrlD:
		return []byte{0x04}
	case tea.KeyCtrlU:
		return []byte{0x15}
	case tea.KeyCtrlL:
		return []byte{0x0c}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	case tea.KeyUp:
		return []byte{0x1b, '[', 'A'}
	case tea.KeyDown:
		return []byte{0x1b, '[', 'B'}
	case tea.KeyRight:
		return []byte{0x1b, '[', 'C'}
	case tea.KeyLeft:
		return []byte{0x1b, '[', 'D'}
	case tea.KeyHome:
		return []byte{0x1b, '[', 'H'}
	case tea.KeyEnd:
		return []byte{0x1b, '[', 'F'}
	case tea.KeyCtrlC:
		return []byte{0x03}
	default:
		return keyToPTYBytesFromString(msg.String())
	}
}

func keyToPTYBytesFromString(s string) []byte {
	switch s {
	case "esc", "escape", "ctrl+[":
		return []byte{0x1b}
	default:
		if len(s) == 1 {
			return []byte(s)
		}
		return nil
	}
}
