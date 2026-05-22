package ssh

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"

	"mterm/internal/config"
)

func TestStartForwardNotConnected(t *testing.T) {
	s := NewSession(config.Host{Name: "x", HostName: "127.0.0.1"}, noAuth{}, insecureHostKey())
	fwd := config.Forward{
		Type:     config.ForwardLocal,
		BindAddr: "127.0.0.1",
		BindPort: 0,
		DialAddr: "127.0.0.1",
		DialPort: 1,
	}
	_, err := s.StartForward(fwd)
	if err == nil {
		t.Fatal("StartForward on unconnected session should return an error")
	}
}

func TestSessionCloseStopsForwards(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	echoHost, echoPort := echoListener(t)
	fwd := config.Forward{
		Type:     config.ForwardLocal,
		BindAddr: "127.0.0.1",
		BindPort: 0,
		DialAddr: echoHost,
		DialPort: echoPort,
	}
	pf, err := s.StartForward(fwd)
	if err != nil {
		t.Fatalf("StartForward: %v", err)
	}
	localAddr := pf.LocalAddr()

	// Confirm the forward is live.
	probe, err := net.Dial("tcp", localAddr)
	if err != nil {
		t.Fatalf("probe dial before Close: %v", err)
	}
	probe.Close()

	// Close the session — this must stop the forward.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close the local listener must be gone.
	if c, err := net.Dial("tcp", localAddr); err == nil {
		c.Close()
		t.Fatal("expected dial to fail after session Close, but it succeeded")
	}

	// Calling Stop again must not panic (idempotent).
	pf.Stop()
}

// echoListener starts a TCP echo server and returns its host and port.
func echoListener(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func TestLocalForwardEndToEnd(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	echoHost, echoPort := echoListener(t)
	fwd := config.Forward{
		Type:     config.ForwardLocal,
		BindAddr: "127.0.0.1",
		BindPort: 0, // 0 = OS-assigned, read back via LocalAddr
		DialAddr: echoHost,
		DialPort: echoPort,
	}
	pf, err := s.StartForward(fwd)
	if err != nil {
		t.Fatalf("StartForward: %v", err)
	}
	t.Cleanup(func() { pf.Stop() })

	conn, err := net.Dial("tcp", pf.LocalAddr())
	if err != nil {
		t.Fatalf("dial forwarded port: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Write([]byte("forwarded!\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if line != "forwarded!\n" {
		t.Fatalf("echo through forward = %q, want %q", line, "forwarded!\n")
	}
}
