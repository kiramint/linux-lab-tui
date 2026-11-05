package tui

import (
	"bytes"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEscKeyTypesAreASCIIEscape(t *testing.T) {
	// bubbletea aliases Esc / Escape / Ctrl+[ to the same KeyType (27).
	if tea.KeyEsc != tea.KeyEscape {
		t.Fatalf("KeyEsc (%v) != KeyEscape (%v)", tea.KeyEsc, tea.KeyEscape)
	}
	if tea.KeyEsc != tea.KeyCtrlOpenBracket {
		t.Fatalf("KeyEsc (%v) != KeyCtrlOpenBracket (%v)", tea.KeyEsc, tea.KeyCtrlOpenBracket)
	}
	if int(tea.KeyEsc) != 0x1b {
		t.Fatalf("KeyEsc should be 0x1b, got %d", tea.KeyEsc)
	}
}

func TestKeyToPTYBytes_Esc(t *testing.T) {
	cases := []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyEscape},
		{Type: tea.KeyCtrlOpenBracket},
	}
	for _, msg := range cases {
		got := keyToPTYBytes(msg)
		want := []byte{0x1b}
		if !bytes.Equal(got, want) {
			t.Fatalf("msg Type=%v String=%q: got %v want %v", msg.Type, msg.String(), got, want)
		}
	}
	got := keyToPTYBytesFromString("esc")
	if !bytes.Equal(got, []byte{0x1b}) {
		t.Fatalf("string esc: got %v", got)
	}
	got = keyToPTYBytesFromString("ctrl+[")
	if !bytes.Equal(got, []byte{0x1b}) {
		t.Fatalf("string ctrl+[: got %v", got)
	}
	got = keyToPTYBytesFromString("escape")
	if !bytes.Equal(got, []byte{0x1b}) {
		t.Fatalf("string escape: got %v", got)
	}
}

func TestKeyToPTYBytes_Common(t *testing.T) {
	if !bytes.Equal(keyToPTYBytes(tea.KeyMsg{Type: tea.KeyEnter}), []byte{'\n'}) {
		t.Fatal("enter")
	}
	if !bytes.Equal(keyToPTYBytes(tea.KeyMsg{Type: tea.KeyTab}), []byte{'\t'}) {
		t.Fatal("tab")
	}
	if !bytes.Equal(keyToPTYBytes(tea.KeyMsg{Type: tea.KeyCtrlC}), []byte{0x03}) {
		t.Fatal("ctrl+c")
	}
}
