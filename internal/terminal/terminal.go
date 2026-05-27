package terminal

import (
	"strings"
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

	// Defensive: cursor position can briefly fall outside the screen during
	// resize, vi's full-screen takeover, or a CUP placing the cursor past
	// the right margin. CellAt / SetCell on out-of-bounds coordinates can
	// panic in xvt. Skip cursor injection rather than crash.
	if cx < 0 || cy < 0 || cx >= t.w || cy >= t.h {
		return t.vt.Render()
	}

	orig := t.vt.CellAt(cx, cy)

	// Fix 1: if the cursor lands on a wide-char continuation cell (Width==0),
	// injecting a new cell there corrupts the source wide character and the
	// restore does not recover it. Skip cursor injection for this rare case.
	if orig != nil && orig.Width == 0 {
		return t.vt.Render()
	}

	// Build the cursor cell: clone the original (or a space) and set reverse video.
	// Fix 3: clone once for saved (restore), then copy for cursorCell mutation.
	var saved *uv.Cell
	var cursorCell uv.Cell
	if orig != nil {
		saved = orig.Clone()
		cursorCell = *saved
	} else {
		cursorCell = uv.EmptyCell
	}
	cursorCell.Style.Attrs |= uv.AttrReverse
	t.vt.SetCell(cx, cy, &cursorCell)
	// Fix 2: use defer so the restore runs even if t.vt.Render() panics.
	defer t.vt.SetCell(cx, cy, saved)
	return t.vt.Render()
}

// ScrollbackLen returns the number of lines currently held in the
// scrollback buffer. 0 when no output has scrolled off the top.
func (t *Terminal) ScrollbackLen() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.vt.ScrollbackLen()
}

// RenderAt composes a view with the given scrollback offset (0 = live view
// matching Render(); positive = that many lines back from the bottom). The
// result is exactly t.h lines high. For offsets greater than ScrollbackLen
// the view clamps to showing the entire scrollback at the top with live
// rows below.
//
// Cursor injection is skipped when offset > 0 — a cursor in the middle of
// historical content is misleading.
func (t *Terminal) RenderAt(offset int) string {
	if offset <= 0 {
		return t.Render()
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	sb := t.vt.Scrollback()
	sbLen := sb.Len()
	if offset > sbLen {
		offset = sbLen
	}

	rows := make([]string, 0, t.h)
	// Most recent `offset` scrollback lines slide up into view.
	for i := sbLen - offset; i < sbLen; i++ {
		ln := sb.Line(i)
		if ln == nil {
			rows = append(rows, "")
			continue
		}
		rows = append(rows, ln.Render())
	}
	// Fill the remainder with top rows of the live screen.
	if len(rows) < t.h {
		live := strings.Split(t.vt.Render(), "\n")
		need := t.h - len(rows)
		for i := 0; i < need && i < len(live); i++ {
			rows = append(rows, live[i])
		}
	}
	// Cap (defensive — shouldn't happen unless scrollback line count
	// arithmetic drifts).
	if len(rows) > t.h {
		rows = rows[:t.h]
	}
	return strings.Join(rows, "\n")
}

// CursorPosition returns the 0-indexed cursor column and row.
func (t *Terminal) CursorPosition() (x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pos := t.vt.CursorPosition()
	return pos.X, pos.Y
}

// Read drains bytes the emulator wants to send BACK to the program — terminal
// replies like Device Status Report (CSI n) responses, cursor-position
// reports, OSC color queries, in-band resize, etc. The caller's reader
// goroutine forwards these to the SSH session's stdin. The underlying pipe
// is goroutine-safe, so this does NOT take t.mu — taking it here would
// deadlock with Write inside the CSI handler that produced the reply.
func (t *Terminal) Read(p []byte) (int, error) {
	return t.vt.Read(p)
}

// Close releases the emulator's internal response pipe, unblocking any
// pending Reader with io.EOF. Safe to call once; idempotent on the
// underlying pipe.
func (t *Terminal) Close() error {
	return t.vt.Close()
}
