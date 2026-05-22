# mterm SSH TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `mterm`, a Go terminal UI that lists SSH hosts and opens each as a live, full-fidelity tabbed terminal session.

**Architecture:** Four layered packages. `config` merges `~/.ssh/config` and an mterm YAML file into `[]Host`. `ssh` turns a `Host` into a live `Session` (native `x/crypto/ssh`, PTY, auto-reconnect, port forwards). `terminal` wraps a VT emulator so a byte stream renders as a screen grid. `tui` (Bubble Tea) composes them: a fuzzy host picker plus tabbed sessions with a `Ctrl-B` prefix key. A 30 fps render ticker coalesces session output.

**Tech Stack:** Go 1.25, Bubble Tea / Bubbles / Lip Gloss, `golang.org/x/crypto/ssh`, `charmbracelet/x/vt`, `kevinburke/ssh_config`, `gopkg.in/yaml.v3`, `atotto/clipboard`. Tests: standard `testing` + `charmbracelet/x/exp/teatest`.

**Design source:** `docs/superpowers/specs/2026-05-22-mterm-ssh-tui-design.md`

---

## Conventions

- **Module path:** `mterm` (bare — local project). Imports are `mterm/internal/...`.
- **Go version:** 1.25 (`go.mod` declares `go 1.25`).
- **Commits:** Conventional Commits (`feat:`, `test:`, `chore:`). Commit after every task.
- **Test command:** `go test ./...` runs everything; per-task commands are given.
- Run `go vet ./...` and `gofmt -l .` before each commit; both must be clean.

## Library-API verification

Two libraries have version-sensitive APIs. **Before Task 4 and Task 11**, verify
the current API with the context7 MCP tool (`resolve-library-id` then
`query-docs`) or `go doc`:

- **Task 4** — `github.com/charmbracelet/x/vt`: confirm the emulator constructor,
  the `io.Writer` feed method, resize, cell/line access, and cursor position.
  The wrapper's *public interface* (defined in Task 4) is the contract the rest
  of the plan depends on — keep that stable even if the internal x/vt calls
  differ from what is shown.
- **Task 11** — `github.com/charmbracelet/x/exp/teatest`: confirm
  `NewTestModel`, `WithInitialTermSize`, `Send`, `Output`, and `WaitFor`.

For all other libraries the code below is current and correct.

## File Structure

```
mterm/
  go.mod
  .gitignore
  main.go                       entrypoint: flags, config load, tea.Program
  internal/
    config/
      types.go                  Host, Forward, Source, ForwardType
      sshconfig.go              parse ~/.ssh/config
      mterm.go                  parse ~/.config/mterm/hosts.yaml
      config.go                 Load(): merge sources, collect warnings
    ssh/
      auth.go                   AuthProvider interface + AgentProvider
      knownhosts.go             host-key callback with TOFU decision
      session.go                Session: connect, PTY, IO, resize, close, state
      reconnect.go              auto-reconnect loop with backoff
      forward.go                local (-L) and remote (-R) port forwards
    terminal/
      terminal.go               VT emulator wrapper -> renderable grid
    tui/
      keys.go                   key bindings, prefix key, keyToBytes encoder
      styles.go                 Lip Gloss styles
      picker.go                 host picker model
      session.go                per-tab session model
      forwards.go               port-forward panel model
      app.go                    root model: views, tab bar, prefix state machine
```

---

## Task 1: Project scaffold

**Files:**
- Create: `go.mod`, `.gitignore`, `main.go`

- [ ] **Step 1: Initialize the module and add dependencies**

Run from the repo root:

```bash
go mod init mterm
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/x/vt@latest
go get github.com/charmbracelet/x/exp/teatest@latest
go get golang.org/x/crypto/ssh@latest
go get github.com/kevinburke/ssh_config@latest
go get gopkg.in/yaml.v3@latest
go get github.com/atotto/clipboard@latest
```

- [ ] **Step 2: Create `.gitignore`**

```
/mterm
*.test
*.out
.DS_Store
```

- [ ] **Step 3: Create a minimal `main.go`**

```go
package main

import "fmt"

func main() {
	fmt.Println("mterm")
}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./... && go vet ./...`
Expected: no output, exit code 0. A `mterm` binary is produced.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum .gitignore main.go
git commit -m "chore: scaffold Go module and dependencies"
```

---

## Task 2: Config types and ssh_config loader

**Files:**
- Create: `internal/config/types.go`, `internal/config/sshconfig.go`
- Test: `internal/config/sshconfig_test.go`

- [ ] **Step 1: Create `internal/config/types.go`**

```go
package config

import "fmt"

type Source int

const (
	SourceSSHConfig Source = iota
	SourceMterm
)

func (s Source) String() string {
	if s == SourceMterm {
		return "mterm"
	}
	return "ssh_config"
}

type ForwardType int

const (
	ForwardLocal ForwardType = iota
	ForwardRemote
)

func (t ForwardType) String() string {
	if t == ForwardRemote {
		return "remote"
	}
	return "local"
}

// Forward is a single port-forwarding rule.
type Forward struct {
	Type     ForwardType
	BindAddr string // empty means localhost
	BindPort int
	DialAddr string
	DialPort int
}

// Host is one connectable SSH target after merging all config sources.
type Host struct {
	Name         string
	HostName     string
	User         string
	Port         int
	Group        string
	Tags         []string
	Source       Source
	IdentityFile string // parsed now, used when key-file auth lands
	ProxyJump    string // parsed now, used when jump-host support lands
	Forwards     []Forward
}

// Addr returns the host:port dial string.
func (h Host) Addr() string {
	return fmt.Sprintf("%s:%d", h.HostName, h.Port)
}
```

- [ ] **Step 2: Write the failing test for the ssh_config loader**

Create `internal/config/sshconfig_test.go`:

```go
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
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadSSHConfig -v`
Expected: FAIL — `loadSSHConfig` undefined.

- [ ] **Step 4: Create `internal/config/sshconfig.go`**

```go
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// loadSSHConfig parses an OpenSSH config file into hosts. A missing file is not
// an error: it returns (nil, nil).
func loadSSHConfig(path string) ([]Host, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, err
	}

	var hosts []Host
	seen := map[string]bool{}
	for _, block := range cfg.Hosts {
		for _, pat := range block.Patterns {
			alias := pat.String()
			if alias == "" || strings.ContainsAny(alias, "*?!") {
				continue
			}
			if seen[alias] {
				continue
			}
			seen[alias] = true
			hosts = append(hosts, hostFromConfig(cfg, alias))
		}
	}
	return hosts, nil
}

func hostFromConfig(cfg *ssh_config.Config, alias string) Host {
	get := func(key string) string {
		v, _ := cfg.Get(alias, key)
		return v
	}
	getAll := func(key string) []string {
		v, _ := cfg.GetAll(alias, key)
		return v
	}

	port := 22
	if p, err := strconv.Atoi(get("Port")); err == nil && p > 0 {
		port = p
	}
	hostName := get("HostName")
	if hostName == "" {
		hostName = alias
	}

	var forwards []Forward
	for _, raw := range getAll("LocalForward") {
		if f, ok := parseForward(raw, ForwardLocal); ok {
			forwards = append(forwards, f)
		}
	}
	for _, raw := range getAll("RemoteForward") {
		if f, ok := parseForward(raw, ForwardRemote); ok {
			forwards = append(forwards, f)
		}
	}

	return Host{
		Name:         alias,
		HostName:     hostName,
		User:         get("User"),
		Port:         port,
		Source:       SourceSSHConfig,
		IdentityFile: get("IdentityFile"),
		ProxyJump:    get("ProxyJump"),
		Forwards:     forwards,
	}
}

// parseForward parses an OpenSSH "LocalForward"/"RemoteForward" value of the
// form "[bind:]port host:hostport".
func parseForward(raw string, typ ForwardType) (Forward, bool) {
	fields := strings.Fields(raw)
	if len(fields) != 2 {
		return Forward{}, false
	}
	bindAddr, bindPort, ok := splitHostPort(fields[0])
	if !ok {
		return Forward{}, false
	}
	dialAddr, dialPort, ok := splitHostPort(fields[1])
	if !ok {
		return Forward{}, false
	}
	return Forward{
		Type:     typ,
		BindAddr: bindAddr,
		BindPort: bindPort,
		DialAddr: dialAddr,
		DialPort: dialPort,
	}, true
}

