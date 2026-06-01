// Package workspaces persists named tab sets so "yesterday's investigation"
// can be reopened with one keystroke. Each workspace is a list of host names
// (matching config.Host.Name) — the picker / connector path takes care of
// the actual connection on restore.
package workspaces

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Workspace is a single named entry.
type Workspace struct {
	Hosts   []string  `json:"hosts"`
	SavedAt time.Time `json:"savedAt"`
}

// Store holds all workspaces in memory and rewrites the JSON file on every
// mutation. Safe for concurrent use.
type Store struct {
	path string

	mu   sync.RWMutex
	data map[string]Workspace
}

// Open loads the JSON file at path (or returns an empty store if the file is
// missing). Any other error surfaces so the caller can warn and fall back to
// an empty in-memory store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]Workspace{}}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	if len(bytes) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(bytes, &s.data); err != nil {
		return s, err
	}
	return s, nil
}

// Save records hosts under name, overwriting any existing workspace by the
// same name, and persists the file. Empty name or empty hosts is rejected.
func (s *Store) Save(name string, hosts []string) error {
	if s == nil {
		return errors.New("workspaces store is nil")
	}
	if name == "" {
		return errors.New("workspace name is empty")
	}
	if len(hosts) == 0 {
		return errors.New("no hosts to save")
	}
	s.mu.Lock()
	clone := append([]string(nil), hosts...)
	s.data[name] = Workspace{Hosts: clone, SavedAt: time.Now()}
	snapshot := s.snapshotLocked()
	s.mu.Unlock()
	return writeAtomic(s.path, snapshot)
}

// Delete removes name (if present) and persists. Missing names are a no-op.
func (s *Store) Delete(name string) error {
	if s == nil {
		return errors.New("workspaces store is nil")
	}
	s.mu.Lock()
	if _, ok := s.data[name]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.data, name)
	snapshot := s.snapshotLocked()
	s.mu.Unlock()
	return writeAtomic(s.path, snapshot)
}

// Get returns the workspace for name and ok=true; ok=false when missing.
func (s *Store) Get(name string) (Workspace, bool) {
	if s == nil {
		return Workspace{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.data[name]
	return w, ok
}

// List returns every workspace sorted by save time, newest first. Empty
// slice when nothing saved.
func (s *Store) List() []NamedWorkspace {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NamedWorkspace, 0, len(s.data))
	for name, w := range s.data {
		out = append(out, NamedWorkspace{Name: name, Workspace: w})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SavedAt.After(out[j].SavedAt)
	})
	return out
}

// NamedWorkspace pairs the name with its workspace for List output.
type NamedWorkspace struct {
	Name string
	Workspace
}

func (s *Store) snapshotLocked() map[string]Workspace {
	out := make(map[string]Workspace, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

func writeAtomic(path string, m map[string]Workspace) error {
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
