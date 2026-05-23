package sessionlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "host", "20260523-120000.log")
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("parent dir not created: %v", err)
	}
}

func TestWriteAppendsAndFlushOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260523-120000.log")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write([]byte("hello ")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write([]byte("world")); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("file = %q, want %q", got, "hello world")
	}
}

func TestSecondOpenAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260523-120000.log")
	for _, w := range []string{"first ", "second"} {
		l, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.Write([]byte(w)); err != nil {
			t.Fatal(err)
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first second" {
		t.Fatalf("file = %q, want %q", got, "first second")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second Close should be a no-op, got: %v", err)
	}
}

func TestCloseOnNilLoggerIsSafe(t *testing.T) {
	var l *Logger // logging disabled path
	if err := l.Close(); err != nil {
		t.Fatalf("Close on nil Logger should be a no-op, got: %v", err)
	}
}

func TestWritePathReportsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	l, _ := Open(path)
	t.Cleanup(func() { l.Close() })
	if got := l.Path(); got != path {
		t.Fatalf("Path() = %q, want %q", got, path)
	}
}

func TestFileNameUsesUTCAndExpectedFormat(t *testing.T) {
	when := time.Date(2026, 5, 23, 14, 7, 9, 0, time.UTC)
	got := FileName(when)
	if got != "20260523-140709.log" {
		t.Fatalf("FileName = %q, want 20260523-140709.log", got)
	}
}

func TestPathForJoinsRootHostAndTimestamp(t *testing.T) {
	when := time.Date(2026, 5, 23, 14, 7, 9, 0, time.UTC)
	got := PathFor("/tmp/logs", "sys-dev", when)
	want := filepath.Join("/tmp/logs", "sys-dev", "20260523-140709.log")
	if got != want {
		t.Fatalf("PathFor = %q, want %q", got, want)
	}
}

func TestPathForEmptyHostFallsBackToPlaceholder(t *testing.T) {
	when := time.Date(2026, 5, 23, 14, 7, 9, 0, time.UTC)
	got := PathFor("/tmp/logs", "", when)
	if !strings.Contains(got, "_unknown") {
		t.Fatalf("PathFor empty host = %q, want to contain _unknown", got)
	}
}