// splitHostPort accepts "port" or "host:port" and returns (host, port).
func splitHostPort(s string) (host string, port int, ok bool) {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		host = s[:i]
		s = s[i+1:]
	}
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 {
		return "", 0, false
	}
	return host, p, true
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS — `TestLoadSSHConfig`, `TestLoadSSHConfigMissingFile`.

- [ ] **Step 6: Commit**

```bash
git add internal/config/types.go internal/config/sshconfig.go internal/config/sshconfig_test.go
git commit -m "feat: parse ~/.ssh/config into hosts"
```

---

## Task 3: mterm YAML loader and config merge

**Files:**
- Create: `internal/config/mterm.go`, `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test for loading and merging**

Create `internal/config/config_test.go`:

```go
package config

import "testing"

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
    hostName: 10.9.0.12
    user: dale
    port: 22
    group: lab
    tags: [scratch]
    forwards:
      - type: local
        bindPort: 8080
        dialAddr: 127.0.0.1
        dialPort: 80
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

func TestMergeSortedByGroupThenName(t *testing.T) {
	sshPath := writeTemp(t, "config", "")
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - {name: zeta, hostName: z, group: alpha}
  - {name: alpha, hostName: a, group: beta}
  - {name: beta, hostName: b, group: alpha}
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestMerge -v`
Expected: FAIL — `loadAndMerge` and `Result` undefined.

- [ ] **Step 3: Create `internal/config/mterm.go`**

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type mtermFile struct {
	Hosts []mtermHost `yaml:"hosts"`
}

type mtermHost struct {
	Name     string         `yaml:"name"`
	HostName string         `yaml:"hostName"`
	User     string         `yaml:"user"`
	Port     int            `yaml:"port"`
	Group    string         `yaml:"group"`
	Tags     []string       `yaml:"tags"`
	Forwards []mtermForward `yaml:"forwards"`
}

type mtermForward struct {
	Type     string `yaml:"type"` // "local" | "remote"
	BindAddr string `yaml:"bindAddr"`
	BindPort int    `yaml:"bindPort"`
	DialAddr string `yaml:"dialAddr"`
	DialPort int    `yaml:"dialPort"`
}

// loadMtermFile parses the mterm YAML file. A missing file returns (nil, nil).
func loadMtermFile(path string) (*mtermFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var mf mtermFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}

func (mf mtermForward) toForward() Forward {
	typ := ForwardLocal
	if mf.Type == "remote" {
		typ = ForwardRemote
	}
	return Forward{
		Type:     typ,
		BindAddr: mf.BindAddr,
		BindPort: mf.BindPort,
		DialAddr: mf.DialAddr,
		DialPort: mf.DialPort,
	}
}
```

- [ ] **Step 4: Create `internal/config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Result is the outcome of loading all config sources.
type Result struct {
	Hosts    []Host
	Warnings []string // non-fatal problems (parse errors, etc.)
}

// Load reads ~/.ssh/config and ~/.config/mterm/hosts.yaml and merges them.
func Load() (*Result, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshPath := filepath.Join(home, ".ssh", "config")
	mtermPath := filepath.Join(home, ".config", "mterm", "hosts.yaml")
	return loadAndMerge(sshPath, mtermPath)
}

// loadAndMerge is the testable core of Load.
func loadAndMerge(sshPath, mtermPath string) (*Result, error) {
	res := &Result{}

	sshHosts, err := loadSSHConfig(sshPath)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("ssh_config: %v", err))
	}

	mf, err := loadMtermFile(mtermPath)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("mterm config: %v", err))
	}

	byName := map[string]*Host{}
	order := []string{}
	for i := range sshHosts {
		h := sshHosts[i]
		byName[h.Name] = &h
		order = append(order, h.Name)
	}

	if mf != nil {
		for _, mh := range mf.Hosts {
			if mh.Name == "" {
				res.Warnings = append(res.Warnings, "mterm config: host with empty name skipped")
				continue
			}
			if existing, ok := byName[mh.Name]; ok {
				applyOverlay(existing, mh)
				continue
			}
			h := hostFromMterm(mh)
			byName[h.Name] = &h
			order = append(order, h.Name)
		}
	}

	for _, name := range order {
		res.Hosts = append(res.Hosts, *byName[name])
	}
	sort.SliceStable(res.Hosts, func(i, j int) bool {
		a, b := res.Hosts[i], res.Hosts[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Name < b.Name
	})
	return res, nil
}

// applyOverlay attaches mterm metadata to an existing ssh_config host.
// mterm-provided non-zero values win; the ssh_config source is preserved.
func applyOverlay(h *Host, mh mtermHost) {
	h.Group = mh.Group
	if len(mh.Tags) > 0 {
		h.Tags = mh.Tags
	}
	if mh.HostName != "" {
		h.HostName = mh.HostName
	}
	if mh.User != "" {
		h.User = mh.User
	}
	if mh.Port != 0 {
		h.Port = mh.Port
	}
	for _, mf := range mh.Forwards {
		h.Forwards = append(h.Forwards, mf.toForward())
	}
}

func hostFromMterm(mh mtermHost) Host {
	port := mh.Port
	if port == 0 {
		port = 22
	}
	hostName := mh.HostName
	if hostName == "" {
		hostName = mh.Name
	}
	var forwards []Forward
	for _, mf := range mh.Forwards {
		forwards = append(forwards, mf.toForward())
	}
	return Host{
		Name:     mh.Name,
		HostName: hostName,
		User:     mh.User,
		Port:     port,
		Group:    mh.Group,
		Tags:     mh.Tags,
		Source:   SourceMterm,
		Forwards: forwards,
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS — all `TestMerge*` plus the Task 2 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/config/mterm.go internal/config/config.go internal/config/config_test.go
git commit -m "feat: merge ssh_config and mterm hosts.yaml"
```

---

## Task 4: Terminal emulator wrapper

**Files:**
- Create: `internal/terminal/terminal.go`
- Test: `internal/terminal/terminal_test.go`

> **Verify the `charmbracelet/x/vt` API first** (see "Library-API verification"
> above). The public methods of `Terminal` below are the contract; adapt the
> internal x/vt calls to the real API. `xvt` is the alias for the x/vt package.

- [ ] **Step 1: Write the failing test**

Create `internal/terminal/terminal_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/terminal/ -v`
Expected: FAIL — `New` undefined.

- [ ] **Step 3: Create `internal/terminal/terminal.go`**

```go
package terminal

import (
	"sync"

	xvt "github.com/charmbracelet/x/vt"
)

// Terminal wraps a VT emulator. It is safe for concurrent use: a reader
// goroutine calls Write while the UI goroutine calls Render.
type Terminal struct {
	mu   sync.Mutex
	vt   *xvt.Terminal
	w, h int
}

// New creates a Terminal with the given character dimensions.
func New(w, h int) *Terminal {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Terminal{vt: xvt.NewTerminal(w, h), w: w, h: h}
}

// Write feeds remote output bytes into the emulator. It never returns a short
// write or an error; the signature satisfies io.Writer.
func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.vt.Write(p)
}

// Resize changes the emulator geometry.
func (t *Terminal) Resize(w, h int) {
	if w < 1 || h < 1 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.w, t.h = w, h
	t.vt.Resize(w, h)
}

// Size returns the current dimensions.
func (t *Terminal) Size() (w, h int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.w, t.h
}

// Render returns the visible screen as a string with rows joined by '\n'.
func (t *Terminal) Render() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.vt.String()
}

// CursorPosition returns the 0-indexed cursor column and row.
func (t *Terminal) CursorPosition() (x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c := t.vt.Cursor()
	return c.X, c.Y
}
```

> If the real x/vt API differs (e.g. `String()` is not present, or the cursor
> accessor has another name), adjust *inside these methods only*. `Render()`
> must return exactly `h` rows separated by `h-1` newlines — pad short rows with
> spaces and join the grid yourself if x/vt has no single-call stringer.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/terminal/ -v`
Expected: PASS — all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/terminal/terminal.go internal/terminal/terminal_test.go
git commit -m "feat: VT emulator wrapper with concurrent-safe render"
```

---

## Task 5: SSH auth provider

**Files:**
- Create: `internal/ssh/auth.go`
- Test: `internal/ssh/auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ssh/auth_test.go`:

```go
package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh/agent"
)

// startFakeAgent serves an in-memory agent holding one key on a unix socket and
// returns the socket path.
func startFakeAgent(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: &priv}); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, conn)
		}
	}()
	return sock
}

func TestAgentProviderReturnsMethods(t *testing.T) {
	sock := startFakeAgent(t)
	p := &AgentProvider{SocketPath: sock}
	methods, err := p.Methods()
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(methods) == 0 {
		t.Fatal("want at least one auth method")
	}
}

func TestAgentProviderMissingSocket(t *testing.T) {
	p := &AgentProvider{SocketPath: filepath.Join(t.TempDir(), "nope.sock")}
	if _, err := p.Methods(); err == nil {
		t.Fatal("want error for missing agent socket")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ssh/ -run TestAgentProvider -v`
Expected: FAIL — `AgentProvider` undefined.

- [ ] **Step 3: Create `internal/ssh/auth.go`**

```go
package ssh

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// AuthProvider supplies SSH authentication methods. v1 ships only AgentProvider;
// key-file, password, and jump-host providers implement this interface later
// with no caller changes.
type AuthProvider interface {
	Methods() ([]ssh.AuthMethod, error)
}

// AgentProvider authenticates using keys held by a running ssh-agent.
type AgentProvider struct {
	SocketPath string // path to the agent's unix socket
}

// AgentProviderFromEnv builds an AgentProvider from SSH_AUTH_SOCK.
func AgentProviderFromEnv() (*AgentProvider, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK is not set; start an ssh-agent and run ssh-add")
	}
	return &AgentProvider{SocketPath: sock}, nil
}

// Methods dials the agent and returns a public-key auth method backed by it.
func (p *AgentProvider) Methods() ([]ssh.AuthMethod, error) {
	conn, err := net.Dial("unix", p.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	}
	ag := agent.NewClient(conn)
	if _, err := ag.List(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("query ssh-agent: %w", err)
	}
	return []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ssh/ -run TestAgentProvider -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ssh/auth.go internal/ssh/auth_test.go
git commit -m "feat: ssh-agent auth provider behind pluggable interface"
```

---

## Task 6: Known-hosts callback with TOFU

**Files:**
- Create: `internal/ssh/knownhosts.go`
- Test: `internal/ssh/knownhosts_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ssh/knownhosts_test.go`:

```go
package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return pk
}

func TestKnownHostsUnknownKeyTriggersDecision(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	var asked bool
	v := NewHostKeyVerifier(path, func(host string, key ssh.PublicKey) bool {
		asked = true
		return true // accept
	})
	cb := v.Callback()
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	if err := cb("host:22", addr, testHostKey(t)); err != nil {
		t.Fatalf("accepted key must pass: %v", err)
	}
	if !asked {
		t.Fatal("decision callback was not invoked for unknown key")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "host") {
		t.Fatal("accepted key was not appended to known_hosts")
	}
}

func TestKnownHostsRejectedKeyFails(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	v := NewHostKeyVerifier(path, func(host string, key ssh.PublicKey) bool {
		return false // reject
	})
	cb := v.Callback()
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	if err := cb("host:22", addr, testHostKey(t)); err == nil {
		t.Fatal("rejected key must produce an error")
	}
}

func TestKnownHostsKnownKeyDoesNotAsk(t *testing.T) {
	path := t.TempDir() + "/known_hosts"
	key := testHostKey(t)

	v1 := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool { return true })
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	if err := v1.Callback()("host:22", addr, key); err != nil {
		t.Fatal(err)
	}

	var asked bool
	v2 := NewHostKeyVerifier(path, func(string, ssh.PublicKey) bool {
		asked = true
		return true
	})
	if err := v2.Callback()("host:22", addr, key); err != nil {
		t.Fatalf("previously accepted key must pass silently: %v", err)
	}
	if asked {
		t.Fatal("known key must not trigger the decision callback")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ssh/ -run TestKnownHosts -v`
Expected: FAIL — `NewHostKeyVerifier` undefined.

- [ ] **Step 3: Create `internal/ssh/knownhosts.go`**

```go
package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// DecisionFunc is asked to approve an unknown host key (trust on first use).
// It returns true to accept and persist the key.
type DecisionFunc func(host string, key ssh.PublicKey) bool

// HostKeyVerifier validates host keys against a known_hosts file, prompting via
// a DecisionFunc when a key is unseen.
type HostKeyVerifier struct {
	path   string
	decide DecisionFunc
}

// NewHostKeyVerifier creates a verifier backed by the given known_hosts file.
func NewHostKeyVerifier(path string, decide DecisionFunc) *HostKeyVerifier {
	return &HostKeyVerifier{path: path, decide: decide}
}

// Callback returns an ssh.HostKeyCallback for use in ssh.ClientConfig.
func (v *HostKeyVerifier) Callback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		base, err := v.baseCallback()
		if err != nil {
			return err
		}
		if err := base(hostname, remote, key); err == nil {
			return nil
		} else if _, isMismatch := err.(*knownhosts.KeyError); !isMismatch {
			return err
		} else {
			// KeyError with an empty Want slice means "unknown host"; a
			// populated Want slice means a genuine mismatch.
			ke := err.(*knownhosts.KeyError)
			if len(ke.Want) > 0 {
				return fmt.Errorf("host key mismatch for %s: possible MITM", hostname)
			}
		}
		// Unknown host: ask the user.
		if !v.decide(hostname, key) {
			return fmt.Errorf("host key for %s rejected by user", hostname)
		}
		return v.append(hostname, remote, key)
	}
}

// baseCallback builds a knownhosts callback, tolerating a missing file.
func (v *HostKeyVerifier) baseCallback() (ssh.HostKeyCallback, error) {
	if _, err := os.Stat(v.path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(v.path), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(v.path, nil, 0o600); err != nil {
			return nil, err
		}
	}
	return knownhosts.New(v.path)
}

func (v *HostKeyVerifier) append(hostname string, remote net.Addr, key ssh.PublicKey) error {
	f, err := os.OpenFile(v.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	addrs := []string{knownhosts.Normalize(hostname)}
	if remote != nil {
		addrs = append(addrs, knownhosts.Normalize(remote.String()))
	}
	line := knownhosts.Line(addrs, key)
	_, err = f.WriteString(line + "\n")
	return err
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ssh/ -run TestKnownHosts -v`
Expected: PASS — all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ssh/knownhosts.go internal/ssh/knownhosts_test.go
git commit -m "feat: known_hosts verifier with trust-on-first-use"
```

---

## Task 7: SSH session — connect, PTY, IO, resize, close

**Files:**
- Create: `internal/ssh/session.go`, `internal/ssh/testserver_test.go`
- Test: `internal/ssh/session_test.go`

- [ ] **Step 1: Create the in-process test SSH server**

Create `internal/ssh/testserver_test.go`:

```go
package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// testServer is an in-process SSH server: it accepts any auth, accepts one
// session channel, accepts pty-req/shell/window-change, and echoes stdin to
// stdout. It records window-change requests and tracks live connections so a
// test can drop them.
type testServer struct {
	ln      net.Listener
	signer  ssh.Signer
	addr    string
	mu      sync.Mutex
	conns   []net.Conn
	winch   chan [2]int
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testServer{ln: ln, signer: signer, addr: ln.Addr().String(), winch: make(chan [2]int, 8)}
	go s.serve()
	t.Cleanup(func() { ln.Close(); s.dropConns() })
	return s
}

func (s *testServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		go s.handle(conn)
	}
}

