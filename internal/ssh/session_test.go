package ssh

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"mterm/internal/config"
)

func dialTestSession(t *testing.T, srv *testServer) *Session {
	t.Helper()
	host := config.Host{Name: "test", HostName: "127.0.0.1"}
	// Override the dial address with the server's ephemeral port.
	s := NewSession(host, noAuth{}, insecureHostKey())
	s.dialAddr = srv.addr
	if err := s.Connect(80, 24); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessionEchoesInput(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	if _, err := s.Write([]byte("ping\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := readWithin(t, s, "ping", time.Second)
	if !strings.Contains(got, "ping") {
		t.Fatalf("want echoed input, got %q", got)
	}
}

func TestSessionResizeReachesServer(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	if err := s.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	select {
	case dims := <-srv.winch:
		if dims[0] != 120 || dims[1] != 40 {
			t.Fatalf("server saw window %v, want [120 40]", dims)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received window-change")
	}
}

func TestSessionStateConnected(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)
	if s.State() != StateConnected {
		t.Fatalf("state = %v, want Connected", s.State())
	}
}

// readWithin reads from the session's output until want appears or timeout.
func readWithin(t *testing.T, s *Session, want string, timeout time.Duration) string {
	t.Helper()
	result := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		var acc strings.Builder
		for {
			n, err := s.Read(buf)
			if n > 0 {
				acc.Write(buf[:n])
				if strings.Contains(acc.String(), want) {
					result <- acc.String()
					return
				}
			}
			if err != nil {
				result <- acc.String()
				return
			}
		}
	}()
	select {
	case got := <-result:
		return got
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %q in session output", want)
		return ""
	}
}

func TestSessionConnectFailureSetsState(t *testing.T) {
	host := config.Host{Name: "bad", HostName: "127.0.0.1"}
	s := NewSession(host, noAuth{}, insecureHostKey())
	s.dialAddr = "127.0.0.1:1" // nothing listening here
	err := s.Connect(80, 24)
	if err == nil {
		t.Fatal("Connect should have failed but returned nil")
	}
	if s.State() != StateFailed {
		t.Fatalf("state = %v, want Failed", s.State())
	}
	if s.LastError() == nil {
		t.Fatal("LastError should be non-nil after failed Connect")
	}
}

func TestSessionWriteBeforeConnect(t *testing.T) {
	host := config.Host{Name: "test", HostName: "127.0.0.1"}
	s := NewSession(host, noAuth{}, insecureHostKey())

	if _, err := s.Write([]byte("x")); err == nil {
		t.Fatal("Write before Connect should return an error")
	}
	if err := s.Resize(80, 24); err == nil {
		t.Fatal("Resize before Connect should return an error")
	}
}

func TestSessionCloseIsIdempotent(t *testing.T) {
	srv := newTestServer(t)
	host := config.Host{Name: "test", HostName: "127.0.0.1"}
	s := NewSession(host, noAuth{}, insecureHostKey())
	s.dialAddr = srv.addr
	if err := s.Connect(80, 24); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Call Close twice; neither should panic.
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if s.State() != StateClosed {
		t.Fatalf("state = %v, want Closed", s.State())
	}
}

// noAuth is an AuthProvider that offers no methods (the test server allows
// NoClientAuth).
type noAuth struct{}

func (noAuth) Methods() ([]ssh.AuthMethod, error) { return nil, nil }

// insecureHostKey accepts any host key (test only).
func insecureHostKey() ssh.HostKeyCallback {
	return ssh.InsecureIgnoreHostKey()
}
