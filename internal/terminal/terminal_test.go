package terminal

import (
	"strings"
	"sync"
	"testing"
)

func TestWriteAndRenderPlainText(t *testing.T) {
	term := New(20, 5)
	if _, err := term.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := term.Render()
	if !strings.Contains(out, "hello") {
		t.Fatalf("Render() missing text, got %q", out)
	}
}

func TestRenderHasOneLinePerRow(t *testing.T) {
	term := New(20, 5)
	out := term.Render()
	if got := strings.Count(out, "\n"); got != 4 {
		t.Fatalf("want 4 newlines for 5 rows, got %d", got)
	}
}

func TestResizeChangesGeometry(t *testing.T) {
	term := New(20, 5)
	term.Resize(40, 10)
	w, h := term.Size()
	if w != 40 || h != 10 {
		t.Fatalf("Size() = %d,%d want 40,10", w, h)
	}
	if got := strings.Count(term.Render(), "\n"); got != 9 {
		t.Fatalf("want 9 newlines after resize, got %d", got)
	}
}

func TestCarriageReturnAndNewlineMoveCursor(t *testing.T) {
	term := New(20, 5)
	term.Write([]byte("abc\r\ndef"))
	lines := strings.Split(term.Render(), "\n")
	if !strings.HasPrefix(lines[0], "abc") {
		t.Fatalf("row 0 = %q want abc...", lines[0])
	}
	if !strings.HasPrefix(lines[1], "def") {
		t.Fatalf("row 1 = %q want def...", lines[1])
	}
}

// TestWideAndMultiByteRender is the core regression test for the wide/multi-byte
// rendering fix. It ensures that multi-byte UTF-8 runes (é) and wide CJK runes
// (中, display width 2) survive a Write/Render round-trip intact.
func TestWideAndMultiByteRender(t *testing.T) {
	const h = 5
	// Terminal wide enough: a(1) + é(1 display col, 2 bytes) + 中(2 display cols) + b(1) = 5
	term := New(20, h)
	if _, err := term.Write([]byte("aé中b")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := term.Render()

	// Must return exactly h rows.
	if got := strings.Count(out, "\n"); got != h-1 {
		t.Fatalf("want %d newlines for %d rows, got %d", h-1, h, got)
	}

	row0 := strings.Split(out, "\n")[0]
	for _, want := range []string{"a", "é", "中", "b"} {
		if !strings.Contains(row0, want) {
			t.Fatalf("row 0 %q does not contain %q", row0, want)
		}
	}
}

// TestCursorPositionAfterWrite verifies that CursorPosition reflects the column
// after writing ASCII text.
func TestCursorPositionAfterWrite(t *testing.T) {
	term := New(20, 5)
	if _, err := term.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	x, y := term.CursorPosition()
	if x != 3 || y != 0 {
		t.Fatalf("CursorPosition() = %d,%d want 3,0", x, y)
	}
}

// TestConcurrentWriteAndRender documents the concurrency-safety invariant:
// concurrent Write and Render calls must not race. Run with -race.
func TestConcurrentWriteAndRender(t *testing.T) {
	term := New(20, 5)
	var wg sync.WaitGroup
	const goroutines = 20
	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = term.Write([]byte("hello"))
		}()
		go func() {
			defer wg.Done()
			_ = term.Render()
		}()
	}
	wg.Wait()
}