// dropConns force-closes every live connection (used to test reconnect).
func (s *testServer) dropConns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		c.Close()
	}
	s.conns = nil
}

func (s *testServer) handle(conn net.Conn) {
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(s.signer)
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "only session")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go func() {
			for r := range chReqs {
				switch r.Type {
				case "pty-req", "shell":
					r.Reply(true, nil)
				case "window-change":
					if len(r.Payload) >= 8 {
						w := int(r.Payload[3])
						h := int(r.Payload[7])
						select {
						case s.winch <- [2]int{w, h}:
						default:
						}
					}
					r.Reply(true, nil)
				default:
					r.Reply(false, nil)
				}
			}
		}()
		go func() { io.Copy(ch, ch); ch.Close() }() // echo stdin -> stdout
	}
}
```

- [ ] **Step 2: Write the failing session test**

Create `internal/ssh/session_test.go`:

```go
package ssh

import (
	"strings"
	"testing"
	"time"

	"mterm/internal/config"
)

func dialTestSession(t *testing.T, srv *testServer) *Session {
	t.Helper()
	host := config.Host{Name: "test", HostName: "127.0.0.1"}
	// Override the dial address with the server's ephemeral port.
	s := NewSession(host, noAuth{}, insecureHostKey())
	s.dialAddr = srv.addr
	if err := s.Connect(80, 24); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessionEchoesInput(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	if _, err := s.Write([]byte("ping\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := readWithin(t, s, "ping", time.Second)
	if !strings.Contains(got, "ping") {
		t.Fatalf("want echoed input, got %q", got)
	}
}

func TestSessionResizeReachesServer(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	if err := s.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	select {
	case dims := <-srv.winch:
		if dims[0] != 120 || dims[1] != 40 {
			t.Fatalf("server saw window %v, want [120 40]", dims)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received window-change")
	}
}

func TestSessionStateConnected(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)
	if s.State() != StateConnected {
		t.Fatalf("state = %v, want Connected", s.State())
	}
}

// readWithin reads from the session's output until want appears or timeout.
func readWithin(t *testing.T, s *Session, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		s.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, _ := s.Read(buf)
		acc.Write(buf[:n])
		if strings.Contains(acc.String(), want) {
			break
		}
	}
	return acc.String()
}
```

> **Note:** `noAuth`, `insecureHostKey`, `Session.dialAddr`, `Session.Read`, and
> `Session.SetReadDeadline` are test-only helpers and fields. Define `noAuth`
> and `insecureHostKey` in `session_test.go` as shown next.

- [ ] **Step 3: Add test helpers to `session_test.go`**

Append to `internal/ssh/session_test.go`:

```go
// noAuth is an AuthProvider that offers no methods (the test server allows
// NoClientAuth).
type noAuth struct{}

func (noAuth) Methods() ([]ssh.AuthMethod, error) { return nil, nil }

// insecureHostKey accepts any host key (test only).
func insecureHostKey() ssh.HostKeyCallback {
	return ssh.InsecureIgnoreHostKey()
}
```

Add the import `"golang.org/x/crypto/ssh"` to the file's import block.

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/ssh/ -run TestSession -v`
Expected: FAIL — `NewSession`, `Session`, `StateConnected` undefined.

- [ ] **Step 5: Create `internal/ssh/session.go`**

```go
package ssh

import (
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"mterm/internal/config"
)

// State is the lifecycle state of a Session.
type State int

const (
	StateConnecting State = iota
	StateConnected
	StateReconnecting
	StateFailed
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateFailed:
		return "failed"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Session is a single live SSH connection with an interactive PTY shell.
type Session struct {
	host     config.Host
	auth     AuthProvider
	hostKey  ssh.HostKeyCallback
	dialAddr string // host:port to dial; defaults to host.Addr()

	mu       sync.Mutex
	state    State
	lastErr  error
	client   *ssh.Client
	sess     *ssh.Session
	stdin    io.WriteCloser
	stdout   io.Reader
	conn     deadlineConn // underlying net.Conn for read deadlines (test use)

	closed bool // true once the user called Close
}

// deadlineConn is the subset of net.Conn used for test read deadlines.
type deadlineConn interface {
	SetReadDeadline(time.Time) error
}

// NewSession builds an unconnected Session for the given host.
func NewSession(host config.Host, auth AuthProvider, hostKey ssh.HostKeyCallback) *Session {
	return &Session{
		host:     host,
		auth:     auth,
		hostKey:  hostKey,
		dialAddr: host.Addr(),
		state:    StateConnecting,
	}
}

// Host returns the session's host definition.
func (s *Session) Host() config.Host { return s.host }

// State returns the current lifecycle state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// LastError returns the most recent connection error, if any.
func (s *Session) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Session) setState(st State, err error) {
	s.mu.Lock()
	s.state = st
	if err != nil {
		s.lastErr = err
	}
	s.mu.Unlock()
}

// Connect dials the host and starts an interactive shell with a PTY of the
// given size.
func (s *Session) Connect(cols, rows int) error {
	methods, err := s.auth.Methods()
	if err != nil {
		s.setState(StateFailed, err)
		return err
	}
	user := s.host.User
	if user == "" {
		user = "root"
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            methods,
		HostKeyCallback: s.hostKey,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", s.dialAddr, cfg)
	if err != nil {
		s.setState(StateFailed, err)
		return err
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}
	sess.Stderr = nil // stderr is merged into the PTY by the server
	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		s.setState(StateFailed, err)
		return err
	}

	s.mu.Lock()
	s.client = client
	s.sess = sess
	s.stdin = stdin
	s.stdout = stdout
	s.state = StateConnected
	s.lastErr = nil
	s.mu.Unlock()
	return nil
}

// Write sends bytes to the remote shell's stdin.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	w := s.stdin
	s.mu.Unlock()
	if w == nil {
		return 0, fmt.Errorf("session not connected")
	}
	return w.Write(p)
}

// Read reads remote output. Used by the reader goroutine and tests.
func (s *Session) Read(p []byte) (int, error) {
	s.mu.Lock()
	r := s.stdout
	s.mu.Unlock()
	if r == nil {
		return 0, io.EOF
	}
	return r.Read(p)
}

// SetReadDeadline is a test hook; it is a no-op when no deadlineConn is set.
func (s *Session) SetReadDeadline(t time.Time) error {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	return c.SetReadDeadline(t)
}

// Resize changes the remote PTY dimensions.
func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	sess := s.sess
	s.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("session not connected")
	}
	return sess.WindowChange(rows, cols)
}

// Close terminates the session; it will not auto-reconnect afterward.
func (s *Session) Close() error {
	s.mu.Lock()
	s.closed = true
	sess, client := s.sess, s.client
	s.sess, s.client, s.stdin, s.stdout = nil, nil, nil, nil
	s.state = StateClosed
	s.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
	if client != nil {
		return client.Close()
	}
	return nil
}

// userClosed reports whether Close was called.
func (s *Session) userClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/ssh/ -run TestSession -v`
Expected: PASS — all three `TestSession*` tests.

- [ ] **Step 7: Commit**

```bash
git add internal/ssh/session.go internal/ssh/session_test.go internal/ssh/testserver_test.go
git commit -m "feat: SSH session with interactive PTY shell"
```

---

## Task 8: Auto-reconnect with backoff

**Files:**
- Create: `internal/ssh/reconnect.go`
- Test: `internal/ssh/reconnect_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ssh/reconnect_test.go`:

```go
package ssh

import (
	"testing"
	"time"
)

func TestBackoffGrowsAndCaps(t *testing.T) {
	b := backoff{base: 100 * time.Millisecond, max: time.Second}
	got := []time.Duration{b.next(), b.next(), b.next(), b.next(), b.next()}
	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		time.Second, // capped
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backoff[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBackoffResets(t *testing.T) {
	b := backoff{base: 100 * time.Millisecond, max: time.Second}
	b.next()
	b.next()
	b.reset()
	if d := b.next(); d != 100*time.Millisecond {
		t.Fatalf("after reset, next = %v, want 100ms", d)
	}
}

func TestReconnectRestoresSession(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	// Drop the server-side connection; the session loses its transport.
	srv.dropConns()

	// Reconnect should re-establish and return to Connected.
	if err := s.Reconnect(80, 24); err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	if s.State() != StateConnected {
		t.Fatalf("state after reconnect = %v, want Connected", s.State())
	}
	if _, err := s.Write([]byte("hi\n")); err != nil {
		t.Fatalf("write after reconnect: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ssh/ -run "TestBackoff|TestReconnect" -v`
Expected: FAIL — `backoff` and `Session.Reconnect` undefined.

- [ ] **Step 3: Create `internal/ssh/reconnect.go`**

```go
package ssh

import "time"

// backoff produces exponentially growing delays capped at max.
type backoff struct {
	base    time.Duration
	max     time.Duration
	attempt int
}

func (b *backoff) next() time.Duration {
	d := b.base << b.attempt
	if d <= 0 || d > b.max {
		d = b.max
	}
	b.attempt++
	return d
}

func (b *backoff) reset() { b.attempt = 0 }

// Reconnect tears down any existing transport and re-runs Connect once. It
// reuses the session (state/host) so the owning tab stays in place.
func (s *Session) Reconnect(cols, rows int) error {
	s.mu.Lock()
	sess, client := s.sess, s.client
	s.sess, s.client, s.stdin, s.stdout = nil, nil, nil, nil
	s.state = StateReconnecting
	s.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
	if client != nil {
		client.Close()
	}
	return s.Connect(cols, rows)
}

// AutoReconnect retries Connect with exponential backoff until it succeeds, the
// user closes the session, or maxAttempts is reached. maxAttempts <= 0 means
// retry forever. It is intended to run in its own goroutine.
func (s *Session) AutoReconnect(cols, rows, maxAttempts int) error {
	b := backoff{base: 500 * time.Millisecond, max: 30 * time.Second}
	for attempt := 0; maxAttempts <= 0 || attempt < maxAttempts; attempt++ {
		if s.userClosed() {
			return nil
		}
		time.Sleep(b.next())
		if s.userClosed() {
			return nil
		}
		if err := s.Reconnect(cols, rows); err == nil {
			return nil
		}
	}
	s.setState(StateFailed, s.LastError())
	return s.LastError()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ssh/ -run "TestBackoff|TestReconnect" -v`
Expected: PASS — all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ssh/reconnect.go internal/ssh/reconnect_test.go
git commit -m "feat: auto-reconnect with exponential backoff"
```

---

## Task 9: Port forwarding

**Files:**
- Create: `internal/ssh/forward.go`
- Test: `internal/ssh/forward_test.go`

> The Task 7 test server only handles `session` channels. This task extends it
> to handle `direct-tcpip` (local forwards) so a forward can be tested
> end-to-end.

- [ ] **Step 1: Extend the test server to handle direct-tcpip**

In `internal/ssh/testserver_test.go`, replace the `for nc := range chans` loop
body's channel-type check so non-session channels of type `direct-tcpip` are
handled instead of rejected. Replace:

```go
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "only session")
			continue
		}
