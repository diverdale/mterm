package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMissingFileReturnsEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open missing file should not error: %v", err)
	}
	if len(s.All()) != 0 {
		t.Fatalf("expected empty store, got %v", s.All())
	}
}

func TestRecordAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	if err := s.RecordConnect("sys-dev", want); err != nil {
		t.Fatal(err)
	}

	// File should exist.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history file not written: %v", err)
	}

	// Reload from disk and verify the entry round-tripped.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.LastConnected("sys-dev")
	if !ok {
		t.Fatal("expected sys-dev in reloaded store")
	}
	if !got.Equal(want) {
		t.Fatalf("reloaded time = %v, want %v", got, want)
	}
}

func TestNilStoreRecordIsNoOp(t *testing.T) {
	var s *Store
	if err := s.RecordConnect("anywhere", time.Now()); err != nil {
		t.Fatalf("nil store should accept RecordConnect; got %v", err)
	}
}

func TestLastConnectedUnknownHostMissing(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "history.json"))
	if _, ok := s.LastConnected("ghost"); ok {
		t.Fatal("unknown host should return ok=false")
	}
}

func TestFormatRelative(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{0, "just now"},
		{30 * time.Second, "just now"},
		{2 * time.Minute, "2m ago"},
		{59 * time.Minute, "59m ago"},
		{2 * time.Hour, "2h ago"},
		{23 * time.Hour, "23h ago"},
		{3 * 24 * time.Hour, "3d ago"},
		{6 * 24 * time.Hour, "6d ago"},
		{2 * 7 * 24 * time.Hour, "2w ago"},
		{2 * 30 * 24 * time.Hour, "2mo ago"},
	}
	for _, tc := range cases {
		got := FormatRelative(now, now.Add(-tc.ago))
		if got != tc.want {
			t.Errorf("FormatRelative(%v ago) = %q, want %q", tc.ago, got, tc.want)
		}
	}
}

func TestFormatRelativeZeroIsDash(t *testing.T) {
	if got := FormatRelative(time.Now(), time.Time{}); got != "—" {
		t.Errorf("zero time should render as em-dash; got %q", got)
	}
}
