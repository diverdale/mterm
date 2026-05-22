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

// Methods dials the agent and returns a public-key auth method backed by it.
func (p *AgentProvider) Methods() ([]ssh.AuthMethod, error) {
	conn, err := net.Dial("unix", p.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	}
	ag := agent.NewClient(conn)
	if _, err := ag.List(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("query ssh-agent: %w", err)
	}
	return []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)}, nil
}
