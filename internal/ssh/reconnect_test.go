package ssh

import (
	"testing"
	"time"

	"mterm/internal/config"
)

func TestBackoffGrowsAndCaps(t *testing.T) {
	b := backoff{base: 100 * time.Millisecond, max: time.Second}
	got := []time.Duration{b.next(), b.next(), b.next(), b.next(), b.next()}
	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		time.Second, // capped
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoff[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBackoffResets(t *testing.T) {
	b := backoff{base: 100 * time.Millisecond, max: time.Second}
	b.next()
	b.next()
	b.reset()
	if d := b.next(); d != 100*time.Millisecond {
		t.Fatalf("after reset, next = %v, want 100ms", d)
	}
}

func TestReconnectRestoresSession(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	// Drop the server-side connection; the session loses its transport.
	srv.dropConns()

	// Reconnect should re-establish and return to Connected.
	if err := s.Reconnect(80, 24); err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	if s.State() != StateConnected {
		t.Fatalf("state after reconnect = %v, want Connected", s.State())
	}
	if _, err := s.Write([]byte("hi\n")); err != nil {
		t.Fatalf("write after reconnect: %v", err)
	}
}

func TestAutoReconnectStopsWhenClosed(t *testing.T) {
	host := config.Host{Name: "x", HostName: "127.0.0.1"}
	s := NewSession(host, noAuth{}, insecureHostKey())
	s.dialAddr = "127.0.0.1:1"
	s.Close() // user closes before any reconnect attempt

	done := make(chan error, 1)
	go func() { done <- s.AutoReconnect(80, 24, 0) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AutoReconnect on a closed session must return nil, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AutoReconnect did not stop promptly for a closed session")
	}
}

func TestAutoReconnectGivesUpAfterMaxAttempts(t *testing.T) {
	host := config.Host{Name: "x", HostName: "127.0.0.1"}
	s := NewSession(host, noAuth{}, insecureHostKey())
	s.dialAddr = "127.0.0.1:1" // nothing listening — every dial fails

	err := s.AutoReconnect(80, 24, 2)
	if err == nil {
		t.Fatal("AutoReconnect must return an error after exhausting attempts")
	}
	if s.State() != StateFailed {
		t.Fatalf("state after exhaustion = %v, want StateFailed", s.State())
	}
}