```

with:

```go
		if nc.ChannelType() == "direct-tcpip" {
			go handleDirectTCPIP(nc)
			continue
		}
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "unsupported channel")
			continue
		}
```

Then append to `internal/ssh/testserver_test.go`:

```go
// directTCPIPPayload mirrors the RFC 4254 direct-tcpip channel request.
type directTCPIPPayload struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

// handleDirectTCPIP dials the requested destination and pipes bytes both ways.
func handleDirectTCPIP(nc ssh.NewChannel) {
	var p directTCPIPPayload
	if err := ssh.Unmarshal(nc.ExtraData(), &p); err != nil {
		nc.Reject(ssh.ConnectionFailed, "bad payload")
		return
	}
	dest := net.JoinHostPort(p.DestAddr, fmt.Sprintf("%d", p.DestPort))
	target, err := net.Dial("tcp", dest)
	if err != nil {
		nc.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	ch, reqs, err := nc.Accept()
	if err != nil {
		target.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	go func() { io.Copy(ch, target); ch.Close() }()
	go func() { io.Copy(target, ch); target.Close() }()
}
```

Add imports `"fmt"` to `testserver_test.go` if not already present.

- [ ] **Step 2: Write the failing forward test**

Create `internal/ssh/forward_test.go`:

```go
package ssh

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"

	"mterm/internal/config"
)

// echoListener starts a TCP echo server and returns its host and port.
func echoListener(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func TestLocalForwardEndToEnd(t *testing.T) {
	srv := newTestServer(t)
	s := dialTestSession(t, srv)

	echoHost, echoPort := echoListener(t)
	fwd := config.Forward{
		Type:     config.ForwardLocal,
		BindAddr: "127.0.0.1",
		BindPort: 0, // 0 = OS-assigned, read back via LocalAddr
		DialAddr: echoHost,
		DialPort: echoPort,
	}
	pf, err := s.StartForward(fwd)
	if err != nil {
		t.Fatalf("StartForward: %v", err)
	}
	t.Cleanup(func() { pf.Stop() })

	conn, err := net.Dial("tcp", pf.LocalAddr())
	if err != nil {
		t.Fatalf("dial forwarded port: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err := conn.Write([]byte("forwarded!\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if line != "forwarded!\n" {
		t.Fatalf("echo through forward = %q, want %q", line, "forwarded!\n")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/ssh/ -run TestLocalForward -v`
Expected: FAIL — `Session.StartForward` undefined.

- [ ] **Step 4: Create `internal/ssh/forward.go`**

```go
package ssh

import (
	"fmt"
	"io"
	"net"
	"sync"

	"mterm/internal/config"
)

// PortForward is a running forward owned by a Session.
type PortForward struct {
	spec     config.Forward
	listener net.Listener // local listener (-L) or remote listener (-R)

	mu      sync.Mutex
	stopped bool
}

// LocalAddr returns the actual local listen address (host:port). For remote
// forwards it returns the remote listen address.
func (p *PortForward) LocalAddr() string {
	return p.listener.Addr().String()
}

// Spec returns the forward's configuration.
func (p *PortForward) Spec() config.Forward { return p.spec }

// Stop closes the forward's listener.
func (p *PortForward) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.stopped = true
	p.listener.Close()
}

// StartForward starts a single port forward on this session.
func (s *Session) StartForward(spec config.Forward) (*PortForward, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("session not connected")
	}

	bindAddr := spec.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	listenAddr := net.JoinHostPort(bindAddr, fmt.Sprintf("%d", spec.BindPort))
	dialAddr := net.JoinHostPort(spec.DialAddr, fmt.Sprintf("%d", spec.DialPort))

	var ln net.Listener
	var err error
	if spec.Type == config.ForwardLocal {
		// -L: listen locally, dial through the SSH client.
		ln, err = net.Listen("tcp", listenAddr)
	} else {
		// -R: listen on the remote, dial locally.
		ln, err = client.Listen("tcp", listenAddr)
	}
	if err != nil {
		return nil, err
	}

	pf := &PortForward{spec: spec, listener: ln}
	go pf.acceptLoop(client, dialAddr)
	return pf, nil
}

// acceptLoop accepts connections on the listener and pipes each to the other
// side of the forward.
func (p *PortForward) acceptLoop(client sshDialer, dialAddr string) {
	for {
		incoming, err := p.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go p.pipe(client, incoming, dialAddr)
	}
}

func (p *PortForward) pipe(client sshDialer, incoming net.Conn, dialAddr string) {
	var target net.Conn
	var err error
	if p.spec.Type == config.ForwardLocal {
		target, err = client.Dial("tcp", dialAddr) // through the tunnel
	} else {
		target, err = net.Dial("tcp", dialAddr) // local side
	}
	if err != nil {
		incoming.Close()
		return
	}
	go func() { io.Copy(target, incoming); target.Close() }()
	go func() { io.Copy(incoming, target); incoming.Close() }()
}

// sshDialer is the subset of *ssh.Client used by forwards.
type sshDialer interface {
	Dial(network, addr string) (net.Conn, error)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/ssh/ -v`
Expected: PASS — every test in the `ssh` package, including `TestLocalForwardEndToEnd`.

- [ ] **Step 6: Commit**

```bash
git add internal/ssh/forward.go internal/ssh/forward_test.go internal/ssh/testserver_test.go
git commit -m "feat: local and remote SSH port forwarding"
```

---

## Task 10: TUI styles and key encoding

**Files:**
- Create: `internal/tui/styles.go`, `internal/tui/keys.go`
- Test: `internal/tui/keys_test.go`

- [ ] **Step 1: Create `internal/tui/styles.go`**

```go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	tabActive = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("231"))

	tabInactive = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("245"))

	statusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	errorText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))

	modalBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)
)
```

- [ ] **Step 2: Write the failing test for key encoding**

Create `internal/tui/keys_test.go`:

```go
package tui

