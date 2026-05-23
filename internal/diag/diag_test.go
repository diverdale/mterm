package diag

import (
	"os"
	"strings"
	"testing"
)

func TestDumpGoroutinesWritesFile(t *testing.T) {
	path, err := DumpGoroutines("dttest")
	if err != nil {
		t.Fatalf("DumpGoroutines: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("dump file %s is empty", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Every Go goroutine stack starts with "goroutine N [...]:".
	if !strings.Contains(string(data), "goroutine ") {
		t.Fatalf("dump file does not look like a stack trace:\n%s", data)
	}
}
