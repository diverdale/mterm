package colors

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinsHasCommonNames(t *testing.T) {
	b := Builtins()
	for _, want := range []string{"limegreen", "hotpink", "red", "blue"} {
		if _, ok := b[want]; !ok {
			t.Errorf("Builtins missing %q", want)
		}
	}
}

func TestOpenMissingFallsBackToBuiltins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "colors.yaml")
	got, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["limegreen"]; !ok {
		t.Error("missing-file path should still expose built-ins")
	}
}

func TestOpenLowercasesKeysAndOverridesBuiltins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "colors.yaml")
	if err := os.WriteFile(path, []byte(`
Red: "#AA0000"
Limegreen: "#00FF00"
ProdRed: "#CC0000"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["red"] != "#AA0000" {
		t.Errorf("user 'Red' should override built-in red; got %q", got["red"])
	}
	if got["limegreen"] != "#00FF00" {
		t.Errorf("user override lost; got %q", got["limegreen"])
	}
	if got["prodred"] != "#CC0000" {
		t.Errorf("user-only name missing; got %q", got["prodred"])
	}
}

func TestResolveCaseInsensitive(t *testing.T) {
	colors := Builtins()
	// Builtins() doesn't lowercase its keys (Open does). Build a lowercased
	// version manually for this test.
	lc := map[string]string{}
	for k, v := range colors {
		lc[k] = v // already lowercase by convention in Builtins
	}
	for _, name := range []string{"limegreen", "LIMEGREEN", "  limegreen  "} {
		if got, ok := Resolve(lc, name); !ok || got != "#32CD32" {
			t.Errorf("Resolve(%q) = %q, ok=%v; want #32CD32", name, got, ok)
		}
	}
}

func TestResolveUnknown(t *testing.T) {
	if _, ok := Resolve(Builtins(), "no-such-color"); ok {
		t.Error("unknown name should return ok=false")
	}
}

func TestResolveNilMap(t *testing.T) {
	if _, ok := Resolve(nil, "anything"); ok {
		t.Error("nil map must not panic and must return ok=false")
	}
}
