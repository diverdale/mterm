package workspaces

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOpenMissingFileReturnsEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open missing file should not error: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatal("expected empty store")
	}
}

func TestSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("dev-day", []string{"sys-dev", "plex-ubuntu"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not persisted: %v", err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := s2.Get("dev-day")
	if !ok {
		t.Fatal("workspace missing after reload")
	}
	if !reflect.DeepEqual(w.Hosts, []string{"sys-dev", "plex-ubuntu"}) {
		t.Fatalf("hosts = %v, want [sys-dev plex-ubuntu]", w.Hosts)
	}
}

func TestSaveOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	s, _ := Open(path)
	if err := s.Save("name", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("name", []string{"x"}); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Get("name")
	if !reflect.DeepEqual(w.Hosts, []string{"x"}) {
		t.Fatalf("overwrite failed: got %v", w.Hosts)
	}
}

func TestSaveRejectsEmpty(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "w.json"))
	if err := s.Save("", []string{"a"}); err == nil {
		t.Error("empty name should fail")
	}
	if err := s.Save("name", nil); err == nil {
		t.Error("nil hosts should fail")
	}
}

func TestDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	s, _ := Open(path)
	_ = s.Save("a", []string{"x"})
	_ = s.Save("b", []string{"y"})
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("a"); ok {
		t.Error("deleted workspace should be gone")
	}
	if _, ok := s.Get("b"); !ok {
		t.Error("untouched workspace should remain")
	}
	// Delete missing name = no-op.
	if err := s.Delete("never-existed"); err != nil {
		t.Errorf("delete of missing name: %v", err)
	}
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	if _, ok := s.Get("x"); ok {
		t.Error("nil store should not report any entries")
	}
	if got := s.List(); got != nil {
		t.Errorf("nil store List() = %v, want nil", got)
	}
	if err := s.Save("x", []string{"a"}); err == nil {
		t.Error("nil store Save should error")
	}
	if err := s.Delete("x"); err == nil {
		t.Error("nil store Delete should error")
	}
}

func TestListSortedByNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	s, _ := Open(path)
	_ = s.Save("older", []string{"a"})
	time.Sleep(2 * time.Millisecond)
	_ = s.Save("newer", []string{"b"})
	got := s.List()
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Name != "newer" || got[1].Name != "older" {
		t.Fatalf("sort order wrong: %v", got)
	}
}
