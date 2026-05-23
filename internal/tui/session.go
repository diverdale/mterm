package tui

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"mterm/internal/config"
	"mterm/internal/sessionlog"
	mssh "mterm/internal/ssh"
	"mterm/internal/terminal"
)

// readChunkSize bounds how many bytes the reader drains per iteration. A
// smaller value caps how long term.Write can hold the terminal mutex, which
// keeps bubbletea's View() responsive when a remote TUI floods output
// (opencode-over-ollama can easily push >1 MB/s of dense ANSI). 32 KiB chunks
// starved the event loop under that load; 8 KiB plus a runtime.Gosched per
// iteration keeps the render loop fed at the cost of slightly more lock
// cycles. See the freeze-investigation in fix/read-loop-yield commit.
const readChunkSize = 8 * 1024

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
	id       int
	host     config.Host
	term     *terminal.Terminal
	sess     atomic.Pointer[mssh.Session]      // nil until connected
	logger   atomic.Pointer[sessionlog.Logger] // optional; nil = no logging
	snapshot atomic.Pointer[string]            // last rendered VT output for View()
	ended    atomic.Bool                       // set once the reader goroutine exits
	w, h     int                               // full tab area including the tab bar
	started  time.Time                         // when this tab was opened
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

// attach binds a connected ssh.Session and starts the three per-tab
// goroutines: readLoop (SSH → VT), snapshotLoop (VT → atomic snapshot for
// View), and replyLoop (VT response pipe → SSH stdin, so terminal queries
// like Device Status Report don't dead-end and freeze the tab).
func (t *sessionTab) attach(s *mssh.Session) {
	t.sess.Store(s)
	initial := t.term.Render()
	t.snapshot.Store(&initial)
	go t.readLoop()
	go t.snapshotLoop()
	go t.replyLoop()
}

// replyLoop forwards bytes the VT wants to send BACK to the remote program
// (DSR responses, cursor-position reports, OSC color queries, in-band resize)
// to the SSH session's stdin. Without this, xvt's CSI handlers eventually
// block on a full internal response pipe, freezing the readLoop with the
// terminal mutex held — which is exactly the tab-hang bug vi exposed.
// Exits cleanly when term.Close() unblocks Read with io.EOF.
func (t *sessionTab) replyLoop() {
	defer func() { _ = recover() }()
	buf := make([]byte, 256)
	for {
		n, err := t.term.Read(buf)
		if n > 0 {
			if s := t.sess.Load(); s != nil {
				_, _ = s.Write(buf[:n])
			}
		}
		if err != nil {
			return
		}
	}
}

// snapshotInterval bounds how often the snapshot goroutine refreshes the
// rendered VT string. ~30 Hz matches the renderInterval tick; the user
// sees frames at most one tick stale even under sustained heavy writes.
const snapshotInterval = 33 * time.Millisecond

// snapshotLoop refreshes the rendered VT string at snapshotInterval until
// the session ends. Each render runs inside takeSnapshot, which recovers
// from panics so a single bad VT state cannot kill the snapshot goroutine
// and permanently freeze the tab's view.
func (t *sessionTab) snapshotLoop() {
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for range ticker.C {
		if t.ended.Load() {
			return
		}
		t.takeSnapshot()
	}
}

// takeSnapshot renders the VT into a fresh string and stores it. A panic in
// Render leaves the existing snapshot in place (so the user sees the last
// good frame) and prints a one-line diagnostic to stderr; the next tick
// tries again.
func (t *sessionTab) takeSnapshot() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "mterm: snapshot panic for tab %d (%s): %v\n", t.id, t.host.Name, r)
		}
	}()
	rendered := t.term.Render()
	t.snapshot.Store(&rendered)
}

// readLoop copies remote output into the emulator (and the session log, when
// set) until the session ends. Recovers from panics so one bad session cannot
// crash the program, and marks the tab ended on exit so the app can reap it.
func (t *sessionTab) readLoop() {
	defer func() {
		_ = recover()
		t.ended.Store(true)
	}()
	s := t.sess.Load()
	if s == nil {
		return
	}
	buf := make([]byte, readChunkSize)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			t.term.Write(buf[:n])
			if l := t.logger.Load(); l != nil {
				// Best-effort: a log write failure does not interrupt the
				// session. Closed-file errors during shutdown are expected.
				_, _ = l.Write(buf[:n])
			}
		}
		if err != nil {
			return
		}
		// Yield so a starving render goroutine can grab term.mu between
		// chunks. Without this, a flood of dense ANSI (e.g. a remote TUI
		// doing full-screen redraws) can wedge bubbletea's event loop —
		// View runs on the same goroutine as Update, so a blocked View
		// stops the clock and ignores keystrokes.
		runtime.Gosched()
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

// View renders the tab body (the terminal screen). Reads the latest
// snapshot from t.snapshot (atomic, no locking) so bubbletea's event
// loop can always render without contending for the terminal mutex.
// Falls back to a synchronous render only before the snapshot loop has
// produced its first frame (e.g. in tests that never call attach).
func (t *sessionTab) View() string {
	if s := t.snapshot.Load(); s != nil {
		return *s
	}
	return t.term.Render()
}

// close terminates the session, drains/flushes the log, and signals the
// reply loop to exit. term.Close unblocks replyLoop's Read with io.EOF;
// it races with the still-pending Read on xvt's internal `closed bool`
// (xvt does not synchronize that field), but the race is benign — Read
// returns io.EOF in either branch and the goroutine exits cleanly.
func (t *sessionTab) close() {
	if s := t.sess.Load(); s != nil {
		_ = s.Close()
	}
	t.sess.Store(nil)
	_ = t.term.Close()
	if l := t.logger.Swap(nil); l != nil {
		_ = l.Close()
	}
}

// setLogger attaches a session log; nil disables logging for this tab.
func (t *sessionTab) setLogger(l *sessionlog.Logger) {
	t.logger.Store(l)
}

// logPath returns the active log file path for this tab, or "" if none.
func (t *sessionTab) logPath() string {
	if l := t.logger.Load(); l != nil {
		return l.Path()
	}
	return ""
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
