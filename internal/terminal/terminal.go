package terminal

import (
	"sync"

	xvt "github.com/charmbracelet/x/vt"
)

// Terminal wraps a VT emulator. It is safe for concurrent use: a reader
// goroutine calls Write while the UI goroutine calls Render.
type Terminal struct {
	mu   sync.Mutex
	vt   *xvt.Emulator
	w, h int
}

// New creates a Terminal with the given character dimensions.
func New(w, h int) *Terminal {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Terminal{vt: xvt.NewEmulator(w, h), w: w, h: h}
}

// Write feeds remote output bytes into the emulator.
func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.vt.Write(p)
}

// Resize changes the emulator geometry.
func (t *Terminal) Resize(w, h int) {
	if w < 1 || h < 1 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.w, t.h = w, h
	t.vt.Resize(w, h)
}

// Size returns the current dimensions.
func (t *Terminal) Size() (w, h int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.w, t.h
}

// Render returns the visible screen as a string with exactly h rows joined by
// h-1 newline characters. Cell foreground/background colors and text attributes
// (bold, italic, etc.) are encoded as ANSI SGR escape sequences so that remote
// programs (htop, vim, ls --color, …) display in color. Each row that contains
// styled content ends with an ANSI reset (\x1b[m) so colors cannot bleed into
// the tab bar or adjacent rows.
func (t *Terminal) Render() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.vt.Render()
}

// CursorPosition returns the 0-indexed cursor column and row.
func (t *Terminal) CursorPosition() (x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pos := t.vt.CursorPosition()
	return pos.X, pos.Y
}
