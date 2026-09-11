package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSnippetsMissingFile(t *testing.T) {
	s, err := LoadSnippets(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if len(s.ForHost("any")) != 0 {
		t.Fatalf("expected empty snippets, got %d", len(s.ForHost("any")))
	}
}

func TestLoadSnippetsGlobalAndHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snippets.yaml")
	err := os.WriteFile(path, []byte(`
snippets:
  - name: uptime
    send: uptime
host_snippets:
  prod-router:
    - name: show version
      send: show version
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	s, err := LoadSnippets(path)
	if err != nil {
		t.Fatalf("LoadSnippets: %v", err)
	}
	got := s.ForHost("prod-router")
	if len(got) != 2 {
		t.Fatalf("want 2 snippets, got %d", len(got))
	}
	if got[0].Name != "uptime" || got[0].Group != "Global" {
		t.Fatalf("first snippet: %+v", got[0])
	}
	if got[1].Name != "show version" || got[1].Group != "prod-router" {
		t.Fatalf("second snippet: %+v", got[1])
	}
	other := s.ForHost("lab-box")
	if len(other) != 1 || other[0].Name != "uptime" {
		t.Fatalf("other host should only see global: %+v", other)
	}
}

func TestLoadSnippetsRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snippets.yaml")
	err := os.WriteFile(path, []byte(`
snippets:
  - name: x
    send: y
    typo: z
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnippets(path); err == nil {
		t.Fatal("expected strict decode error for unknown field")
	}
}

func TestForHostSkipsEmptyEntries(t *testing.T) {
	s := Snippets{
		Global: []Snippet{{Name: "", Send: "x"}, {Name: "ok", Send: "y"}},
	}
	got := s.ForHost("h")
	if len(got) != 1 || got[0].Name != "ok" {
		t.Fatalf("got %+v", got)
	}
}
