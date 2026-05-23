package terminal

import (
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// sgrRE matches any ANSI SGR escape sequence (e.g. \x1b[31m, \x1b[0m, \x1b[m).
var sgrRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestWriteDoesNotBlockOnUnreadResponsePipe pins the bug that froze tabs
// running vi: xvt's CSI handler for "n" (Device Status Report) writes the
// reply to an internal io.Pipe with no buffer. Without a goroutine draining
// the response side, the first DSR makes vt.Write block forever — and since
// Terminal.Write holds t.mu, the snapshot worker starves and the tab hangs.
//
// The fix is to drain Terminal.Read on a background goroutine and forward
// those bytes back to the SSH session. This test models that drain and
// verifies Write returns within a reasonable time when a DSR comes through.
func TestWriteDoesNotBlockOnUnreadResponsePipe(t *testing.T) {
	term := New(80, 24)
	// Intentionally NOT calling term.Close in Cleanup: closing while the
	// drainer goroutine is still blocked inside vt.Read races with xvt's
	// internal `closed bool` (handlers in vt aren't synchronized). The
	// race is benign in production (Read returns io.EOF either way) but
	// the race detector flags it. The drainer goroutine simply leaks for
	// the rest of the test process — harmless.

	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := term.Read(buf); err != nil {
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		// CSI 6n = "report cursor position" — triggers a reply write into
		// the emulator's response pipe.
		_, _ = term.Write([]byte("\x1b[6n"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Terminal.Write blocked on a full response pipe — drainer not wired")
	}
}

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
	// Strip any ANSI SGR codes before comparing text content so this test
	// remains correct even when Render() emits styling.
	plain0 := sgrRE.ReplaceAllString(lines[0], "")
	plain1 := sgrRE.ReplaceAllString(lines[1], "")
	if !strings.HasPrefix(plain0, "abc") {
		t.Fatalf("row 0 = %q want abc...", lines[0])
	}
	if !strings.HasPrefix(plain1, "def") {
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

// TestRenderIncludesColor is the regression test that proves color information
// survives a Write/Render round-trip. A program that emits "\x1b[31mred\x1b[0m"
// (red text) must produce output from Render() that contains at least one ANSI
// SGR escape sequence — i.e. Render() is no longer plain text.
func TestRenderIncludesColor(t *testing.T) {
	term := New(20, 5)
	if _, err := term.Write([]byte("\x1b[31mred\x1b[0m")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := term.Render()
	if !sgrRE.MatchString(out) {
		t.Fatalf("Render() contains no ANSI SGR escape sequence; got %q", out)
	}
	// Also confirm the text content is present.
	plain := sgrRE.ReplaceAllString(out, "")
	if !strings.Contains(plain, "red") {
		t.Fatalf("Render() plain text does not contain 'red'; got %q", out)
	}
}

// reverseVideoRE matches the SGR reverse-video attribute (\x1b[7m) where 7
// appears as a complete parameter, not as a substring of another parameter
// (e.g. it must not match \x1b[37m or \x1b[27m).
var reverseVideoRE = regexp.MustCompile(`\x1b\[(?:\d+;)*7(?:;\d+)*m`)

// TestRenderDrawsCursor verifies that Render() draws a reverse-video block
// cursor at the cursor position when the cursor is visible (DECTCEM default).
func TestRenderDrawsCursor(t *testing.T) {
	term := New(20, 5)
	// Write some text so the cursor moves to a non-zero column.
	if _, err := term.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := term.Render()
	// The output must contain a reverse-video SGR sequence (\x1b[7m or
	// equivalent) because the cursor cell is rendered with reverse video.
	if !reverseVideoRE.MatchString(out) {
		t.Fatalf("Render() does not contain a reverse-video cursor; got %q", out)
	}
}

// TestHiddenCursorNotDrawn verifies that when the remote application hides the
// cursor via \x1b[?25l (DECTCEM reset), Render() does NOT emit a reverse-video
// cursor block.
func TestHiddenCursorNotDrawn(t *testing.T) {
	term := New(20, 5)
	// Write text and then hide the cursor.
	if _, err := term.Write([]byte("hello\x1b[?25l")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := term.Render()
	if reverseVideoRE.MatchString(out) {
		t.Fatalf("Render() drew a cursor even though DECTCEM is reset; got %q", out)
	}
}

// TestCursorDoesNotCorruptWideChar is the regression test for Fix 1: when the
// cursor lands on a wide-char continuation cell (the right half of a wide
// character such as 中), Render() must not destroy the wide character.
func TestCursorDoesNotCorruptWideChar(t *testing.T) {
	term := New(20, 3)
	term.Write([]byte("中\x08")) // wide char, then backspace -> cursor on continuation cell
	if r1 := term.Render(); !strings.Contains(r1, "中") {
		t.Fatalf("wide char missing before second render: %q", r1)
	}
	if r2 := term.Render(); !strings.Contains(r2, "中") {
		t.Fatalf("Render() corrupted the wide char: %q", r2)
	}
}
