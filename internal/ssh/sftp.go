package ssh

import (
	"errors"
	"sync"

	"github.com/pkg/sftp"
)

// sftpHolder is bolted onto Session.mu and lazily opens a single SFTP
// subsystem channel per connected Session. Multiple file-browser
// operations share the same client; closing the SSH session also closes
// the SFTP client.
//
// We keep it as a separate file so the existing session.go stays focused
// on the interactive PTY path, but it's still part of the Session value —
// the SFTP() method below opens or returns the cached client.
var (
	errSessionNotConnected = errors.New("ssh: session not connected")
	errSessionClosed       = errors.New("ssh: session closed")
)

// sftpState lives on Session as a non-exported field; declared here so the
// Session struct in session.go stays clean.
type sftpState struct {
	once   sync.Once
	client *sftp.Client
	err    error
}

// SFTP returns a sftp.Client backed by the session's existing SSH transport.
// The first call lazily opens the SFTP subsystem; subsequent calls return
// the same client. After s.Close(), all calls return errSessionClosed.
//
// The returned client is goroutine-safe — pkg/sftp serializes its own
// requests internally — so it's fine to call ReadDir/Open/Create from
// multiple goroutines simultaneously.
func (s *Session) SFTP() (*sftp.Client, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errSessionClosed
	}
	c := s.client
	if c == nil {
		s.mu.Unlock()
		return nil, errSessionNotConnected
	}
	st := &s.sftpCache
	s.mu.Unlock()

	st.once.Do(func() {
		st.client, st.err = sftp.NewClient(c)
	})
	return st.client, st.err
}

// closeSFTP is invoked by Close() to tear down the cached client. Safe to
// call when no client has ever been opened.
func (s *Session) closeSFTP() {
	if s.sftpCache.client != nil {
		_ = s.sftpCache.client.Close()
		s.sftpCache.client = nil
	}
}
