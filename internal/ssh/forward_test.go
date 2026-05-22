package ssh

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"

	"mterm/internal/config"
)

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
	conn.SetDeadline(time.Now().Add(2 * time.Second))

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