import (
	"bytes"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyToBytes(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want []byte
	}{
		{"runes", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")}, []byte("ab")},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, []byte{'\r'}},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, []byte{0x7f}},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, []byte{'\t'}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, []byte{0x1b}},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, []byte{' '}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, []byte("\x1b[A")},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, []byte("\x1b[B")},
		{"ctrl-c", tea.KeyMsg{Type: tea.KeyCtrlC}, []byte{0x03}},
		{"ctrl-a", tea.KeyMsg{Type: tea.KeyCtrlA}, []byte{0x01}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := keyToBytes(c.key)
			if !bytes.Equal(got, c.want) {
				t.Fatalf("keyToBytes(%v) = %v, want %v", c.key, got, c.want)
			}
		})
	}
}

func TestIsPrefixKey(t *testing.T) {
	if !isPrefixKey(tea.KeyMsg{Type: tea.KeyCtrlB}) {
		t.Fatal("Ctrl-B must be the prefix key")
	}
	if isPrefixKey(tea.KeyMsg{Type: tea.KeyCtrlA}) {
		t.Fatal("Ctrl-A must not be the prefix key")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run "TestKeyToBytes|TestIsPrefixKey" -v`
Expected: FAIL — `keyToBytes`, `isPrefixKey` undefined.

- [ ] **Step 4: Create `internal/tui/keys.go`**

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// isPrefixKey reports whether k is the mterm prefix key (Ctrl-B, tmux-style).
func isPrefixKey(k tea.KeyMsg) bool {
	return k.Type == tea.KeyCtrlB
}

// keyToBytes encodes a Bubble Tea key event as the bytes a terminal would send
// to a remote PTY.
func keyToBytes(k tea.KeyMsg) []byte {
	switch k.Type {
	case tea.KeyRunes:
		return []byte(string(k.Runes))
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	}
	// Ctrl-A..Ctrl-_ are contiguous KeyType values equal to control bytes 1..31.
	if k.Type >= tea.KeyCtrlA && k.Type <= tea.KeyCtrlUnderscore {
		return []byte{byte(k.Type)}
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run "TestKeyToBytes|TestIsPrefixKey" -v`
Expected: PASS — all subtests.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/styles.go internal/tui/keys.go internal/tui/keys_test.go
git commit -m "feat: TUI styles and key-to-bytes encoder"
```

---

## Task 11: Host picker model

**Files:**
- Create: `internal/tui/picker.go`
- Test: `internal/tui/picker_test.go`

> **Verify the `teatest` API first** (see "Library-API verification" above).

- [ ] **Step 1: Write the failing test**

Create `internal/tui/picker_test.go`:

```go
package tui

import (
	"testing"

	"mterm/internal/config"
)

func sampleHosts() []config.Host {
	return []config.Host{
		{Name: "prod-web", HostName: "10.0.1.4", Group: "production", Tags: []string{"web"}},
		{Name: "prod-db", HostName: "10.0.2.9", Group: "production"},
		{Name: "lab-box", HostName: "10.9.0.12", Group: "lab"},
	}
}

func TestPickerFiltersByQuery(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setQuery("lab")

	visible := p.visibleHosts()
	if len(visible) != 1 || visible[0].Name != "lab-box" {
		t.Fatalf("filter 'lab' = %v, want [lab-box]", names(visible))
	}
}

func TestPickerFilterIsCaseInsensitive(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setQuery("PROD")
	if got := len(p.visibleHosts()); got != 2 {
		t.Fatalf("filter 'PROD' matched %d hosts, want 2", got)
	}
}

func TestPickerSelectionReturnsHost(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setQuery("lab")
	h, ok := p.selected()
	if !ok || h.Name != "lab-box" {
		t.Fatalf("selected() = %v,%v want lab-box,true", h.Name, ok)
	}
}

func names(hs []config.Host) []string {
	out := []string{}
	for _, h := range hs {
		out = append(out, h.Name)
	}
	return out
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestPicker -v`
Expected: FAIL — `newPicker` undefined.

- [ ] **Step 3: Create `internal/tui/picker.go`**

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
)

// pickerChosenMsg is emitted when the user selects a host in the picker.
type pickerChosenMsg struct{ host config.Host }

// pickerCancelledMsg is emitted when the user dismisses the picker.
type pickerCancelledMsg struct{}

// picker is the fuzzy host-selection view.
type picker struct {
	all     []config.Host
	query   string
	cursor  int
	w, h    int
}

func newPicker(hosts []config.Host) *picker {
	return &picker{all: hosts}
}

func (p *picker) setSize(w, h int) { p.w, p.h = w, h }

func (p *picker) setQuery(q string) {
	p.query = q
	p.cursor = 0
}

// visibleHosts returns hosts matching the current query (case-insensitive
// substring over name, hostname, group, and tags).
func (p *picker) visibleHosts() []config.Host {
	if p.query == "" {
		return p.all
	}
	q := strings.ToLower(p.query)
	var out []config.Host
	for _, h := range p.all {
		if hostMatches(h, q) {
			out = append(out, h)
		}
	}
	return out
}

func hostMatches(h config.Host, q string) bool {
	hay := strings.ToLower(strings.Join(append([]string{
		h.Name, h.HostName, h.Group,
	}, h.Tags...), " "))
	return strings.Contains(hay, q)
}

// selected returns the host under the cursor, if any.
func (p *picker) selected() (config.Host, bool) {
	v := p.visibleHosts()
	if len(v) == 0 || p.cursor < 0 || p.cursor >= len(v) {
		return config.Host{}, false
	}
	return v[p.cursor], true
}

func (p *picker) moveCursor(delta int) {
	v := p.visibleHosts()
	if len(v) == 0 {
		p.cursor = 0
		return
	}
	p.cursor = (p.cursor + delta + len(v)) % len(v)
}

// Update handles picker key events. It returns a command carrying a
// pickerChosenMsg or pickerCancelledMsg when the picker resolves.
func (p *picker) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		if h, ok := p.selected(); ok {
			return func() tea.Msg { return pickerChosenMsg{host: h} }
		}
	case tea.KeyEsc:
		return func() tea.Msg { return pickerCancelledMsg{} }
	case tea.KeyUp:
		p.moveCursor(-1)
	case tea.KeyDown:
		p.moveCursor(1)
	case tea.KeyCtrlK:
		p.moveCursor(-1)
	case tea.KeyCtrlJ:
		p.moveCursor(1)
	case tea.KeyBackspace:
		if p.query != "" {
			p.setQuery(p.query[:len(p.query)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		p.setQuery(p.query + string(msg.Runes))
	}
	return nil
}

// View renders the picker.
func (p *picker) View() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Search: %s\n\n", p.query))
	v := p.visibleHosts()
	if len(v) == 0 {
		b.WriteString(statusBar.Render("  (no matching hosts)"))
		return b.String()
	}
	lastGroup := "\x00"
	for i, h := range v {
		if h.Group != lastGroup {
			lastGroup = h.Group
			group := h.Group
			if group == "" {
				group = "(ungrouped)"
			}
			b.WriteString(statusBar.Render("  " + group))
			b.WriteString("\n")
		}
		cursor := "  "
		if i == p.cursor {
			cursor = "> "
		}
		tags := ""
		if len(h.Tags) > 0 {
			tags = statusBar.Render("  [" + strings.Join(h.Tags, ",") + "]")
		}
		b.WriteString(fmt.Sprintf("%s%-16s %-18s%s\n", cursor, h.Name, h.HostName, tags))
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("  enter: connect   esc: back   ctrl-c: quit"))
	return b.String()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestPicker -v`
Expected: PASS — all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/picker.go internal/tui/picker_test.go
git commit -m "feat: fuzzy host picker model"
```

---

## Task 12: Session tab model

**Files:**
- Create: `internal/tui/session.go`
- Test: `internal/tui/session_test.go`

This model wraps one `ssh.Session` plus its `terminal.Terminal`. A reader
goroutine copies session output into the emulator; the UI reads the emulator in
`View`.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/session_test.go`:

```go
package tui

import (
	"testing"

	"mterm/internal/config"
)

func TestSessionTabTitleUsesHostName(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "prod-web"}, 80, 24)
	if tab.title() != "prod-web" {
		t.Fatalf("title() = %q, want prod-web", tab.title())
	}
	if tab.id != 1 {
		t.Fatalf("id = %d, want 1", tab.id)
	}
}

func TestSessionTabResizeUpdatesTerminal(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	tab.resize(100, 30)
	w, h := tab.term.Size()
	// Terminal height is the tab body: total rows minus the tab bar row.
	if w != 100 || h != 29 {
		t.Fatalf("terminal size = %d,%d want 100,29", w, h)
	}
}

func TestSessionTabViewRendersTerminal(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	tab.term.Write([]byte("session-output"))
	if got := tab.View(); got == "" {
		t.Fatal("View() returned empty")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestSessionTab -v`
Expected: FAIL — `newSessionTab` undefined.

- [ ] **Step 3: Create `internal/tui/session.go`**

```go
package tui

import (
	"sync/atomic"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
	"mterm/internal/terminal"
)

// tabBarRows is the height reserved for the tab bar.
const tabBarRows = 1

// sessionTab is one connection tab: an ssh.Session feeding a terminal emulator.
type sessionTab struct {
	id     int
	host   config.Host
	term   *terminal.Terminal
	sess   *mssh.Session // nil until connected
	dirty  atomic.Bool   // set by the reader goroutine, cleared on render
	w, h   int           // full tab area including the tab bar
}

// newSessionTab creates a tab sized to the given total area.
func newSessionTab(id int, host config.Host, w, h int) *sessionTab {
	bodyH := h - tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	return &sessionTab{
		id:   id,
		host: host,
		term: terminal.New(w, bodyH),
		w:    w,
		h:    h,
	}
}

func (t *sessionTab) title() string { return t.host.Name }

// attach binds a connected ssh.Session and starts the reader goroutine.
func (t *sessionTab) attach(s *mssh.Session) {
	t.sess = s
	go t.readLoop()
}

// readLoop copies remote output into the emulator until the session ends. It
// recovers from panics so one bad session cannot crash the program.
func (t *sessionTab) readLoop() {
	defer func() { _ = recover() }()
	buf := make([]byte, 32*1024)
	for {
		n, err := t.sess.Read(buf)
		if n > 0 {
			t.term.Write(buf[:n])
			t.dirty.Store(true)
		}
		if err != nil {
			t.dirty.Store(true)
			return
		}
	}
}

// resize updates both the tab area and the underlying terminal/PTY.
func (t *sessionTab) resize(w, h int) {
	t.w, t.h = w, h
	bodyH := h - tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	t.term.Resize(w, bodyH)
	if t.sess != nil {
		_ = t.sess.Resize(w, bodyH)
	}
}

// sendInput writes encoded key bytes to the remote shell.
func (t *sessionTab) sendInput(p []byte) {
	if t.sess != nil && len(p) > 0 {
		_, _ = t.sess.Write(p)
	}
}

// View renders the tab body (the terminal screen).
func (t *sessionTab) View() string {
	return t.term.Render()
}

// close terminates the session.
func (t *sessionTab) close() {
	if t.sess != nil {
		_ = t.sess.Close()
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestSessionTab -v`
Expected: PASS — all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/session.go internal/tui/session_test.go
git commit -m "feat: session tab model with reader goroutine"
```

---

## Task 13: Forwards panel model

**Files:**
- Create: `internal/tui/forwards.go`
- Test: `internal/tui/forwards_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tui/forwards_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"mterm/internal/config"
)

func TestForwardsPanelListsForwards(t *testing.T) {
	host := config.Host{
		Name: "h",
		Forwards: []config.Forward{
			{Type: config.ForwardLocal, BindPort: 8080, DialAddr: "127.0.0.1", DialPort: 80},
			{Type: config.ForwardRemote, BindPort: 9000, DialAddr: "127.0.0.1", DialPort: 3000},
		},
	}
	panel := newForwardsPanel(host)
	view := panel.View()
	if !strings.Contains(view, "8080") || !strings.Contains(view, "9000") {
		t.Fatalf("panel must list both forwards, got:\n%s", view)
	}
	if !strings.Contains(view, "local") || !strings.Contains(view, "remote") {
		t.Fatalf("panel must show forward types, got:\n%s", view)
	}
}

func TestForwardsPanelToggle(t *testing.T) {
	host := config.Host{
		Name:     "h",
		Forwards: []config.Forward{{Type: config.ForwardLocal, BindPort: 8080}},
	}
	panel := newForwardsPanel(host)
	if panel.enabled(0) {
		t.Fatal("forward should start disabled")
	}
	panel.toggle(0)
	if !panel.enabled(0) {
		t.Fatal("toggle must enable the forward")
	}
	panel.toggle(0)
	if panel.enabled(0) {
		t.Fatal("toggle must disable the forward")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestForwardsPanel -v`
Expected: FAIL — `newForwardsPanel` undefined.

- [ ] **Step 3: Create `internal/tui/forwards.go`**

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
)

// forwardsClosedMsg is emitted when the forwards panel is dismissed.
type forwardsClosedMsg struct{}

// forwardsPanel lists a host's port forwards and lets the user toggle each.
type forwardsPanel struct {
	host    config.Host
	on      []bool // enabled state, parallel to host.Forwards
	cursor  int
}

func newForwardsPanel(host config.Host) *forwardsPanel {
	return &forwardsPanel{
		host: host,
		on:   make([]bool, len(host.Forwards)),
	}
}

func (p *forwardsPanel) enabled(i int) bool {
	return i >= 0 && i < len(p.on) && p.on[i]
}

func (p *forwardsPanel) toggle(i int) {
	if i >= 0 && i < len(p.on) {
		p.on[i] = !p.on[i]
	}
}

// Update handles panel key events.
func (p *forwardsPanel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		return func() tea.Msg { return forwardsClosedMsg{} }
	case tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
	case tea.KeyDown:
		if p.cursor < len(p.host.Forwards)-1 {
			p.cursor++
		}
	case tea.KeySpace, tea.KeyEnter:
		p.toggle(p.cursor)
	}
	return nil
}

// View renders the panel.
func (p *forwardsPanel) View() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Port forwards — %s\n\n", p.host.Name))
	if len(p.host.Forwards) == 0 {
		b.WriteString(statusBar.Render("  (no forwards configured for this host)\n"))
	}
	for i, f := range p.host.Forwards {
		cursor := "  "
		if i == p.cursor {
			cursor = "> "
		}
		mark := "[ ]"
		if p.on[i] {
			mark = "[x]"
		}
		b.WriteString(fmt.Sprintf("%s%s %-6s %d -> %s:%d\n",
			cursor, mark, f.Type.String(), f.BindPort, f.DialAddr, f.DialPort))
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("  space: toggle   esc: close"))
	return modalBox.Render(b.String())
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestForwardsPanel -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/forwards.go internal/tui/forwards_test.go
git commit -m "feat: port-forward panel model"
```

---

## Task 14: Root app model — views, tab bar, prefix state machine

**Files:**
- Create: `internal/tui/app.go`
- Test: `internal/tui/app_test.go`

The root model owns the view mode, the session tabs, and the prefix-key state
machine. A 30 fps tick coalesces session output into renders.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/app_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
)

func newTestApp() *App {
	return NewApp(sampleHosts(), nil)
}

func TestAppStartsInPickerMode(t *testing.T) {
	app := newTestApp()
	if app.mode != modePicker {
		t.Fatalf("mode = %v, want modePicker", app.mode)
	}
}

func TestAppPrefixThenNextTabSwitchesTab(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	// Two tabs, both attached to nil sessions for this unit test.
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	app.active = 0
	app.mode = modeSession

	// Prefix key is consumed, sets pending state.
	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if !app.prefixPending {
		t.Fatal("Ctrl-B must set prefixPending")
	}
	// 'n' advances to the next tab and clears pending.
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if app.active != 1 {
		t.Fatalf("active tab = %d, want 1", app.active)
	}
	if app.prefixPending {
		t.Fatal("prefixPending must clear after the command key")
	}
}

func TestAppPlainKeyInSessionModeIsNotACommand(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0
	app.mode = modeSession

	// 'n' without the prefix must NOT switch tabs (there is only one anyway)
	// and must not be treated as a command.
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if app.prefixPending {
		t.Fatal("a plain key must not set prefixPending")
	}
}

func TestAppPrefixCQuitWithNoTabs(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.prefixPending = true
	cmd := app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("prefix-q with no live tabs should return a quit command")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestApp -v`
Expected: FAIL — `NewApp`, `App`, `modePicker` undefined.

- [ ] **Step 3: Create `internal/tui/app.go`**

```go
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
)

// viewMode is the current top-level view.
type viewMode int

const (
	modePicker viewMode = iota
	modeSession
	modeForwards
)

// renderInterval is the coalesced redraw cadence (~30 fps).
const renderInterval = 33 * time.Millisecond

// tickMsg drives periodic re-renders so session output is coalesced.
type tickMsg struct{}

// Connector turns a host into a connected session. main.go supplies the real
// implementation; tests may pass nil and avoid connecting.
type Connector func(config.Host, int, int) (*mssh.Session, error)

// App is the root Bubble Tea model.
type App struct {
	hosts   []config.Host
	connect Connector

	mode          viewMode
	prevMode      viewMode // mode to restore when a modal/picker closes
	prefixPending bool

	picker   *picker
	forwards *forwardsPanel
	tabs     []*sessionTab
	active   int
	nextID   int

	width, height int
	statusMsg     string
}

// NewApp builds the root model.
func NewApp(hosts []config.Host, connect Connector) *App {
	return &App{
		hosts:   hosts,
		connect: connect,
		mode:    modePicker,
		picker:  newPicker(hosts),
		nextID:  1,
	}
}

// Init starts the render ticker.
func (a *App) Init() tea.Cmd {
	return tea.Tick(renderInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// Update is the Bubble Tea entry point; it delegates to update.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return a, a.update(msg)
}

// update handles one message and returns a command. Split out so tests can call
// it directly.
func (a *App) update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tickMsg:
		return tea.Tick(renderInterval, func(time.Time) tea.Msg { return tickMsg{} })

	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.picker.setSize(m.Width, m.Height)
		for _, t := range a.tabs {
			t.resize(m.Width, m.Height)
		}
		return nil

	case tea.KeyMsg:
		return a.handleKey(m)

	case pickerChosenMsg:
		return a.openTab(m.host)

	case pickerCancelledMsg:
		if len(a.tabs) > 0 {
			a.mode = modeSession
		}
		return nil

	case forwardsClosedMsg:
		a.mode = modeSession
		return nil
	}
	return nil
}

func (a *App) handleKey(k tea.KeyMsg) tea.Cmd {
	// Ctrl-C always quits.
	if k.Type == tea.KeyCtrlC {
		return tea.Quit
	}

	switch a.mode {
	case modePicker:
		return a.picker.Update(k)

	case modeForwards:
		return a.forwards.Update(k)

	case modeSession:
		if a.prefixPending {
			a.prefixPending = false
			return a.handleCommandKey(k)
		}
		if isPrefixKey(k) {
			a.prefixPending = true
			return nil
		}
		// Forward the keystroke to the active session.
		if t := a.activeTab(); t != nil {
			t.sendInput(keyToBytes(k))
		}
		return nil
	}
	return nil
}

// handleCommandKey runs a prefix-key command.
func (a *App) handleCommandKey(k tea.KeyMsg) tea.Cmd {
	switch keyString(k) {
	case "c":
		a.picker.setQuery("")
		a.mode = modePicker
	case "n":
		a.cycleTab(1)
	case "p":
		a.cycleTab(-1)
	case "x":
		a.closeActiveTab()
	case "f":
		if t := a.activeTab(); t != nil {
			a.forwards = newForwardsPanel(t.host)
			a.mode = modeForwards
		}
	case "q":
		return tea.Quit
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(k.Runes[0] - '1')
		if idx < len(a.tabs) {
			a.active = idx
		}
	}
	return nil
}

// keyString renders a command key as a comparable string.
func keyString(k tea.KeyMsg) string {
	if k.Type == tea.KeyRunes && len(k.Runes) == 1 {
		return string(k.Runes)
	}
	return ""
}

// openTab connects to a host and adds a new tab.
func (a *App) openTab(host config.Host) tea.Cmd {
	tab := newSessionTab(a.nextID, host, a.width, a.height)
	a.nextID++
	if a.connect != nil {
		bodyH := a.height - tabBarRows
		if bodyH < 1 {
			bodyH = 1
		}
		sess, err := a.connect(host, a.width, bodyH)
		if err != nil {
			a.statusMsg = fmt.Sprintf("connect %s: %v", host.Name, err)
		} else {
			tab.attach(sess)
		}
	}
	a.tabs = append(a.tabs, tab)
	a.active = len(a.tabs) - 1
	a.mode = modeSession
	return nil
}

func (a *App) activeTab() *sessionTab {
	if a.active < 0 || a.active >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.active]
}

func (a *App) cycleTab(delta int) {
	if len(a.tabs) == 0 {
		return
	}
	a.active = (a.active + delta + len(a.tabs)) % len(a.tabs)
}

func (a *App) closeActiveTab() {
	t := a.activeTab()
	if t == nil {
		return
	}
	t.close()
	a.tabs = append(a.tabs[:a.active], a.tabs[a.active+1:]...)
	if a.active >= len(a.tabs) {
		a.active = len(a.tabs) - 1
	}
	if len(a.tabs) == 0 {
		a.mode = modePicker
		a.picker.setQuery("")
	}
}

// View renders the current view.
func (a *App) View() string {
	switch a.mode {
	case modePicker:
		return a.picker.View()
	case modeForwards:
		return a.forwards.View()
	case modeSession:
		return a.sessionView()
	}
	return ""
}

func (a *App) sessionView() string {
	t := a.activeTab()
	if t == nil {
		return "no active session"
	}
	return a.tabBar() + "\n" + t.View()
}

func (a *App) tabBar() string {
	var parts []string
	for i, t := range a.tabs {
		label := fmt.Sprintf("%d:%s", i+1, t.title())
		if i == a.active {
			parts = append(parts, tabActive.Render(label))
		} else {
			parts = append(parts, tabInactive.Render(label))
		}
	}
	bar := strings.Join(parts, " ")
	if a.prefixPending {
		bar += "  " + statusBar.Render("[prefix]")
	}
	if a.statusMsg != "" {
		bar += "  " + errorText.Render(a.statusMsg)
	}
	return bar
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -v`
Expected: PASS — every test in the `tui` package.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat: root app model with tab bar and prefix state machine"
```

---

## Task 15: Wire up main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Replace `main.go`**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	gossh "golang.org/x/crypto/ssh"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
	"mterm/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mterm:", err)
		os.Exit(1)
	}
}

func run() error {
	res, err := config.Load()
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "mterm: warning:", w)
	}
	if len(res.Hosts) == 0 {
		return fmt.Errorf("no hosts found in ~/.ssh/config or ~/.config/mterm/hosts.yaml")
	}

	auth, err := mssh.AgentProviderFromEnv()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")

	connect := func(host config.Host, cols, rows int) (*mssh.Session, error) {
		// v1: trust-on-first-use accepts unknown keys automatically. A future
		// version routes this through an interactive modal.
		verifier := mssh.NewHostKeyVerifier(knownHostsPath,
			func(string, gossh.PublicKey) bool { return true })
		sess := mssh.NewSession(host, auth, verifier.Callback())
		if err := sess.Connect(cols, rows); err != nil {
			return nil, err
		}
		return sess, nil
	}

	app := tui.NewApp(res.Hosts, connect)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
