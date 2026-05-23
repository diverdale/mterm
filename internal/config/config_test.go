package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMergeOverlayAndNewHost(t *testing.T) {
	sshPath := writeTemp(t, "config", `
Host prod-web
    HostName 10.0.1.4
    User deploy
`)
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - name: prod-web
    group: production
    tags: [web, critical]
  - name: lab-box
    address: 10.9.0.12
    user: dale
    port: 22
    group: lab
    tags: [scratch]
    forwards:
      - type: local
        bindport: 8080
        dialaddr: 127.0.0.1
        dialport: 80
`)

	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatalf("loadAndMerge: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	byName := map[string]Host{}
	for _, h := range res.Hosts {
		byName[h.Name] = h
	}

	web, ok := byName["prod-web"]
	if !ok {
		t.Fatal("prod-web missing")
	}
	if web.HostName != "10.0.1.4" || web.User != "deploy" {
		t.Fatalf("overlay must not clobber ssh_config base: %+v", web)
	}
	if web.Group != "production" || len(web.Tags) != 2 {
		t.Fatalf("overlay group/tags not applied: %+v", web)
	}
	if web.Source != SourceSSHConfig {
		t.Fatalf("overlaid host keeps ssh_config source, got %v", web.Source)
	}

	lab, ok := byName["lab-box"]
	if !ok {
		t.Fatal("lab-box missing")
	}
	if lab.Source != SourceMterm || lab.HostName != "10.9.0.12" {
		t.Fatalf("mterm-only host wrong: %+v", lab)
	}
	if len(lab.Forwards) != 1 || lab.Forwards[0].BindPort != 8080 {
		t.Fatalf("mterm forwards not parsed: %+v", lab.Forwards)
	}
}

func TestMergeBadYAMLIsWarningNotFatal(t *testing.T) {
	sshPath := writeTemp(t, "config", "Host a\n  HostName 1.2.3.4\n")
	mtermPath := writeTemp(t, "hosts.yaml", "hosts: [ this is not valid")

	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatalf("bad YAML must not be fatal, got %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("want a warning for bad YAML")
	}
	if len(res.Hosts) != 1 {
		t.Fatalf("ssh_config host must still load, got %d", len(res.Hosts))
	}
}

func TestMergeEmptyNameHostIsWarning(t *testing.T) {
	sshPath := writeTemp(t, "config", "")
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - name: ""
    group: orphan
  - name: valid-host
    address: 1.2.3.4
`)

	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatalf("loadAndMerge: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("want a warning for host with empty name, got none")
	}
	for _, h := range res.Hosts {
		if h.Name == "" {
			t.Fatalf("host with empty name must not be added to results: %+v", h)
		}
	}
	if len(res.Hosts) != 1 {
		t.Fatalf("want exactly 1 host (valid-host), got %d: %+v", len(res.Hosts), res.Hosts)
	}
}

func TestMergeOverlayUserPortAndEmptyGroup(t *testing.T) {
	sshPath := writeTemp(t, "config", `
Host bastion
    HostName bastion.example.com
    User ubuntu
    Port 22
`)
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - name: bastion
    user: ec2-user
    port: 2222
`)

	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatalf("loadAndMerge: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}

	var bastion *Host
	for i := range res.Hosts {
		if res.Hosts[i].Name == "bastion" {
			bastion = &res.Hosts[i]
			break
		}
	}
	if bastion == nil {
		t.Fatal("bastion host missing from results")
	}
	if bastion.User != "ec2-user" {
		t.Fatalf("mterm user override not applied: got %q, want %q", bastion.User, "ec2-user")
	}
	if bastion.Port != 2222 {
		t.Fatalf("mterm port override not applied: got %d, want 2222", bastion.Port)
	}
	// mterm entry omits group: existing Group (empty string) must be unchanged.
	if bastion.Group != "" {
		t.Fatalf("applyOverlay must not clobber existing Group with empty mterm value: got %q", bastion.Group)
	}
}

func TestMergeSortedByGroupThenName(t *testing.T) {
	sshPath := writeTemp(t, "config", "")
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - {name: zeta, address: z, group: alpha}
  - {name: alpha, address: a, group: beta}
  - {name: beta, address: b, group: alpha}
`)
	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	for _, h := range res.Hosts {
		order = append(order, h.Name)
	}
	want := []string{"beta", "zeta", "alpha"} // group alpha first, then name
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("sort order = %v, want %v", order, want)
		}
	}
}

func TestLoadAndMergeBorderColorValid(t *testing.T) {
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: prod
    bordercolor: "#FF3344"
  - name: dev
    bordercolor: "#F33"
  - name: plain
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range res.Hosts {
		got[h.Name] = h.BorderColor
	}
	want := map[string]string{"prod": "#FF3344", "dev": "#F33", "plain": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BorderColor map = %v, want %v", got, want)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", res.Warnings)
	}
}

func TestLoadAndMergeBorderColorInvalidWarns(t *testing.T) {
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: badhost
    bordercolor: "red"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 1 || res.Hosts[0].Name != "badhost" {
		t.Fatalf("hosts = %+v, want one badhost", res.Hosts)
	}
	if res.Hosts[0].BorderColor != "" {
		t.Fatalf("BorderColor = %q, want \"\" on invalid", res.Hosts[0].BorderColor)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("want a warning for invalid bordercolor, got none")
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "badhost") && strings.Contains(w, "bordercolor") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v, want one naming host+bordercolor", res.Warnings)
	}
}

func TestLoadAndMergeUnknownYAMLKeyWarns(t *testing.T) {
	// A misspelled or wrong-case key (the classic "hostName" vs "hostname")
	// must now surface as a warning instead of being silently dropped — that
	// silent drop was the cause of the v1 connect-failure bug where users
	// wrote camelCase keys and the host ended up with empty fields.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: typo-host
    hostName: 10.0.0.5
    user: dale
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath)
	if err != nil {
		t.Fatalf("loadAndMerge: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning naming the unknown field hostName")
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "hostName") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v, want one mentioning hostName", res.Warnings)
	}
}

func TestLoadAndMergeNameAliases(t *testing.T) {
	// `name`, `hostname`, and `host` are interchangeable spellings for the
	// friendly label. `address` is the dial target. Verify each spelling
	// produces the same canonical Host.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: via-name
    address: 10.0.0.1
  - hostname: via-hostname
    address: 10.0.0.2
  - host: via-host
    address: 10.0.0.3
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	want := map[string]string{
		"via-name":     "10.0.0.1",
		"via-hostname": "10.0.0.2",
		"via-host":     "10.0.0.3",
	}
	got := map[string]string{}
	for _, h := range res.Hosts {
		got[h.Name] = h.HostName
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLoadAndMergeAddressFallsBackToName(t *testing.T) {
	// No `address` set → dial target falls back to Name (DNS resolves it).
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: just-a-name
    user: dale
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 1 || res.Hosts[0].HostName != "just-a-name" {
		t.Fatalf("HostName should fall back to Name; got %+v", res.Hosts)
	}
}

func TestLoadAndMergeNameAliasConflictWarns(t *testing.T) {
	// `name: foo` and `hostname: bar` together: name wins, but a warning
	// surfaces so the user knows the hostname field was dropped.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: winner
    hostname: loser
    address: 10.0.0.1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 1 || res.Hosts[0].Name != "winner" {
		t.Fatalf("Name should be 'winner'; got %+v", res.Hosts)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "winner") && strings.Contains(w, "hostname=loser") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v, want one naming the conflict", res.Warnings)
	}
}
