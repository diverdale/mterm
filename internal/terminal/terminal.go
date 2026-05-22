package terminal

import (
	"sync"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	xvt "github.com/charmbracelet/x/vt"
)

// Terminal wraps a VT emulator. It is safe for concurrent use: a reader
// goroutine calls Write while the UI goroutine calls Render.
type Terminal struct {
	mu            sync.Mutex
	vt            *xvt.Emulator
	w, h          int
	cursorVisible atomic.Bool // tracks DECTCEM (?25h / ?25l); true = visible
}

// New creates a Terminal with the given character dimensions.
func New(w, h int) *Terminal {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	t := &Terminal{
		vt: xvt.NewEmulator(w, h),
		w:  w,
		h:  h,
	}
	// DECTCEM (?25h) defaults to enabled — cursor is visible on startup.
	t.cursorVisible.Store(true)
	// Register a callback so we track cursor visibility changes caused by
	// remote programs sending \x1b[?25l (hide) or \x1b[?25h (show).
	// The callback is invoked from within Write (which holds t.mu), so we
	// must NOT acquire t.mu here; atomic.Bool avoids the deadlock.
	t.vt.SetCallbacks(xvt.Callbacks{
		CursorVisibility: func(visible bool) {
			t.cursorVisible.Store(visible)
		},
	})
	return t
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
//
// When the cursor is visible (DECTCEM ?25h), a block cursor is drawn at the
// current cursor position using reverse-video. When the remote program has
// hidden the cursor via \x1b[?25l (e.g. vim, htop), no cursor block is drawn.
func (t *Terminal) Render() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.cursorVisible.Load() {
		return t.vt.Render()
	}

	// Draw a block cursor (reverse-video) at the current cursor cell.
	// We temporarily replace the cell with a reverse-video clone, render,
	// then restore the original so the emulator state is not mutated.
	pos := t.vt.CursorPosition()
	cx, cy := pos.X, pos.Y

	orig := t.vt.CellAt(cx, cy)
	var saved *uv.Cell
	if orig != nil {
		saved = orig.Clone()
	}

	// Build the cursor cell: clone the original (or a space) and set reverse video.
	var cursorCell uv.Cell
	if orig != nil {
		cursorCell = *orig.Clone()
	} else {
		cursorCell = uv.EmptyCell
	}
	cursorCell.Style.Attrs |= uv.AttrReverse
	t.vt.SetCell(cx, cy, &cursorCell)

	out := t.vt.Render()

	// Restore the original cell.
	t.vt.SetCell(cx, cy, saved)

	return out
}

// CursorPosition returns the 0-indexed cursor column and row.
func (t *Terminal) CursorPosition() (x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pos := t.vt.CursorPosition()
	return pos.X, pos.Y
}
