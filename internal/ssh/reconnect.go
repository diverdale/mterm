package ssh

import "time"

// backoff produces exponentially growing delays capped at max.
type backoff struct {
	base    time.Duration
	max     time.Duration
	attempt int
}

func (b *backoff) next() time.Duration {
	d := b.base << b.attempt
	if d <= 0 || d > b.max {
		d = b.max
	}
	b.attempt++
	return d
}

func (b *backoff) reset() { b.attempt = 0 }

// Reconnect tears down any existing transport and re-runs Connect once. It
// reuses the session (state/host) so the owning tab stays in place.
func (s *Session) Reconnect(cols, rows int) error {
	s.mu.Lock()
	sess, client := s.sess, s.client
	s.sess, s.client, s.stdin, s.stdout = nil, nil, nil, nil
	s.state = StateReconnecting
	s.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
	if client != nil {
		client.Close()
	}
	return s.Connect(cols, rows)
}

// AutoReconnect retries Connect with exponential backoff until it succeeds, the
// user closes the session, or maxAttempts is reached. maxAttempts <= 0 means
// retry forever. It is intended to run in its own goroutine.
func (s *Session) AutoReconnect(cols, rows, maxAttempts int) error {
	b := backoff{base: 500 * time.Millisecond, max: 30 * time.Second}
	for attempt := 0; maxAttempts <= 0 || attempt < maxAttempts; attempt++ {
		if s.userClosed() {
			return nil
		}
		time.Sleep(b.next())
		if s.userClosed() {
			return nil
		}
		if err := s.Reconnect(cols, rows); err == nil {
			return nil
		}
	}
	s.setState(StateFailed, s.LastError())
	return s.LastError()
}
