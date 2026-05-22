package ssh

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// AuthProvider supplies SSH authentication methods. v1 ships only AgentProvider;
// key-file, password, and jump-host providers implement this interface later
// with no caller changes.
type AuthProvider interface {
	Methods() ([]ssh.AuthMethod, error)
}

// AgentProvider authenticates using keys held by a running ssh-agent.
type AgentProvider struct {
	SocketPath string // path to the agent's unix socket
}

// AgentProviderFromEnv builds an AgentProvider from SSH_AUTH_SOCK.
func AgentProviderFromEnv() (*AgentProvider, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK is not set; start an ssh-agent and run ssh-add")
	}
	return &AgentProvider{SocketPath: sock}, nil
}

// Methods returns a public-key auth method backed by the ssh-agent. It probes
// the agent socket eagerly so an unreachable agent fails fast; the returned
// auth method dials a fresh, short-lived connection for each key negotiation,
// so no socket is held open between handshakes.
func (p *AgentProvider) Methods() ([]ssh.AuthMethod, error) {
	probe, err := net.Dial("unix", p.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	}
	probe.Close()
	return []ssh.AuthMethod{ssh.PublicKeysCallback(p.signers)}, nil
}

// signers dials the agent, retrieves its signers, and closes the connection.
func (p *AgentProvider) signers() ([]ssh.Signer, error) {
	conn, err := net.Dial("unix", p.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	}
	defer conn.Close()
	return agent.NewClient(conn).Signers()
}
