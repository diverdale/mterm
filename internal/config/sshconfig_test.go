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

func TestLoadSSHConfigRemoteForward(t *testing.T) {
	path := writeTemp(t, "config", `
Host jump-box
    HostName 10.0.2.1
    User admin
    Port 22
    RemoteForward 2222 127.0.0.1:22
    LocalForward 9090 127.0.0.1:9090
    LocalForward 8443 127.0.0.1:443
`)
	hosts, err := loadSSHConfig(path)
	if err != nil {
		t.Fatalf("loadSSHConfig: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("want 1 host, got %d", len(hosts))
	}
	h := hosts[0]

	// Gap 1: RemoteForward produces a Forward with Type == ForwardRemote.
	var remotes []Forward
	for _, f := range h.Forwards {
		if f.Type == ForwardRemote {
			remotes = append(remotes, f)
		}
	}
	if len(remotes) != 1 {
		t.Fatalf("want 1 RemoteForward, got %d", len(remotes))
	}
	rf := remotes[0]
	if rf.BindPort != 2222 {
		t.Errorf("RemoteForward BindPort: want 2222, got %d", rf.BindPort)
	}
	if rf.DialAddr != "127.0.0.1" {
		t.Errorf("RemoteForward DialAddr: want 127.0.0.1, got %q", rf.DialAddr)
	}
	if rf.DialPort != 22 {
		t.Errorf("RemoteForward DialPort: want 22, got %d", rf.DialPort)
	}

	// Gap 2: Two LocalForward lines produce two Forward entries of type ForwardLocal.
	var locals []Forward
	for _, f := range h.Forwards {
		if f.Type == ForwardLocal {
			locals = append(locals, f)
		}
	}
	if len(locals) != 2 {
		t.Fatalf("want 2 LocalForwards, got %d", len(locals))
	}
}

func TestLoadSSHConfigDefaults(t *testing.T) {
	// Gaps 3, 4, 6: HostName defaults to alias, Port defaults to 22,
	// and LocalForward with plain "port host:port" produces BindAddr == "".
	path := writeTemp(t, "config", `
Host bare-host
    User ops
    LocalForward 8080 127.0.0.1:80
`)
	hosts, err := loadSSHConfig(path)
	if err != nil {
		t.Fatalf("loadSSHConfig: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("want 1 host, got %d", len(hosts))
	}
	h := hosts[0]

	// Gap 3: HostName defaults to the alias when no HostName directive is present.
	if h.HostName != h.Name {
		t.Errorf("HostName default: want %q (alias), got %q", h.Name, h.HostName)
	}

	// Gap 4: Port defaults to 22 when no Port directive is present.
	if h.Port != 22 {
		t.Errorf("Port default: want 22, got %d", h.Port)
	}

	// Gap 6: LocalForward "8080 127.0.0.1:80" produces BindAddr == "".
	if len(h.Forwards) != 1 {
		t.Fatalf("want 1 forward, got %d", len(h.Forwards))
	}
	if h.Forwards[0].BindAddr != "" {
		t.Errorf("BindAddr: want empty string, got %q", h.Forwards[0].BindAddr)
	}
	if h.Forwards[0].BindPort != 8080 {
		t.Errorf("BindPort: want 8080, got %d", h.Forwards[0].BindPort)
	}
	if h.Forwards[0].DialAddr != "127.0.0.1" {
		t.Errorf("DialAddr: want 127.0.0.1, got %q", h.Forwards[0].DialAddr)
	}
	if h.Forwards[0].DialPort != 80 {
		t.Errorf("DialPort: want 80, got %d", h.Forwards[0].DialPort)
	}
}

func TestLoadSSHConfigDeduplication(t *testing.T) {
	// Gap 5: The same alias in two Host blocks yields exactly one host (first wins).
	path := writeTemp(t, "config", `
Host dup-host
    HostName 10.0.0.1
    User first
    Port 2222

Host dup-host
    HostName 10.0.0.2
    User second
    Port 3333
`)
	hosts, err := loadSSHConfig(path)
	if err != nil {
		t.Fatalf("loadSSHConfig: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("deduplication: want 1 host, got %d", len(hosts))
	}
	h := hosts[0]
	if h.User != "first" {
		t.Errorf("deduplication: want first occurrence (User=first), got User=%q", h.User)
	}
	if h.Port != 2222 {
		t.Errorf("deduplication: want first occurrence (Port=2222), got Port=%d", h.Port)
	}
}
