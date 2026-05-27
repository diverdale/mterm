// Package history persists per-host connection history (currently just the
// last-connected timestamp) so the picker can show "● 2h ago" decorations.
// The store is a tiny JSON file under ~/.config/<app>/history.json that
// gets atomically rewritten on every connect.
package history

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store holds the connection history in memory and writes it back to disk on
// every recorded connect. Safe for concurrent use.
type Store struct {
	path string

	mu      sync.RWMutex
	entries map[string]time.Time // host name → last successful connect
}

// Open loads history from path (creating an empty store if the file is
// missing or unreadable). Errors other than "file not found" surface — the
// caller can warn and fall back to a fresh store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, entries: map[string]time.Time{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.entries); err != nil {
		return s, err
	}
	return s, nil
}

// LastConnected returns the last-connect timestamp for hostname and whether
// any history exists for that host. Zero time + false when never connected.
func (s *Store) LastConnected(hostname string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.entries[hostname]
	return t, ok
}

// All returns a copy of the full history map. Useful for the picker to read
// once per render without locking the store per row.
func (s *Store) All() map[string]time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]time.Time, len(s.entries))
	for k, v := range s.entries {
		out[k] = v
	}
	return out
}

// RecordConnect stamps hostname with t and atomically rewrites the file.
// A nil store is a no-op so callers can pass nil to disable persistence.
func (s *Store) RecordConnect(hostname string, t time.Time) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.entries[hostname] = t
	snapshot := make(map[string]time.Time, len(s.entries))
	for k, v := range s.entries {
		snapshot[k] = v
	}
	s.mu.Unlock()
	return writeAtomic(s.path, snapshot)
}

// writeAtomic marshals m to JSON and replaces path with the result via a
// rename so a partial write can't corrupt the file.
func writeAtomic(path string, m map[string]time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// FormatRelative renders a time as a compact human-readable "5m ago" style
// string. Zero time returns "—". Future times return "just now".
func FormatRelative(now, t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	if d < 0 || d < 60*time.Second {
		return "just now"
	}
	switch {
	case d < time.Hour:
		return formatN(int(d/time.Minute), "m")
	case d < 24*time.Hour:
		return formatN(int(d/time.Hour), "h")
	case d < 7*24*time.Hour:
		return formatN(int(d/(24*time.Hour)), "d")
	case d < 30*24*time.Hour:
		return formatN(int(d/(7*24*time.Hour)), "w")
	default:
		return formatN(int(d/(30*24*time.Hour)), "mo")
	}
}

func formatN(n int, unit string) string {
	if n < 1 {
		n = 1
	}
	return itoa(n) + unit + " ago"
}

// itoa avoids pulling in strconv for one int — local helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
