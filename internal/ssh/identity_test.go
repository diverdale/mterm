package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// writeUnencryptedEd25519Key writes a fresh ed25519 private key in OpenSSH
// format to path. Returns the path so tests can chain.
func writeUnencryptedEd25519Key(t *testing.T, path string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIdentityFileProviderLoadsUnencryptedKey(t *testing.T) {
	path := writeUnencryptedEd25519Key(t, filepath.Join(t.TempDir(), "id_test"))
	p := NewIdentityFileProvider(path)
	methods, _, err := p.Methods()
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected 1 auth method, got %d", len(methods))
	}
}

func TestIdentityFileProviderMissingFile(t *testing.T) {
	p := NewIdentityFileProvider(filepath.Join(t.TempDir(), "does-not-exist"))
	_, _, err := p.Methods()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read identity file") {
		t.Fatalf("unexpected error wording: %v", err)
	}
}

func TestIdentityFileProviderEmptyPath(t *testing.T) {
	p := &IdentityFileProvider{Path: ""}
	if _, _, err := p.Methods(); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestExpandPathTilde(t *testing.T) {
	t.Setenv("HOME", "/tmp/fake-home")
	cases := map[string]string{
		"~/.ssh/id":   "/tmp/fake-home/.ssh/id",
		"~":           "/tmp/fake-home",
		"$HOME/.ssh":  "/tmp/fake-home/.ssh",
		"/etc/passwd": "/etc/passwd",
		"":            "",
	}
	for in, want := range cases {
		if got := ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// stubProvider lets the chain tests inject deterministic results.
type stubProvider struct {
	methods []ssh.AuthMethod
	err     error
}

func (s stubProvider) Methods() ([]ssh.AuthMethod, func(), error) {
	return s.methods, nil, s.err
}

func TestChainProviderConcatenatesMethods(t *testing.T) {
	a := stubProvider{methods: []ssh.AuthMethod{ssh.Password("a")}}
	b := stubProvider{methods: []ssh.AuthMethod{ssh.Password("b")}}
	chain := ChainProvider(a, b)
	got, _, err := chain.Methods()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(got))
	}
}

func TestChainProviderSkipsErrors(t *testing.T) {
	a := stubProvider{err: errChainTest("oops")}
	b := stubProvider{methods: []ssh.AuthMethod{ssh.Password("b")}}
	chain := ChainProvider(a, b)
	got, _, err := chain.Methods()
	if err != nil {
		t.Fatalf("when at least one method succeeds, err should be nil; got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 method from the surviving provider, got %d", len(got))
	}
}

func TestChainProviderSurfacesFirstErrorWhenAllFail(t *testing.T) {
	a := stubProvider{err: errChainTest("first")}
	b := stubProvider{err: errChainTest("second")}
	chain := ChainProvider(a, b)
	_, _, err := chain.Methods()
	if err == nil || !strings.Contains(err.Error(), "first") {
		t.Fatalf("expected first error to surface; got %v", err)
	}
}

func TestChainProviderIgnoresNilProviders(t *testing.T) {
	a := stubProvider{methods: []ssh.AuthMethod{ssh.Password("a")}}
	chain := ChainProvider(nil, a, nil)
	got, _, err := chain.Methods()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 method, got %d", len(got))
	}
}

type errChainTest string

func (e errChainTest) Error() string { return string(e) }
