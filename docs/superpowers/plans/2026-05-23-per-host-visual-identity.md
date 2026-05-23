# Per-Host Visual Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-host `borderColor` in `hosts.yaml` recolors the frame, active tab, and footer host-info when that host's tab is the active session. Other modes (picker / palette / help / forwards) and the session body are unaffected.

**Architecture:** Add an optional `borderColor` (hex) field to the mterm config overlay. New helper `chromeStylesFor(c string) styleSet` derives a per-host styleSet from a hex color, falling back to the global `sty` when empty. Refactor the chrome render helpers (`renderWindow`, `renderTabStrip`, `renderFooter`, and their internals) to take a `styleSet` parameter; session-mode rendering passes the per-host set, all other modes pass the global `sty`.

**Tech Stack:** Go, Bubble Tea v1.3.10, Lip Gloss v1.1.0, `github.com/charmbracelet/x/{ansi,vt}`, `gopkg.in/yaml.v3`.

---

## File Structure

**New code:**
- `internal/config/border_color.go` — `parseBorderColor` validator
- `internal/config/border_color_test.go` — validator tests

**Modified:**
- `internal/config/types.go` — add `Host.BorderColor`
- `internal/config/mterm.go` — add `mtermHost.BorderColor` yaml tag
- `internal/config/config.go` — `applyOverlay` and `hostFromMterm` carry the field; `loadAndMerge` validates and warns
- `internal/config/config_test.go` — overlay + parsing integration
- `internal/tui/theme.go` — add `chromeStylesFor`
- `internal/tui/theme_test.go` — verify derived styleSet foregrounds
- `internal/tui/frame.go` — render helpers take `styleSet`
- `internal/tui/frame_test.go` — pass `sty` in calls
- `internal/tui/tabstrip.go` — `renderTabStrip` takes `styleSet`
- `internal/tui/tabstrip_test.go` — pass `sty` in calls
- `internal/tui/footer.go` — `renderFooter` takes `styleSet`
- `internal/tui/footer_test.go` — pass `sty` in calls
- `internal/tui/app.go` — `View()` session path picks per-host styleSet from active tab's host
- `internal/tui/app_test.go` — integration test for session-mode color override

---

## Task 1: Config — `borderColor` field + validator

**Files:**
- Create: `internal/config/border_color.go`
- Create: `internal/config/border_color_test.go`
- Modify: `internal/config/types.go`
- Modify: `internal/config/mterm.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

### Step 1.1: Failing test for `parseBorderColor`

- [ ] Write `internal/config/border_color_test.go`:

```go
package config

import "testing"

