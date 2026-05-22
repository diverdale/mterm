package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadSSHConfig(t *testing.T) {
	path := writeTemp(t, "config", `
Host prod-web
    HostName 10.0.1.4
    User deploy
    Port 2222
    LocalForward 8080 127.0.0.1:80

Host *
    User ignored
`)
	hosts, err := loadSSHConfig(path)
	if err != nil {
		t.Fatalf("loadSSHConfig: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("want 1 host (wildcard skipped), got %d", len(hosts))
	}
	h := hosts[0]
	if h.Name != "prod-web" || h.HostName != "10.0.1.4" || h.User != "deploy" || h.Port != 2222 {
		t.Fatalf("unexpected host: %+v", h)
	}
	if h.Source != SourceSSHConfig {
		t.Fatalf("want SourceSSHConfig, got %v", h.Source)
	}
	if len(h.Forwards) != 1 {
		t.Fatalf("want 1 forward, got %d", len(h.Forwards))
	}
	f := h.Forwards[0]
	if f.Type != ForwardLocal || f.BindPort != 8080 || f.DialAddr != "127.0.0.1" || f.DialPort != 80 {
		t.Fatalf("unexpected forward: %+v", f)
	}
}

func TestLoadSSHConfigMissingFile(t *testing.T) {
	hosts, err := loadSSHConfig(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing file must not error, got %v", err)
	}
	if hosts != nil {
		t.Fatalf("want nil hosts, got %v", hosts)
	}
}
