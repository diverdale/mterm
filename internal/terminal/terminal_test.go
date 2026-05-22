package terminal

import (
	"strings"
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
