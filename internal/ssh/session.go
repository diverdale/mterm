package ssh

import (
	"fmt"
	"io"
	osuser "os/user"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"mterm/internal/config"
)

// State is the lifecycle state of a Session.
type State int

const (
	StateConnecting State = iota
	StateConnected
	StateReconnecting
	StateFailed
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateFailed:
		return "failed"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Session is a single live SSH connection with an interactive PTY shell.
type Session struct {
	host     config.Host
	auth     AuthProvider
	hostKey  ssh.HostKeyCallback
	dialAddr string // host:port to dial; defaults to host.Addr()

	mu       sync.Mutex
	state    State
	lastErr  error
	client   *ssh.Client
	sess     *ssh.Session
	stdin    io.WriteCloser
	stdout   io.Reader
	forwards []*PortForward // active port forwards, stopped on Close

	closed bool // true once the user called Close
}

// NewSession builds an unconnected Session for the given host.
func NewSession(host config.Host, auth AuthProvider, hostKey ssh.HostKeyCallback) *Session {
	return &Session{
		host:     host,
		auth:     auth,
		hostKey:  hostKey,
		dialAddr: host.Addr(),
		state:    StateConnecting,
	}
}

// Host returns the session's host definition.
func (s *Session) Host() config.Host { return s.host }

// State returns the current lifecycle state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// LastError returns the most recent connection error, if any.
func (s *Session) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Session) setState(st State, err error) {
	s.mu.Lock()
	s.state = st
	if err != nil {
		s.lastErr = err
	}
	s.mu.Unlock()
}

// Connect dials the host and starts an interactive shell with a PTY of the
// given size.
func (s *Session) Connect(cols, rows int) error {
	methods, err := s.auth.Methods()
	if err != nil {
		s.setState(StateFailed, err)
		return err
	}
	user := s.host.User
	if user == "" {
		if u, err := osuser.Current(); err == nil {
			user = u.Username
		} else {
			user = "root"
		}
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            methods,
		HostKeyCallback: s.hostKey,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", s.dialAddr, cfg)
	if err != nil {
		s.setState(StateFailed, err)
		return err
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	sess.Stderr = nil // stderr is merged into the PTY by the server
	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		sess.Close()
		client.Close()
		return fmt.Errorf("session closed during connect")
	}
	s.client = client
	s.sess = sess
	s.stdin = stdin
	s.stdout = stdout
	s.state = StateConnected
	s.lastErr = nil
	s.mu.Unlock()
	return nil
}

// Write sends bytes to the remote shell's stdin.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	w := s.stdin
	s.mu.Unlock()
	if w == nil {
		return 0, fmt.Errorf("session not connected")
	}
	return w.Write(p)
}

// Read reads remote output. Used by the reader goroutine and tests.
func (s *Session) Read(p []byte) (int, error) {
	s.mu.Lock()
	r := s.stdout
	s.mu.Unlock()
	if r == nil {
		return 0, io.EOF
	}
	return r.Read(p)
}

// Resize changes the remote PTY dimensions.
func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	sess := s.sess
	s.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("session not connected")
	}
	return sess.WindowChange(rows, cols)
}

// Close terminates the session; it will not auto-reconnect afterward.
func (s *Session) Close() error {
	s.mu.Lock()
	s.closed = true
	sess, client := s.sess, s.client
	s.sess, s.client, s.stdin, s.stdout = nil, nil, nil, nil
	s.state = StateClosed
	forwards := s.forwards
	s.forwards = nil
	s.mu.Unlock()
	for _, pf := range forwards {
		pf.Stop()
	}
	if sess != nil {
		sess.Close()
	}
	if client != nil {
		return client.Close()
	}
	return nil
}

// userClosed reports whether Close was called.
func (s *Session) userClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
