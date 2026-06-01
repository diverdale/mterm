package ssh

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// IdentityFileProvider authenticates using an unencrypted private key read
// from disk — the same fallback `ssh` uses when SSH_AUTH_SOCK is empty or
// the agent has no matching key.
type IdentityFileProvider struct {
	Path string // ~/ and $HOME are expanded by NewIdentityFileProvider
}

// NewIdentityFileProvider expands path (handles "~/", "~", and "$HOME") and
// returns a provider that will load + parse the key on each Methods() call.
func NewIdentityFileProvider(path string) *IdentityFileProvider {
	return &IdentityFileProvider{Path: ExpandPath(path)}
}

// Methods reads, parses, and returns one publickey auth method backed by the
// identity file. Encrypted keys produce a clear error so the user sees
// "decrypt failed" instead of a generic "no supported methods remain."
func (p *IdentityFileProvider) Methods() ([]ssh.AuthMethod, func(), error) {
	if p.Path == "" {
		return nil, nil, errors.New("identity file path is empty")
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read identity file %q: %w", p.Path, err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		var pmErr *ssh.PassphraseMissingError
		if errors.As(err, &pmErr) {
			return nil, nil, fmt.Errorf("identity file %q is passphrase-protected; load it into ssh-agent (ssh-add --apple-use-keychain) or use an unencrypted key", p.Path)
		}
		return nil, nil, fmt.Errorf("parse identity file %q: %w", p.Path, err)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil, nil
}

// ExpandPath replaces "~" / "~/..." / "$HOME" prefixes with the user's home
// directory. Returns the input unchanged if no expansion applies or if home
// can't be determined.
func ExpandPath(path string) string {
	if path == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	case strings.HasPrefix(path, "$HOME"):
		return filepath.Join(home, path[len("$HOME"):])
	}
	return path
}
