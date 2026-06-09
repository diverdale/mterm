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

	res, err := loadAndMerge(sshPath, mtermPath, nil)
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

	res, err := loadAndMerge(sshPath, mtermPath, nil)
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

	res, err := loadAndMerge(sshPath, mtermPath, nil)
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

	res, err := loadAndMerge(sshPath, mtermPath, nil)
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

func TestMergeRespectsYAMLOrder(t *testing.T) {
	// Sort is hierarchical by intentOrder: groups appear in the order their
	// first host appeared, and hosts inside a group appear in yaml order.
	// Alphabetical sort within a group is gone — explicit user intent wins.
	//
	// File order:
	//   zeta (group: alpha)  → intentOrder 0 → prefixMin[alpha] = 0
	//   alpha (group: beta)  → intentOrder 1 → prefixMin[beta]  = 1
	//   beta (group: alpha)  → intentOrder 2 (alpha already min 0)
	//
	// Sort: group alpha (0) before group beta (1); within alpha, zeta
	// before beta (yaml order, NOT alphabetical).
	sshPath := writeTemp(t, "config", "")
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - {name: zeta, address: z, group: alpha}
  - {name: alpha, address: a, group: beta}
  - {name: beta, address: b, group: alpha}
`)
	res, err := loadAndMerge(sshPath, mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, h := range res.Hosts {
		got = append(got, h.Name)
	}
	want := []string{"zeta", "beta", "alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sort order = %v, want %v", got, want)
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
	res, err := loadAndMerge("", mtermPath, nil)
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

func TestLoadAndMergeBorderColorResolvesNamedColor(t *testing.T) {
	// With a non-nil colors map, non-hex bordercolor values resolve
	// through it. Hex still wins for entries that look hex.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: prod
    bordercolor: limegreen
  - name: staging
    bordercolor: "Hotpink"
  - name: literal
    bordercolor: "#FF3344"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	colors := map[string]string{
		"limegreen": "#32CD32",
		"hotpink":   "#FF69B4",
	}
	res, err := loadAndMerge("", mtermPath, colors)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	got := map[string]string{}
	for _, h := range res.Hosts {
		got[h.Name] = h.BorderColor
	}
	want := map[string]string{
		"prod":    "#32CD32",
		"staging": "#FF69B4",
		"literal": "#FF3344",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BorderColor map = %v, want %v", got, want)
	}
}

func TestLoadAndMergeBorderColorUnknownNameWarns(t *testing.T) {
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: badname
    bordercolor: foobar
`), 0o600); err != nil {
		t.Fatal(err)
	}
	colors := map[string]string{"limegreen": "#32CD32"}
	res, err := loadAndMerge("", mtermPath, colors)
	if err != nil {
		t.Fatal(err)
	}
	if res.Hosts[0].BorderColor != "" {
		t.Fatalf("unknown name should clear BorderColor; got %q", res.Hosts[0].BorderColor)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning for unknown color name")
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "foobar") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warning should name the bad value; got %v", res.Warnings)
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
	res, err := loadAndMerge("", mtermPath, nil)
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
	res, err := loadAndMerge("", mtermPath, nil)
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
	res, err := loadAndMerge("", mtermPath, nil)
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
	res, err := loadAndMerge("", mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 1 || res.Hosts[0].HostName != "just-a-name" {
		t.Fatalf("HostName should fall back to Name; got %+v", res.Hosts)
	}
}