func TestParseBorderColor(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"#FF3344", "#FF3344", false},
		{"#ff3344", "#ff3344", false},
		{"#F33", "#F33", false},
		{"#f33", "#f33", false},
		{"red", "", true},
		{"#FF334", "", true},     // 6 chars total — wrong length
		{"#FF33445", "", true},   // 8 chars total — wrong length
		{"#GGGGGG", "", true},    // non-hex
		{"FF3344", "", true},     // missing #
		{"#12 345", "", true},    // space in hex
	}
	for _, tc := range cases {
		got, err := parseBorderColor(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseBorderColor(%q) want error, got nil", tc.in)
			}
			if got != "" {
				t.Errorf("parseBorderColor(%q) on error want \"\", got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseBorderColor(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseBorderColor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
```

### Step 1.2: Run test and verify it fails

- [ ] Run: `go test ./internal/config/ -run TestParseBorderColor -v`
  Expected: build failure ("parseBorderColor undefined").

### Step 1.3: Implement `parseBorderColor`

- [ ] Write `internal/config/border_color.go`:

```go
package config

import "fmt"

// parseBorderColor validates a host borderColor value. Empty input is allowed
// and returns ("", nil) — the host has no override. A non-empty value must be
// a 7-char "#RRGGBB" or 4-char "#RGB" hex string; anything else returns an
// error and an empty result so callers can warn and continue.
func parseBorderColor(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if len(s) != 7 && len(s) != 4 {
		return "", fmt.Errorf("want #RRGGBB or #RGB")
	}
	if s[0] != '#' {
		return "", fmt.Errorf("want #RRGGBB or #RGB")
	}
	for _, c := range s[1:] {
		if !isHexDigit(c) {
			return "", fmt.Errorf("want #RRGGBB or #RGB")
		}
	}
	return s, nil
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
}
```

### Step 1.4: Verify the validator tests pass

- [ ] Run: `go test ./internal/config/ -run TestParseBorderColor -v`
  Expected: PASS.

### Step 1.5: Add `BorderColor` to schema types

- [ ] In `internal/config/types.go`, add to `Host`:

```go
type Host struct {
	Name         string
	HostName     string
	User         string
	Port         int
	Group        string
	Tags         []string
	Source       Source
	IdentityFile string
	ProxyJump    string
	Forwards     []Forward
	BorderColor  string // hex like "#RRGGBB" or "#RGB"; "" = no override
}
```

- [ ] In `internal/config/mterm.go`, add to `mtermHost`:

```go
type mtermHost struct {
	Name        string         `yaml:"name"`
	HostName    string         `yaml:"hostName"`
	User        string         `yaml:"user"`
	Port        int            `yaml:"port"`
	Group       string         `yaml:"group"`
	Tags        []string       `yaml:"tags"`
	Forwards    []mtermForward `yaml:"forwards"`
	BorderColor string         `yaml:"borderColor"`
}
```

### Step 1.6: Failing test — overlay + parsing through `Load`

- [ ] Add to `internal/config/config_test.go` (alongside other `loadAndMerge` tests):

```go
func TestLoadAndMergeBorderColorValid(t *testing.T) {
	dir := t.TempDir()
	mtermPath := filepath.Join(dir, "hosts.yaml")
	if err := os.WriteFile(mtermPath, []byte(`hosts:
  - name: prod
    borderColor: "#FF3344"
  - name: dev
    borderColor: "#F33"
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
    borderColor: "red"
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
		t.Fatal("want a warning for invalid borderColor, got none")
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "badhost") && strings.Contains(w, "borderColor") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v, want one naming host+borderColor", res.Warnings)
	}
}
```

Add `"reflect"` and `"strings"` to the test file's import list if not already present.

### Step 1.7: Run the failing tests

- [ ] Run: `go test ./internal/config/ -run BorderColor -v`
  Expected: tests run but FAIL (BorderColor is parsed by yaml but never validated; warnings won't appear).

### Step 1.8: Wire `BorderColor` through merge

- [ ] In `internal/config/config.go` `applyOverlay`, append after the `Forwards` loop:

```go
	if mh.BorderColor != "" {
		h.BorderColor = mh.BorderColor
	}
```

- [ ] In `internal/config/config.go` `hostFromMterm`, set `BorderColor: mh.BorderColor` in the returned `Host` literal.

- [ ] In `internal/config/config.go` `loadAndMerge`, validate every host's `BorderColor` after the final ordering+sort. Replace the `for _, name := range order` block + sort with:

```go
	for _, name := range order {
		h := *byName[name]
		if h.BorderColor != "" {
			parsed, err := parseBorderColor(h.BorderColor)
			if err != nil {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("mterm config: host %q borderColor %q: %v", h.Name, h.BorderColor, err))
				h.BorderColor = ""
			} else {
				h.BorderColor = parsed
			}
		}
		res.Hosts = append(res.Hosts, h)
	}
	sort.SliceStable(res.Hosts, func(i, j int) bool {
		a, b := res.Hosts[i], res.Hosts[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Name < b.Name
	})
```

### Step 1.9: Verify all config tests pass

- [ ] Run: `go test ./internal/config/ -v`
  Expected: PASS, no regressions.

### Step 1.10: Commit

- [ ] Run:

```bash
git add internal/config/border_color.go internal/config/border_color_test.go \
        internal/config/types.go internal/config/mterm.go internal/config/config.go \
        internal/config/config_test.go
git commit -m "feat(config): add borderColor field with hex validation"
```

---

## Task 2: Theme — `chromeStylesFor`

**Files:**
- Modify: `internal/tui/theme.go`
- Modify: `internal/tui/theme_test.go`

### Step 2.1: Failing test for `chromeStylesFor`

- [ ] Add to `internal/tui/theme_test.go`:

```go
func TestChromeStylesForEmptyReturnsGlobal(t *testing.T) {
	s := chromeStylesFor("")
	if s.border.GetForeground() != sty.border.GetForeground() {
		t.Fatalf("empty override should match global sty.border foreground")
	}
	if s.title.GetForeground() != sty.title.GetForeground() {
		t.Fatalf("empty override should match global sty.title foreground")
	}
	if s.tabActive.GetForeground() != sty.tabActive.GetForeground() {
		t.Fatalf("empty override should match global sty.tabActive foreground")
	}
	if s.footerInfo.GetForeground() != sty.footerInfo.GetForeground() {
		t.Fatalf("empty override should match global sty.footerInfo foreground")
	}
}

func TestChromeStylesForOverridesHostTintedStyles(t *testing.T) {
	want := lipgloss.Color("#FF3344")
	s := chromeStylesFor("#FF3344")
	for name, got := range map[string]lipgloss.TerminalColor{
		"border":     s.border.GetForeground(),
		"title":      s.title.GetForeground(),
		"tabActive":  s.tabActive.GetForeground(),
		"footerInfo": s.footerInfo.GetForeground(),
	} {
		if got != want {
			t.Errorf("%s foreground = %v, want %v", name, got, want)
		}
	}
}

func TestChromeStylesForPreservesUntintedStyles(t *testing.T) {
	s := chromeStylesFor("#FF3344")
	// These stay theme colors and must not be overridden.
	if s.dim.GetForeground() != sty.dim.GetForeground() {
		t.Errorf("dim foreground should not change")
	}
	if s.errorText.GetForeground() != sty.errorText.GetForeground() {
		t.Errorf("errorText foreground should not change")
	}
	if s.connected.GetForeground() != sty.connected.GetForeground() {
		t.Errorf("connected foreground should not change")
	}
}
```

If `lipgloss` is not already imported in `theme_test.go`, add `"github.com/charmbracelet/lipgloss"`.

### Step 2.2: Run, verify failure

- [ ] Run: `go test ./internal/tui/ -run ChromeStylesFor -v`
  Expected: build failure ("chromeStylesFor undefined").

### Step 2.3: Implement `chromeStylesFor`

- [ ] In `internal/tui/theme.go`, append:

```go
// chromeStylesFor returns a styleSet identical to the active set, except that
// the four host-tinted styles use c instead of the theme's Frame/Accent.
// When c is "", returns the global sty unchanged so callers can use this
// unconditionally.
func chromeStylesFor(c string) styleSet {
	if c == "" {
		return sty
	}
	col := lipgloss.Color(c)
	s := sty
	s.border = lipgloss.NewStyle().Foreground(col)
	s.title = lipgloss.NewStyle().Foreground(col).Bold(true)
	s.tabActive = lipgloss.NewStyle().Foreground(col).Bold(true)
	s.footerInfo = lipgloss.NewStyle().Foreground(col)
	return s
}
```

### Step 2.4: Verify

- [ ] Run: `go test ./internal/tui/ -run ChromeStylesFor -v`
  Expected: PASS.

### Step 2.5: Commit

```bash
git add internal/tui/theme.go internal/tui/theme_test.go
git commit -m "feat(tui): add chromeStylesFor host-color override"
```

---

## Task 3: Render helpers take `styleSet`

This is a pure refactor — behavior unchanged; existing tests pass once they're updated to pass `sty` explicitly.

**Files:**
- Modify: `internal/tui/frame.go`
- Modify: `internal/tui/frame_test.go`
- Modify: `internal/tui/tabstrip.go`
- Modify: `internal/tui/tabstrip_test.go`
- Modify: `internal/tui/footer.go`
- Modify: `internal/tui/footer_test.go`
- Modify: `internal/tui/app.go` (call-site updates)

### Step 3.1: Update `frame.go` signatures

- [ ] In `internal/tui/frame.go`:

  - `renderWindow(o windowOpts)` → `renderWindow(o windowOpts, s styleSet)`. Inside the function, replace every `sty.` with `s.`. The internal helper calls (`topBorder`, `sideRow`, `divider`, `bottomBorder`) all need `s` threaded through.
  - `topBorder(title string, width int)` → `topBorder(title string, width int, s styleSet)`. Replace `sty.border` with `s.border`.
  - `bottomBorder(width int)` → `bottomBorder(width int, s styleSet)`. Same replacement.
  - `divider(width int)` → `divider(width int, s styleSet)`. Same replacement.
  - `sideRow(content string, innerWidth int)` → `sideRow(content string, innerWidth int, s styleSet)`. Same replacement.
  - `clampLine` does not reference `sty`; leave unchanged.

### Step 3.2: Update `frame_test.go`

- [ ] In every `renderWindow(...)` call in `internal/tui/frame_test.go`, add `, sty` as the second argument.

### Step 3.3: Update `tabstrip.go`

- [ ] `renderTabStrip(tabs []*sessionTab, activeIdx, spinnerCounter, width int)` → `renderTabStrip(tabs []*sessionTab, activeIdx, spinnerCounter, width int, s styleSet)`. Replace `sty.tabActive` / `sty.tabInactive` with `s.tabActive` / `s.tabInactive`.
- [ ] `statusGlyph` references `sty.connected`, `sty.errorText`, `sty.footerKey` — these are intentionally NOT host-tinted (connection-state semantics), so leave `statusGlyph` reading the global `sty` directly. Don't thread `s` into it.

### Step 3.4: Update `tabstrip_test.go`

- [ ] In every `renderTabStrip(...)` call in `internal/tui/tabstrip_test.go`, append `, sty` as the new argument.

### Step 3.5: Update `footer.go`

- [ ] `renderFooter(o footerOpts)` → `renderFooter(o footerOpts, s styleSet)`. Replace every `sty.` reference inside `renderFooter` with `s.`.

### Step 3.6: Update `footer_test.go`

- [ ] In every `renderFooter(...)` call in `internal/tui/footer_test.go`, add `, sty` as the second argument.

### Step 3.7: Update call sites in `app.go`

- [ ] Locate every call to `renderWindow`, `renderTabStrip`, `renderFooter` in `internal/tui/app.go` (also check whether any other file calls them). Pass `sty` as the new last argument for every call. Task 4 will replace `sty` with `chromeStylesFor(...)` only for the session-mode path.

### Step 3.8: Verify

- [ ] Run: `go build ./...`
  Expected: builds clean.
- [ ] Run: `go test ./internal/tui/ -v`
  Expected: all existing tests still PASS (pure refactor).

### Step 3.9: Commit

```bash
git add internal/tui/frame.go internal/tui/frame_test.go \
        internal/tui/tabstrip.go internal/tui/tabstrip_test.go \
        internal/tui/footer.go internal/tui/footer_test.go \
        internal/tui/app.go
git commit -m "refactor(tui): render helpers take explicit styleSet"
```

---

## Task 4: Wire per-host styleSet into session-mode rendering

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

### Step 4.1: Failing integration test

- [ ] Add to `internal/tui/app_test.go`:

```go
func TestAppSessionViewUsesHostBorderColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	host := config.Host{Name: "prod", HostName: "prod", Port: 22, BorderColor: "#FF3344"}
	app.tabs = []*sessionTab{newSessionTab(1, host, 80, 24)}
	app.active = 0

	got := app.View()

	// Truecolor escape for #FF3344 is "\x1b[38;2;255;51;68m"
	want := "\x1b[38;2;255;51;68m"
	if !strings.Contains(got, want) {
		t.Fatalf("session view missing host border color escape %q\noutput:\n%s", want, got)
	}
}

func TestAppSessionViewIgnoresEmptyBorderColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	host := config.Host{Name: "plain", HostName: "plain", Port: 22}
	app.tabs = []*sessionTab{newSessionTab(1, host, 80, 24)}
	app.active = 0

	got := app.View()
	notWant := "\x1b[38;2;255;51;68m"
	if strings.Contains(got, notWant) {
		t.Fatalf("session view contains unexpected color escape %q", notWant)
	}
}
```

Add imports if missing:

```go
import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)
```

### Step 4.2: Run, verify failure

- [ ] Run: `go test ./internal/tui/ -run "TestAppSessionViewUsesHostBorderColor|TestAppSessionViewIgnoresEmptyBorderColor" -v`
  Expected: FAIL — the first test cannot find the color escape; the second passes.

### Step 4.3: Pick per-host styleSet in the session view path

- [ ] In `internal/tui/app.go`, locate the `case modeSession:` branch of `App.View()`. Where the render helpers are called, replace the global `sty` argument with a derived styleSet:

```go
case modeSession:
	t := a.activeTab()
	s := sty
	if t != nil {
		s = chromeStylesFor(t.host.BorderColor)
	}
	// then pass `s` to renderTabStrip, renderFooter, and renderWindow for
	// the session-mode body.
```

Other `case` arms (`modePicker`, `modeForwards`, `modePalette`, `modeHelp`) continue passing the global `sty` directly.

### Step 4.4: Verify

- [ ] Run: `go test ./internal/tui/ -run "TestAppSessionView" -v`
  Expected: PASS.
- [ ] Run: `go test ./... -race`
  Expected: PASS across all packages.

### Step 4.5: Commit

```bash
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): apply host borderColor to active session chrome"
```

---

## Task 5: Final verification + branch handoff

### Step 5.1: Static checks

- [ ] Run: `gofmt -l .`
  Expected: empty output.
- [ ] Run: `go vet ./...`
  Expected: no issues.
- [ ] Run: `go test ./... -race`
  Expected: PASS.

### Step 5.2: Manual smoke

- [ ] Add a test entry to your local `~/.config/mterm/hosts.yaml`:

```yaml
hosts:
  - name: <pick a real host you can connect to>
    borderColor: "#FF3344"
```

- [ ] Run: `go run .`, connect to that host, verify red frame + red active tab brackets + red `user@host:port`. Switch to a tab without `borderColor` (or back to the picker) — chrome reverts to the theme.

### Step 5.3: Hand off to finishing-a-development-branch

- [ ] Final reviewer pass, then invoke the `superpowers:finishing-a-development-branch` skill to present merge/PR/keep/discard options.
