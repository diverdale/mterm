package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapConfigDirCreatesMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := BootstrapConfigDir(); err != nil {
		t.Fatalf("BootstrapConfigDir: %v", err)
	}
	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("expected config dir to exist at %s: %v", dir, err)
	} else if !info.IsDir() {
		t.Fatalf("%s exists but is not a directory", dir)
	}
}

func TestBootstrapConfigDirIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// First call creates it.
	if err := BootstrapConfigDir(); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Second call must not error on existing dir.
	if err := BootstrapConfigDir(); err != nil {
		t.Fatalf("second call on existing dir: %v", err)
	}
}

func TestHostsFileUnderConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	got, err := HostsFile()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, ".config", "mterm", "hosts.yaml")
	if got != want {
		t.Fatalf("HostsFile() = %q, want %q", got, want)
	}
}
