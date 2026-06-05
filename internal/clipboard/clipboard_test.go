package clipboard

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestOSC52SequenceWrapsBase64Payload(t *testing.T) {
	got := OSC52Sequence("hello, world")
	wantPrefix := "\x1b]52;c;"
	wantSuffix := "\x1b\\"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("missing OSC 52 prefix: got %q", got[:8])
	}
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("missing ST terminator: got %q", got[len(got)-4:])
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(got, wantPrefix), wantSuffix)
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload %q not valid base64: %v", payload, err)
	}
	if string(decoded) != "hello, world" {
		t.Fatalf("decoded = %q, want %q", decoded, "hello, world")
	}
}

func TestOSC52SequenceEmptyStringYieldsEmptyPayload(t *testing.T) {
	got := OSC52Sequence("")
	want := "\x1b]52;c;\x1b\\"
	if got != want {
		t.Fatalf("empty-string sequence = %q, want %q", got, want)
	}
}

func TestOSC52SequencePreservesUnicodeBytes(t *testing.T) {
	const in = "héllo · 世界"
	got := OSC52Sequence(in)
	payload := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]52;c;"), "\x1b\\")
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != in {
		t.Fatalf("decoded = %q, want %q", decoded, in)
	}
}

func TestCopyWritesOSC52ToProvidedWriter(t *testing.T) {
	var buf bytes.Buffer
	// We don't care whether the native side succeeds in CI; the
	// contract is "either path counts." OSC 52 to our buffer is
	// always observable and always succeeds.
	_ = Copy(&buf, "ping")
	if !strings.Contains(buf.String(), OSC52Sequence("ping")) {
		t.Fatalf("buffer missing OSC 52 sequence for \"ping\": %q", buf.String())
	}
}

func TestCopyReturnsErrorOnlyWhenBothPathsFail(t *testing.T) {
	// A nil writer makes OSC 52 fail. With no DISPLAY/WAYLAND_DISPLAY
	// set (or no helper), native also fails — so Copy must return an
	// error. We can't reliably set the env without disrupting other
	// tests, so this case is the only one we assert here.
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("PATH", "") // also block native helper detection
	err := Copy(nil, "anything")
	if err == nil {
		t.Fatal("Copy(nil writer, no env) should error — both paths broken")
	}
}
