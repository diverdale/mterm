package ssh

import (
	"fmt"
	"io"
	"net"
	"sync"

	"mterm/internal/config"
)

// PortForward is a running forward owned by a Session.
type PortForward struct {
	spec     config.Forward
	listener net.Listener // local listener (-L) or remote listener (-R)

	mu      sync.Mutex
	stopped bool
}

// LocalAddr returns the actual local listen address (host:port). For remote
// forwards it returns the remote listen address.
func (p *PortForward) LocalAddr() string {
	return p.listener.Addr().String()
}

// Spec returns the forward's configuration.
func (p *PortForward) Spec() config.Forward { return p.spec }

// Stop closes the forward's listener.
func (p *PortForward) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.stopped = true
	p.listener.Close()
}

// StartForward starts a single port forward on this session.
func (s *Session) StartForward(spec config.Forward) (*PortForward, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("session not connected")
	}

	bindAddr := spec.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	listenAddr := net.JoinHostPort(bindAddr, fmt.Sprintf("%d", spec.BindPort))
	dialAddr := net.JoinHostPort(spec.DialAddr, fmt.Sprintf("%d", spec.DialPort))

	var ln net.Listener
	var err error
	if spec.Type == config.ForwardLocal {
		// -L: listen locally, dial through the SSH client.
		ln, err = net.Listen("tcp", listenAddr)
	} else {
		// -R: listen on the remote, dial locally.
		ln, err = client.Listen("tcp", listenAddr)
	}
	if err != nil {
		return nil, err
	}

	pf := &PortForward{spec: spec, listener: ln}
	go pf.acceptLoop(client, dialAddr)
	return pf, nil
}

// acceptLoop accepts connections on the listener and pipes each to the other
// side of the forward.
func (p *PortForward) acceptLoop(client sshDialer, dialAddr string) {
	for {
		incoming, err := p.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go p.pipe(client, incoming, dialAddr)
	}
}

func (p *PortForward) pipe(client sshDialer, incoming net.Conn, dialAddr string) {
	var target net.Conn
	var err error
	if p.spec.Type == config.ForwardLocal {
		target, err = client.Dial("tcp", dialAddr) // through the tunnel
	} else {
		target, err = net.Dial("tcp", dialAddr) // local side
	}
	if err != nil {
		incoming.Close()
		return
	}
	go func() { io.Copy(target, incoming); target.Close() }()
	go func() { io.Copy(incoming, target); incoming.Close() }()
}

// sshDialer is the subset of *ssh.Client used by forwards.
type sshDialer interface {
	Dial(network, addr string) (net.Conn, error)
}
