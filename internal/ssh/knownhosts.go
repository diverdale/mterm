package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// DecisionFunc is asked to approve an unknown host key (trust on first use).
// It returns true to accept and persist the key.
type DecisionFunc func(host string, key ssh.PublicKey) bool

// HostKeyVerifier validates host keys against a known_hosts file, prompting via
// a DecisionFunc when a key is unseen.
type HostKeyVerifier struct {
	path   string
	decide DecisionFunc
}

// NewHostKeyVerifier creates a verifier backed by the given known_hosts file.
func NewHostKeyVerifier(path string, decide DecisionFunc) *HostKeyVerifier {
	return &HostKeyVerifier{path: path, decide: decide}
}

// Callback returns an ssh.HostKeyCallback for use in ssh.ClientConfig.
func (v *HostKeyVerifier) Callback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		base, err := v.baseCallback()
		if err != nil {
			return err
		}
		if err := base(hostname, remote, key); err == nil {
			return nil
		} else if _, isMismatch := err.(*knownhosts.KeyError); !isMismatch {
			return err
		} else {
			// KeyError with an empty Want slice means "unknown host"; a
			// populated Want slice means a genuine mismatch.
			ke := err.(*knownhosts.KeyError)
			if len(ke.Want) > 0 {
				return fmt.Errorf("host key mismatch for %s: possible MITM", hostname)
			}
		}
		// Unknown host: ask the user.
		if !v.decide(hostname, key) {
			return fmt.Errorf("host key for %s rejected by user", hostname)
		}
		return v.append(hostname, remote, key)
	}
}

// baseCallback builds a knownhosts callback, tolerating a missing file.
func (v *HostKeyVerifier) baseCallback() (ssh.HostKeyCallback, error) {
	if _, err := os.Stat(v.path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(v.path), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(v.path, nil, 0o600); err != nil {
			return nil, err
		}
	}
	return knownhosts.New(v.path)
}

func (v *HostKeyVerifier) append(hostname string, remote net.Addr, key ssh.PublicKey) error {
	f, err := os.OpenFile(v.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	addrs := []string{knownhosts.Normalize(hostname)}
	if remote != nil {
		addrs = append(addrs, knownhosts.Normalize(remote.String()))
	}
	line := knownhosts.Line(addrs, key)
	_, err = f.WriteString(line + "\n")
	return err
}
