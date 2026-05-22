package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// isPrefixKey reports whether k is the mterm prefix key (Ctrl-B, tmux-style).
func isPrefixKey(k tea.KeyMsg) bool {
	return k.Type == tea.KeyCtrlB
}

// keyToBytes encodes a Bubble Tea key event as the bytes a terminal would send
// to a remote PTY.
func keyToBytes(k tea.KeyMsg) []byte {
	switch k.Type {
	case tea.KeyRunes:
		return []byte(string(k.Runes))
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	}
	// Ctrl-A..Ctrl-_ are contiguous KeyType values equal to control bytes 1..31.
	if k.Type >= tea.KeyCtrlA && k.Type <= tea.KeyCtrlUnderscore {
		return []byte{byte(k.Type)}
	}
	return nil
}