```

- [ ] **Step 2: Verify the whole project builds and all tests pass**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build succeeds, vet is clean, every package's tests PASS.

- [ ] **Step 3: Manual smoke test**

Ensure an ssh-agent is running with a key (`ssh-add -l` lists at least one key)
and `~/.ssh/config` has at least one reachable host. Then:

```bash
go run .
```

Verify, in order:
1. The picker lists hosts; typing filters them.
2. `Enter` opens a host as a tab; the remote shell is interactive.
3. A full-screen program works inside the tab — run `htop` or `vim` on the
   remote and confirm it renders.
4. `Ctrl-B c` returns to the picker; opening a second host adds a second tab.
5. `Ctrl-B n` / `Ctrl-B p` and `Ctrl-B 1` / `Ctrl-B 2` switch tabs.
6. `Ctrl-B f` opens the forwards panel; `Esc` closes it.
7. `Ctrl-B x` closes a tab; `Ctrl-B q` quits.

Document any deviations; fix before committing if they are defects.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: wire config, ssh, and tui into the mterm entrypoint"
```

---

## Deferred to a follow-up plan

The spec lists these as v1 features but they need a second plan because they
depend on Bubble Tea wiring that only exists after Task 15, and on the render
ticker actually running:

- **Auto-reconnect integration in the TUI** — Task 8 builds the
  `Session.AutoReconnect` mechanism and tests it. Wiring a dropped session to
  trigger reconnect and show a "reconnecting…" banner in the tab belongs in the
  follow-up, because it needs the running app loop and a `tea.Msg` for state
  changes.