func TestLoadAndMergeOnConnectFlowsThrough(t *testing.T) {
	// `on_connect: [...]` set in hosts.yaml must reach
	// config.Host.OnConnect verbatim from both flat and nested forms.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`groups:
  - name: Work
    hosts:
      - name: nested-host
        address: 10.0.0.1
        on_connect:
          - "tmux new -A -s mterm-work"
          - "cd ~/proj"
hosts:
  - name: flat-host
    address: 10.0.0.2
    on_connect: ["screen -DR"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	got := map[string][]string{}
	for _, h := range res.Hosts {
		got[h.Name] = h.OnConnect
	}
	want := map[string][]string{
		"nested-host": {"tmux new -A -s mterm-work", "cd ~/proj"},
		"flat-host":   {"screen -DR"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OnConnect map = %v, want %v", got, want)
	}
}

func TestLoadAndMergeIdentityFileFlowsThrough(t *testing.T) {
	// `identityfile` set in hosts.yaml must reach config.Host.IdentityFile
	// in both forms: flat and nested. ssh_config-loaded values also
	// survive because the merge layer doesn't touch them.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`groups:
  - name: Work
    hosts:
      - name: nested-host
        address: 10.0.0.1
        identityfile: ~/.ssh/work-key
hosts:
  - name: flat-host
    address: 10.0.0.2
    identityfile: /tmp/explicit-key
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	got := map[string]string{}
	for _, h := range res.Hosts {
		got[h.Name] = h.IdentityFile
	}
	want := map[string]string{
		"nested-host": "~/.ssh/work-key",
		"flat-host":   "/tmp/explicit-key",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IdentityFile map = %v, want %v", got, want)
	}
}

func TestLoadAndMergeProxyJumpFlowsThrough(t *testing.T) {
	// `proxy_jump:` in hosts.yaml must reach config.Host.ProxyJump for both
	// the flat and nested forms.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`groups:
  - name: Work
    hosts:
      - name: end-device
        address: 10.0.0.5
        proxy_jump: jump-host
hosts:
  - name: jump-host
    address: jump.example.com
  - name: literal-proxy
    address: 10.0.0.6
    proxy_jump: alice@bastion.example.com:2222
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	got := map[string]string{}
	for _, h := range res.Hosts {
		got[h.Name] = h.ProxyJump
	}
	want := map[string]string{
		"end-device":    "jump-host",
		"jump-host":     "",
		"literal-proxy": "alice@bastion.example.com:2222",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProxyJump map = %v, want %v", got, want)
	}
}

func TestLoadAndMergeLogTriState(t *testing.T) {
	// Default = on (nil Log → Logging() == true).
	// Explicit log: false → off.
	// Explicit log: true → on.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: default-on
    address: 10.0.0.1
  - name: explicit-off
    address: 10.0.0.2
    log: false
  - name: explicit-on
    address: 10.0.0.3
    log: true
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"default-on":   true,
		"explicit-off": false,
		"explicit-on":  true,
	}
	for _, h := range res.Hosts {
		if want[h.Name] != h.Logging() {
			t.Errorf("host %q Logging() = %v, want %v", h.Name, h.Logging(), want[h.Name])
		}
	}
}

func TestLoadAndMergeNestedGroupsSchema(t *testing.T) {
	// Structured nested groups: yaml hierarchy becomes the slash-delimited
	// Group field, and yaml order is preserved in the result.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`groups:
  - name: Work
    groups:
      - name: Lab
        hosts:
          - name: sao-dev
            address: 10.122.26.36
            user: dale
      - name: MSFT
        hosts:
          - name: msft-optical-1
            address: 10.122.161.81
            user: administrator
  - name: Home
    groups:
      - name: Media
        hosts:
          - name: plex
            address: 192.168.2.50
          - name: binarr
            address: 192.168.2.12
      - name: Development
        hosts:
          - name: sys-dev
            address: 192.168.2.20
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}

	// Verify each host has the correct slash-delimited Group derived from
	// its position in the tree.
	want := map[string]string{
		"sao-dev":        "Work/Lab",
		"msft-optical-1": "Work/MSFT",
		"plex":           "Home/Media",
		"binarr":         "Home/Media",
		"sys-dev":        "Home/Development",
	}
	got := map[string]string{}
	for _, h := range res.Hosts {
		got[h.Name] = h.Group
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Group paths = %v, want %v", got, want)
	}

	// Verify yaml order is preserved: Work block first (Lab before MSFT),
	// then Home block (Media before Development; plex before binarr).
	names := []string{}
	for _, h := range res.Hosts {
		names = append(names, h.Name)
	}
	wantOrder := []string{"sao-dev", "msft-optical-1", "plex", "binarr", "sys-dev"}
	if !reflect.DeepEqual(names, wantOrder) {
		t.Fatalf("order = %v, want %v", names, wantOrder)
	}
}

func TestLoadAndMergeMixedFlatAndNestedSchema(t *testing.T) {
	// Flat `hosts:` entries with slash-delimited groups must coexist with
	// the structured `groups:` form. Hosts of the same group from both
	// sources land in the same picker section.
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`groups:
  - name: Home
    groups:
      - name: Media
        hosts:
          - name: plex
            address: 1.1.1.1
hosts:
  - name: binarr
    address: 2.2.2.2
    group: Home/Media
`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadAndMerge("", mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 2 {
		t.Fatalf("hosts = %d, want 2", len(res.Hosts))
	}
	// Both should share group "Home/Media" and be adjacent.
	if res.Hosts[0].Group != "Home/Media" || res.Hosts[1].Group != "Home/Media" {
		t.Fatalf("expected both hosts in Home/Media, got %+v", res.Hosts)
	}
	// plex was declared first (nested) → comes before binarr (flat).
	if res.Hosts[0].Name != "plex" || res.Hosts[1].Name != "binarr" {
		t.Fatalf("order = %s,%s; want plex,binarr", res.Hosts[0].Name, res.Hosts[1].Name)
	}
}

func TestLoadAndMergeSSHConfigHostsTrailYAMLDeclared(t *testing.T) {
	// ssh_config hosts with no yaml counterpart must sort after all
	// yaml-declared hosts so the user's explicit organization wins.
	sshPath := writeTemp(t, "config", `
Host ssh-only-host
    HostName ssh-only-host
`)
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - name: yaml-host
    address: 1.1.1.1
    group: Work
`)
	res, err := loadAndMerge(sshPath, mtermPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, h := range res.Hosts {
		names = append(names, h.Name)
	}
	if !reflect.DeepEqual(names, []string{"yaml-host", "ssh-only-host"}) {
		t.Fatalf("order = %v, want [yaml-host ssh-only-host]", names)
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
	res, err := loadAndMerge("", mtermPath, nil)
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
