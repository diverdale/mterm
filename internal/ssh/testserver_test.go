package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// testServer is an in-process SSH server: it accepts any auth, accepts one
// session channel, accepts pty-req/shell/window-change, and echoes stdin to
// stdout. It records window-change requests and tracks live connections so a
// test can drop them.
type testServer struct {
	ln     net.Listener
	signer ssh.Signer
	addr   string
	mu     sync.Mutex
	conns  []net.Conn
	winch  chan [2]int
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testServer{ln: ln, signer: signer, addr: ln.Addr().String(), winch: make(chan [2]int, 8)}
	go s.serve()
	t.Cleanup(func() { ln.Close(); s.dropConns() })
	return s
}

func (s *testServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		go s.handle(conn)
	}
}

// dropConns force-closes every live connection (used to test reconnect).
func (s *testServer) dropConns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		c.Close()
	}
	s.conns = nil
}

func (s *testServer) handle(conn net.Conn) {
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(s.signer)
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "only session")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go func() {
			for r := range chReqs {
				switch r.Type {
				case "pty-req", "shell":
					r.Reply(true, nil)
				case "window-change":
					if len(r.Payload) >= 8 {
						w := int(r.Payload[3])
						h := int(r.Payload[7])
						select {
						case s.winch <- [2]int{w, h}:
						default:
						}
					}
					r.Reply(true, nil)
				default:
					r.Reply(false, nil)
				}
			}
		}()
		go func() { io.Copy(ch, ch); ch.Close() }() // echo stdin -> stdout
	}
}
