package ssh

import (
	"testing"
	"time"
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
