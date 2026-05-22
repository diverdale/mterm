package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return pk
}

func TestKnownHostsUnknownKeyTriggersDecision(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	var asked bool
	v := NewHostKeyVerifier(path, func(host string, key ssh.PublicKey) bool {
		asked = true
		return true // accept
	})
	cb := v.Callback()
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	if err := cb("host:22", addr, testHostKey(t)); err != nil {
		t.Fatalf("accepted key must pass: %v", err)
	}
	if !asked {
		t.Fatal("decision callback was not invoked for unknown key")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "host") {
		t.Fatal("accepted key was not appended to known_hosts")
	}
}

func TestKnownHostsRejectedKeyFails(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	v := NewHostKeyVerifier(path, func(host string, key ssh.PublicKey) bool {
		return false // reject
	})
	cb := v.Callback()
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	if err := cb("host:22", addr, testHostKey(t)); err == nil {
		t.Fatal("rejected key must produce an error")
	}
}

func TestKnownHostsKnownKeyDoesNotAsk(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	key := testHostKey(t)

	v1 := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool { return true })
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	if err := v1.Callback()("host:22", addr, key); err != nil {
		t.Fatal(err)
	}

	var asked bool
	v2 := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool {
		asked = true
		return true
	})
	if err := v2.Callback()("host:22", addr, key); err != nil {
		t.Fatalf("previously accepted key must pass silently: %v", err)
	}
	if asked {
		t.Fatal("known key must not trigger the decision callback")
	}
}
