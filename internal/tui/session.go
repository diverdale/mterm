package tui

import (
	"sync/atomic"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
	"mterm/internal/terminal"
)

// tabBarRows is the height reserved for the tab bar.
const tabBarRows = 1

// sessionTab is one connection tab: an ssh.Session feeding a terminal emulator.
type sessionTab struct {
	id    int
	host  config.Host
	term  *terminal.Terminal
	sess  atomic.Pointer[mssh.Session] // nil until connected
	ended atomic.Bool                  // set once the reader goroutine exits
	w, h  int                          // full tab area including the tab bar
}

// newSessionTab creates a tab sized to the given total area.
func newSessionTab(id int, host config.Host, w, h int) *sessionTab {
	bodyH := h - tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	return &sessionTab{
		id:   id,
		host: host,
		term: terminal.New(w, bodyH),
		w:    w,
		h:    h,
	}
}

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
	bodyH := h - tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	t.term.Resize(w, bodyH)
	if s := t.sess.Load(); s != nil {
		_ = s.Resize(w, bodyH)
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
