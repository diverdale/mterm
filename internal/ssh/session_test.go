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
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		s.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, _ := s.Read(buf)
		acc.Write(buf[:n])
		if strings.Contains(acc.String(), want) {
			break
		}
	}
	return acc.String()
}

// noAuth is an AuthProvider that offers no methods (the test server allows
// NoClientAuth).
type noAuth struct{}

func (noAuth) Methods() ([]ssh.AuthMethod, error) { return nil, nil }

// insecureHostKey accepts any host key (test only).
func insecureHostKey() ssh.HostKeyCallback {
	return ssh.InsecureIgnoreHostKey()
}
