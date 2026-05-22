package tui

import (
	"bytes"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyToBytes(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want []byte
	}{
		{"runes", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")}, []byte("ab")},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, []byte{'\r'}},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, []byte{0x7f}},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, []byte{'\t'}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, []byte{0x1b}},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, []byte{' '}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, []byte("\x1b[A")},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, []byte("\x1b[B")},
		{"ctrl-c", tea.KeyMsg{Type: tea.KeyCtrlC}, []byte{0x03}},
		{"ctrl-a", tea.KeyMsg{Type: tea.KeyCtrlA}, []byte{0x01}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := keyToBytes(c.key)
			if !bytes.Equal(got, c.want) {
				t.Fatalf("keyToBytes(%v) = %v, want %v", c.key, got, c.want)
			}
		})
	}
}

func TestIsPrefixKey(t *testing.T) {
	if !isPrefixKey(tea.KeyMsg{Type: tea.KeyCtrlB}) {
		t.Fatal("Ctrl-B must be the prefix key")
	}
	if isPrefixKey(tea.KeyMsg{Type: tea.KeyCtrlA}) {
		t.Fatal("Ctrl-A must not be the prefix key")
	}
}