- **Scrollback / copy mode** — `Ctrl-B [`, scrollback navigation, selection,
  and `atotto/clipboard` yank. This needs the `terminal` package to expose
  scrollback access, which depends on the verified x/vt API from Task 4.
- **Forwards panel → live forwards** — Task 13 builds the panel and toggle
  state; Task 9 builds `Session.StartForward`. Connecting a toggle to actually
  starting/stopping a `PortForward` belongs in the follow-up.

After Task 15, start a new brainstorming/planning cycle for "mterm v1 polish:
reconnect, copy mode, live forwards" to finish these three. Tasks 1–15 deliver a
working, tested tabbed SSH client.

## Self-Review notes

- **Spec coverage:** picker, tabs, VT emulation, ssh_config+yaml merge, agent
  auth, known_hosts TOFU, native SSH, `Ctrl-B` prefix, port-forward
  parsing/panel/StartForward, auto-reconnect mechanism — all have tasks.
  Auto-reconnect *integration*, copy mode, and live-forward toggling are
  explicitly carried into a named follow-up plan above (they are v1 scope but
  not deliverable until the app loop exists).
- **Type consistency:** `Session`, `State`, `Forward`, `Host`, `sessionTab`,
  `App`, `picker`, `forwardsPanel` names are used identically across tasks.
- **No placeholders:** every code step contains complete code; the only
  flagged-for-verification items are two external library APIs, with the local
  contract (wrapper interface) pinned so dependents are unaffected.
