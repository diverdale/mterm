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
	proxy    *Session // optional jump host; closed when this session closes

	mu       sync.Mutex
	state    State
	lastErr  error
	client   *ssh.Client
	sess     *ssh.Session
	stdin    io.WriteCloser
	stdout   io.Reader
	forwards []*PortForward // active port forwards, stopped on Close
	sftpCache sftpState // lazily-opened SFTP client; see sftp.go

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

// NewSessionVia builds an unconnected Session that will dial host through
// the supplied proxy session. proxy must be already-connected before this
// session's Connect runs (its underlying SSH client is what tunnels the
// TCP conn for the target). The target session takes ownership of proxy:
// Close on this session closes proxy too.
func NewSessionVia(host config.Host, auth AuthProvider, hostKey ssh.HostKeyCallback, proxy *Session) *Session {
	s := NewSession(host, auth, hostKey)
	s.proxy = proxy
	return s
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
	methods, cleanup, err := s.auth.Methods()
	if err != nil {
		s.setState(StateFailed, err)
		return err
	}
	if cleanup != nil {
		defer cleanup()
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

	client, err := s.dialClient(cfg)
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

// dialClient opens an *ssh.Client to s.dialAddr. When s.proxy is set, the
// TCP conn is tunneled through the proxy's existing SSH client before the
// target handshake; otherwise a direct net.Dial is used. The split mirrors
// ssh.Dial's own implementation but lets us substitute the conn source.
func (s *Session) dialClient(cfg *ssh.ClientConfig) (*ssh.Client, error) {
	if s.proxy == nil {
		return ssh.Dial("tcp", s.dialAddr, cfg)
	}
	s.proxy.mu.Lock()
	proxyClient := s.proxy.client
	s.proxy.mu.Unlock()
	if proxyClient == nil {
		return nil, fmt.Errorf("proxy session not connected")
	}
	conn, err := proxyClient.Dial("tcp", s.dialAddr)
	if err != nil {
		return nil, fmt.Errorf("proxy dial %s: %w", s.dialAddr, err)
	}
	// Apply the handshake timeout manually since ssh.NewClientConn doesn't
	// respect cfg.Timeout the way ssh.Dial does.
	if cfg.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(cfg.Timeout))
	}
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, s.dialAddr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy handshake %s: %w", s.dialAddr, err)
	}
	// Clear the deadline once the handshake's done; otherwise keepalives /
	// long-idle reads would fail.
	if cfg.Timeout > 0 {
		_ = conn.SetDeadline(time.Time{})
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
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
// When the session was opened via a jump host (NewSessionVia), the proxy
// session is also closed so the chain tears down completely.
func (s *Session) Close() error {
	s.mu.Lock()
	s.closed = true
	sess, client, proxy := s.sess, s.client, s.proxy
	s.sess, s.client, s.stdin, s.stdout = nil, nil, nil, nil
	s.state = StateClosed
	forwards := s.forwards
	s.forwards = nil
	s.mu.Unlock()
	for _, pf := range forwards {
		pf.Stop()
	}
	s.closeSFTP()
	if sess != nil {
		sess.Close()
	}
	var firstErr error
	if client != nil {
		firstErr = client.Close()
	}
	if proxy != nil {
		if err := proxy.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// userClosed reports whether Close was called.
func (s *Session) userClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
