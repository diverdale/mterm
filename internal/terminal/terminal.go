package terminal

import (
	"strings"
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

// Render returns the visible screen as a plain-text string with exactly h rows
// joined by h-1 newline characters. Each row is padded with spaces to width w.
func (t *Terminal) Render() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var sb strings.Builder
	for y := 0; y < t.h; y++ {
		if y > 0 {
			sb.WriteByte('\n')
		}
		row := make([]byte, t.w)
		for i := range row {
			row[i] = ' '
		}
		for x := 0; x < t.w; x++ {
			cell := t.vt.CellAt(x, y)
			if cell != nil && !cell.IsZero() {
				s := cell.Content
				if len(s) > 0 {
					// Write as many bytes as fit in the row slot.
					copy(row[x:], []byte(s))
				}
			}
		}
		sb.Write(row)
	}
	return sb.String()
}

// CursorPosition returns the 0-indexed cursor column and row.
func (t *Terminal) CursorPosition() (x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pos := t.vt.CursorPosition()
	return pos.X, pos.Y
}
