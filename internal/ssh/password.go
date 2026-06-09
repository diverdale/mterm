package ssh

import (
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/crypto/ssh"
)

// PasswordProvider authenticates with a static password literal. Registers
// both `password` and `keyboard-interactive` methods backed by the same
// credential — many network OSes (Arista, Juniper, some Cisco) negotiate
// keyboard-interactive even when the credential is just a password.
type PasswordProvider struct {
	password string
}

// NewPasswordProvider returns a provider that offers password literal as
// both password and keyboard-interactive credentials. Empty password
// makes the provider a no-op (Methods returns nil, nil, nil) so callers
// can unconditionally chain it without checking emptiness.
func NewPasswordProvider(password string) *PasswordProvider {
	return &PasswordProvider{password: password}
}

// Methods returns ssh.Password and ssh.KeyboardInteractive, both feeding
// the same password back to the server. Keyboard-interactive replies the
// password verbatim for every question (matches how `expect` scripts
// drive network gear).
func (p *PasswordProvider) Methods() ([]ssh.AuthMethod, func(), error) {
	if p == nil || p.password == "" {
		return nil, nil, nil
	}
	pw := p.password
	kbd := ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = pw
		}
		return answers, nil
	})
	return []ssh.AuthMethod{ssh.Password(pw), kbd}, nil, nil
}

// PasswordCommandProvider runs an external command and uses its trimmed
// stdout as the password. Lets callers pull secrets from a vault, a
// password manager (`op read`, `pass show`, `bw get password`) etc. so
// the hosts.yaml never holds plaintext.
type PasswordCommandProvider struct {
	cmdline string // shell command line; passed to `sh -c`
}

// NewPasswordCommandProvider returns a provider that resolves the
// password by running cmdline through `sh -c`. Empty cmdline makes the
// provider a no-op.
func NewPasswordCommandProvider(cmdline string) *PasswordCommandProvider {
	return &PasswordCommandProvider{cmdline: cmdline}
}

// Methods runs the configured command and wraps its trimmed stdout in
// the same password + keyboard-interactive methods PasswordProvider
// offers. A non-zero exit or empty output returns an error so the chain
// can fall through to whatever is left.
func (p *PasswordCommandProvider) Methods() ([]ssh.AuthMethod, func(), error) {
	if p == nil || p.cmdline == "" {
		return nil, nil, nil
	}
	out, err := exec.Command("sh", "-c", p.cmdline).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("password_command %q: %w", p.cmdline, err)
	}
	pw := strings.TrimRight(string(out), "\r\n")
	if pw == "" {
		return nil, nil, fmt.Errorf("password_command %q: produced empty output", p.cmdline)
	}
	return NewPasswordProvider(pw).Methods()
}
