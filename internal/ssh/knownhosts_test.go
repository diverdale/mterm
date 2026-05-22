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

func TestKnownHostsMismatchFails(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	key1 := testHostKey(t)
	key2 := testHostKey(t) // a different key for the same host

	// Accept key1 for host:22.
	v1 := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool { return true })
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	if err := v1.Callback()("host:22", addr, key1); err != nil {
		t.Fatal(err)
	}

	// Present key2 for the same host — must hard-fail (MITM), not prompt.
	var asked bool
	v2 := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool {
		asked = true
		return true
	})
	err := v2.Callback()("host:22", addr, key2)
	if err == nil {
		t.Fatal("key mismatch must return an error")
	}
	if asked {
		t.Fatal("key mismatch must not invoke the decision callback")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("error should mention mismatch, got: %v", err)
	}
}

func TestKnownHostsToleratesMalformedLine(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	// Line 1 is a corrupt fragment x/crypto's strict parser rejects
	// ("missing host pattern"); it must not block verification of any host.
	if err := os.WriteFile(path, []byte(":442\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := testHostKey(t)
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}

	var asked bool
	v := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool {
		asked = true
		return true
	})
	if err := v.Callback()("host:22", addr, key); err != nil {
		t.Fatalf("malformed known_hosts line must not break the verifier: %v", err)
	}
	if !asked {
		t.Fatal("unknown host should still trigger TOFU despite the malformed line")
	}

	// The accepted entry now sits after the malformed line; it must still
	// verify silently on a fresh verifier.
	v2 := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool {
		t.Fatal("known host must not prompt")
		return false
	})
	if err := v2.Callback()("host:22", addr, key); err != nil {
		t.Fatalf("entry after a malformed line must still verify: %v", err)
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
