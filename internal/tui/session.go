package tui

import (
	"sync/atomic"
	"time"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
	"mterm/internal/terminal"
)

// chromeCols and chromeRows are the screen space the window frame reserves
// around a session's terminal body: left+right borders, and (top border, tab
// strip, tab divider, footer divider, footer, bottom border) respectively.
const (
	chromeCols = 2
	chromeRows = 6
)

// tabStatus is a session tab's connection state, derived from the tab.
type tabStatus int

const (
	statusConnecting tabStatus = iota
	statusConnected
	statusFailed
)

// status derives the tab's connection state. A tab whose reader goroutine has
// exited is failed — an absorbing state, checked first, since the reader can
// exit while sess is still set. Otherwise a live session is connected, and a
// tab still being dialed is connecting.
func (t *sessionTab) status() tabStatus {
	if t.ended.Load() {
		return statusFailed
	}
	if t.sess.Load() != nil {
		return statusConnected
	}
	return statusConnecting
}

// sessionTab is one connection tab: an ssh.Session feeding a terminal emulator.
type sessionTab struct {
	id      int
	host    config.Host
	term    *terminal.Terminal
	sess    atomic.Pointer[mssh.Session] // nil until connected
	ended   atomic.Bool                  // set once the reader goroutine exits
	w, h    int                          // full tab area including the tab bar
	started time.Time                    // when this tab was opened
}

// newSessionTab creates a tab sized to the given total area.
func newSessionTab(id int, host config.Host, w, h int) *sessionTab {
	bodyW, bodyH := h2body(w, h)
	return &sessionTab{
		id:      id,
		host:    host,
		term:    terminal.New(bodyW, bodyH),
		w:       w,
		h:       h,
		started: time.Now(),
	}
}

// uptime returns how long this tab has been open.
func (t *sessionTab) uptime() time.Duration { return time.Since(t.started) }

func (t *sessionTab) title() string { return t.host.Name }

// hasEnded reports whether the remote session has finished (the user ran
// `exit`, the connection dropped, etc.).
func (t *sessionTab) hasEnded() bool { return t.ended.Load() }

// attach binds a connected ssh.Session and starts the reader goroutine.
func (t *sessionTab) attach(s *mssh.Session) {
	t.sess.Store(s)
	go t.readLoop()
}

// readLoop copies remote output into the emulator until the session ends. It
// recovers from panics so one bad session cannot crash the program, and marks
// the tab ended on exit so the app can reap it.
func (t *sessionTab) readLoop() {
	defer func() {
		_ = recover()
		t.ended.Store(true)
	}()
	s := t.sess.Load()
	if s == nil {
		return
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			t.term.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// resize updates both the tab area and the underlying terminal/PTY.
func (t *sessionTab) resize(w, h int) {
	t.w, t.h = w, h
	bodyW, bodyH := h2body(w, h)
	t.term.Resize(bodyW, bodyH)
	if s := t.sess.Load(); s != nil {
		_ = s.Resize(bodyW, bodyH)
	}
}

// sendInput writes encoded key bytes to the remote shell.
func (t *sessionTab) sendInput(p []byte) {
	if s := t.sess.Load(); s != nil && len(p) > 0 {
		_, _ = s.Write(p)
	}
}

// View renders the tab body (the terminal screen).
func (t *sessionTab) View() string {
	return t.term.Render()
}

// close terminates the session.
func (t *sessionTab) close() {
	if s := t.sess.Load(); s != nil {
		_ = s.Close()
	}
	t.sess.Store(nil)
}

// h2body converts a full tab area (w, h) into the inner terminal body size,
// clamped to a minimum of 1×1.
func h2body(w, h int) (int, int) {
	bw, bh := w-chromeCols, h-chromeRows
	if bw < 1 {
		bw = 1
	}
	if bh < 1 {
		bh = 1
	}
	return bw, bh
}
