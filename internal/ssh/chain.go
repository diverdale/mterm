package ssh

import "golang.org/x/crypto/ssh"

// ChainProvider returns an AuthProvider that concatenates auth methods from
// every provider in order. Errors from any individual provider are silently
// skipped — the chain is "try each, use what works" semantics. If every
// provider errors, the result is an empty Methods slice and the caller's
// SSH handshake will fail with "no supported methods remain."
//
// The first non-nil error is returned alongside the methods so the caller
// can surface it (e.g. "identity file is passphrase-protected") instead of
// the generic handshake error.
func ChainProvider(providers ...AuthProvider) AuthProvider {
	return chain(providers)
}

type chain []AuthProvider

func (c chain) Methods() ([]ssh.AuthMethod, func(), error) {
	var methods []ssh.AuthMethod
	var cleanups []func()
	var firstErr error
	for _, p := range c {
		if p == nil {
			continue
		}
		ms, cleanup, err := p.Methods()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		methods = append(methods, ms...)
		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
	}
	combined := func() {
		for _, c := range cleanups {
			c()
		}
	}
	if len(methods) == 0 {
		return nil, combined, firstErr
	}
	// Methods succeeded — swallow the per-provider error since auth has at
	// least one viable path. Encrypted-key errors surface only when there
	// is genuinely no other option.
	return methods, combined, nil
}
