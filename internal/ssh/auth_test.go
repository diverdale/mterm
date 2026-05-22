package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh/agent"
)

// startFakeAgent serves an in-memory agent holding one key on a unix socket and
// returns the socket path.
func startFakeAgent(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: &priv}); err != nil {
		t.Fatal(err)
	}
	// Use os.MkdirTemp with a short base to avoid macOS's 104-byte unix
	// socket path limit, which t.TempDir() can exceed with long test names.
	dir, err := os.MkdirTemp("", "ag")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, conn)
		}
	}()
	return sock
}

func TestAgentProviderReturnsMethods(t *testing.T) {
	sock := startFakeAgent(t)
	p := &AgentProvider{SocketPath: sock}
	methods, cleanup, err := p.Methods()
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if cleanup == nil {
		t.Fatal("want a non-nil cleanup func")
	}
	defer cleanup()
	if len(methods) == 0 {
		t.Fatal("want at least one auth method")
	}
}

func TestAgentProviderMissingSocket(t *testing.T) {
	p := &AgentProvider{SocketPath: filepath.Join(t.TempDir(), "nope.sock")}
	if _, _, err := p.Methods(); err == nil {
		t.Fatal("want error for missing agent socket")
	}
}

func TestAgentProviderFromEnv(t *testing.T) {
	t.Run("with socket set", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "/some/path")
		p, err := AgentProviderFromEnv()
		if err != nil {
			t.Fatalf("AgentProviderFromEnv: %v", err)
		}
		if p.SocketPath != "/some/path" {
			t.Fatalf("want SocketPath=/some/path, got %q", p.SocketPath)
		}
	})

	t.Run("with socket unset", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "")
		_, err := AgentProviderFromEnv()
		if err == nil {
			t.Fatal("want error when SSH_AUTH_SOCK is empty")
		}
	})
}
