package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"mterm/internal/config"
)

// ansiRE strips every CSI escape (SGR colors, attributes, etc.) so tests can
// assert on visible text without coupling to lipgloss's specific ANSI output.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

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
	p.moveCursor(1) // skip lab group header
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

func TestPickerCursorWrapsAround(t *testing.T) {
	p := newPicker(sampleHosts())
	p.moveCursor(-1)
	h, ok := p.selected()
	if !ok || h.Name != "lab-box" {
		t.Fatalf("moveCursor(-1) from 0 = %v,%v; want lab-box,true", h.Name, ok)
	}
	p.moveCursor(1)
	if row := p.cursorRow(); row == nil || row.kind != pickerRowGroup {
		t.Fatalf("moveCursor(1) from last should land on first row (group header)")
	}
}

func TestPickerUpdateKeyDispatch(t *testing.T) {
	p := newPicker(sampleHosts())

	// Row 0 = production header, row 1 = prod-web, row 2 = prod-db
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	h, ok := p.selected()
	if !ok || h.Name != "prod-db" {
		t.Fatalf("after two KeyDown selected = %v,%v; want prod-db,true", h.Name, ok)
	}

	cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("KeyEnter returned nil cmd, want non-nil")
	}
	msg := cmd()
	chosen, ok := msg.(pickerChosenMsg)
	if !ok {
		t.Fatalf("cmd() type = %T; want pickerChosenMsg", msg)
	}
	if chosen.host.Name != "prod-db" {
		t.Fatalf("pickerChosenMsg.host = %v; want prod-db", chosen.host.Name)
	}

	cmd = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("KeyEsc returned nil cmd, want non-nil")
	}
	msg = cmd()
	if _, ok := msg.(pickerCancelledMsg); !ok {
		t.Fatalf("cmd() type = %T; want pickerCancelledMsg", msg)
	}
}

func TestPickerViewEmptyDoesNotPanic(t *testing.T) {
	p := newPicker(nil)
	s := p.View()
	if s == "" {
		t.Fatal("View() with nil hosts returned empty string")
	}

	p2 := newPicker(sampleHosts())
	p2.setQuery("zzzznomatch")
	s2 := p2.View()
	if s2 == "" {
		t.Fatal("View() with no-match query returned empty string")
	}
	if len(p2.visibleHosts()) != 0 {
		t.Fatal("visibleHosts() with no-match query should be empty")
	}
}

func TestPickerSelectedGroupHeaderHasNoEmbeddedAccentANSI(t *testing.T) {
	t.Cleanup(func() { setTheme(Midnight) })
	setTheme(Matrix)

	row := pickerRow{kind: pickerRowGroup, label: "production", path: "production", depth: 0, count: 2}
	width := 40
	line := groupHeaderLine(row.label, row.depth, row.count, true, "> ")
	want := sty.selectionBar.Width(width).Render(ansi.Truncate(line, width, ""))
	got := renderPickerRow(row, map[string]bool{}, pickerDecorations{}, width, true)
	if got != want {
		t.Fatalf("selected group header should match plain selection bar render\ngot:  %q\nwant: %q", got, want)
	}
}

func TestPickerSelectionBarIsFullWidth(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.moveCursor(1) // prod-web host row
	out := p.View()
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "prod-web") {
			if w := lipgloss.Width(ln); w != p.w {
				t.Fatalf("selection bar width = %d, want %d (full width)", w, p.w)
			}
			return
		}
	}
	t.Fatal("could not find the selected host row in the picker view")
}

func TestPickerViewHasSelectionAndGroups(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	out := p.View()
	for _, want := range []string{"▾ production", "▾ lab"} {
		if !strings.Contains(out, want) {
			t.Fatalf("picker body missing group header %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("picker body has no styling at all:\n%s", out)
	}
}

func TestPickerSelectedGroupHeaderMarkerFollowsIndent(t *testing.T) {
	// Mirrors Work/MSFT/Infra from a typical nested hosts.yaml.
	row := pickerRow{kind: pickerRowGroup, label: "Infra", path: "Work/MSFT/Infra", depth: 2, count: 3}
	stripped := ansiRE.ReplaceAllString(renderPickerRow(row, map[string]bool{}, pickerDecorations{}, 80, true), "")
	if !strings.HasPrefix(stripped, "    > ▾ Infra") {
		t.Fatalf("selected group header marker should follow depth indent; got %q", stripped)
	}
	unsel := ansiRE.ReplaceAllString(renderPickerRow(row, map[string]bool{}, pickerDecorations{}, 80, false), "")
	if !strings.HasPrefix(unsel, "      ▾ Infra") {
		t.Fatalf("unselected group header should reserve a marker column; got %q", unsel)
	}
}

func TestPickerGroupHeaderPreservesYamlCase(t *testing.T) {
	hosts := []config.Host{
		{Name: "a", Group: "PRODUCTION/Web"},
		{Name: "b", Group: "PRODUCTION/DB"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	out := ansiRE.ReplaceAllString(p.View(), "")
	for _, want := range []string{"▾ PRODUCTION", "    ▾ Web", "    ▾ DB"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected header %q in:\n%s", want, out)
		}
	}
}

func TestPickerShowsColumnHeader(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	stripped := ansiRE.ReplaceAllString(p.View(), "")
	if !strings.Contains(stripped, "NAME") || !strings.Contains(stripped, "ADDRESS") || !strings.Contains(stripped, "LAST") {
		t.Fatalf("picker should show column headers; got:\n%s", stripped)
	}
}

func TestPickerNestedGroupsRenderHierarchy(t *testing.T) {
	hosts := []config.Host{
		{Name: "binarr", HostName: "192.168.2.12", Group: "Home/Media"},
		{Name: "plex", HostName: "192.168.2.50", Group: "Home/Media"},
		{Name: "sys-dev", HostName: "192.168.2.20", Group: "Home/Development"},
		{Name: "box1", HostName: "10.0.1.1", Group: "Work/Project1"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	out := p.View()

	for _, want := range []string{"▾ Home", "▾ Media", "▾ Development", "▾ Work", "▾ Project1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing header %q in:\n%s", want, out)
		}
	}
	for _, want := range []string{"    ▾ Media", "    ▾ Development", "    ▾ Project1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing indented header %q in:\n%s", want, out)
		}
	}
}

func TestPickerSiblingsShareSingleParentHeader(t *testing.T) {
	hosts := []config.Host{
		{Name: "a", Group: "Home/Media"},
		{Name: "b", Group: "Home/Development"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	out := p.View()
	if got := strings.Count(out, "▾ Home"); got != 1 {
		t.Fatalf("expected exactly one Home header, got %d in:\n%s", got, out)
	}
}

func TestPickerNestedHostRowIndented(t *testing.T) {
	hosts := []config.Host{
		{Name: "alpha", HostName: "1.1.1.1", Group: "Home/Media"},
		{Name: "beta", HostName: "2.2.2.2", Group: "Home/Media"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	p.moveCursor(2) // HOME header, MEDIA header, then alpha host
	out := p.View()
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "    > ○ alpha") {
		t.Fatalf("cursor row at depth 2 should start with %q; stripped:\n%s", "    > ○ alpha", stripped)
	}
	if !strings.Contains(stripped, "      ○ beta") {
		t.Fatalf("non-cursor row at depth 2 should be indented + have glyph; stripped:\n%s", stripped)
	}
}

func TestPickerConnectionGlyphsReflectDeco(t *testing.T) {
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	hosts := []config.Host{
		{Name: "open-now"},
		{Name: "ever"},
		{Name: "never"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	p.setDecorations(pickerDecorations{
		OpenHosts:     map[string]bool{"open-now": true},
		LastConnected: map[string]time.Time{"ever": now.Add(-2 * time.Hour)},
		Now:           now,
	})
	stripped := ansiRE.ReplaceAllString(p.View(), "")

	if !strings.Contains(stripped, "◉ open-now") {
		t.Errorf("open-now should render with ◉; got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "● ever") {
		t.Errorf("ever should render with ●; got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "○ never") {
		t.Errorf("never should render with ○; got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "2h ago") {
		t.Errorf("ever should show '2h ago'; got:\n%s", stripped)
	}
}

func TestPickerGroupHeaderShowsCount(t *testing.T) {
	hosts := []config.Host{
		{Name: "a", Group: "Home/Media"},
		{Name: "b", Group: "Home/Media"},
		{Name: "c", Group: "Home/Development"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	out := ansiRE.ReplaceAllString(p.View(), "")

	for _, want := range []string{"▾ Home · 3", "▾ Media · 2", "▾ Development · 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected header %q in:\n%s", want, out)
		}
	}
}

func TestPickerCollapseHidesHosts(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setCollapsed("production", true)
	out := ansiRE.ReplaceAllString(p.View(), "")
	if strings.Contains(out, "prod-web") || strings.Contains(out, "prod-db") {
		t.Fatalf("collapsed production should hide hosts; got:\n%s", out)
	}
	if !strings.Contains(out, "▸ production") {
		t.Fatalf("collapsed production header should remain visible")
	}
	if !strings.Contains(out, "lab-box") {
		t.Fatalf("lab hosts should remain visible")
	}
}

func TestPickerNestedCollapseHidesSubtree(t *testing.T) {
	hosts := []config.Host{
		{Name: "a", Group: "Home/Media"},
		{Name: "b", Group: "Home/Development"},
		{Name: "c", Group: "Work/Project1"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	p.setCollapsed("Home", true)
	out := ansiRE.ReplaceAllString(p.View(), "")
	if strings.Contains(out, "○ a") || strings.Contains(out, "○ b") {
		t.Fatalf("collapsed Home should hide all descendants; got:\n%s", out)
	}
	if !strings.Contains(out, "▸ Home") {
		t.Fatalf("Home header should remain")
	}
	if !strings.Contains(out, "c") {
		t.Fatalf("Work group should remain visible")
	}
}

func TestPickerCollapsePreservesChildState(t *testing.T) {
	hosts := []config.Host{
		{Name: "a", Group: "Home/Media"},
		{Name: "b", Group: "Home/Development"},
	}
	p := newPicker(hosts)
	p.setCollapsed("Home/Media", true)
	p.setCollapsed("Home", true)
	p.setCollapsed("Home", false) // re-expand Home; Media stays collapsed
	out := ansiRE.ReplaceAllString(p.View(), "")
	if strings.Contains(out, "○ a") {
		t.Fatalf("Media should stay collapsed after re-expanding Home; got:\n%s", out)
	}
	if !strings.Contains(out, "○ b") {
		t.Fatalf("Development host should be visible; got:\n%s", out)
	}
}

func TestPickerSearchOverridesCollapse(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setCollapsed("production", true)
	p.setQuery("prod-db")
	out := ansiRE.ReplaceAllString(p.View(), "")
	if !strings.Contains(out, "prod-db") {
		t.Fatalf("search should show matching host despite collapse; got:\n%s", out)
	}
	p.setQuery("")
	out = ansiRE.ReplaceAllString(p.View(), "")
	if strings.Contains(out, "prod-db") {
		t.Fatalf("clearing search should restore collapse; got:\n%s", out)
	}
}

func TestPickerEnterOnHeaderToggles(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.Update(tea.KeyMsg{Type: tea.KeyEnter}) // toggle production header
	out := ansiRE.ReplaceAllString(p.View(), "")
	if strings.Contains(out, "prod-web") {
		t.Fatalf("Enter on production header should collapse; got:\n%s", out)
	}
	if !strings.Contains(out, "▸ production") {
		t.Fatalf("header should show collapsed glyph")
	}
}

func TestPickerSpaceOnHeaderToggles(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.Update(tea.KeyMsg{Type: tea.KeySpace})
	out := ansiRE.ReplaceAllString(p.View(), "")
	if strings.Contains(out, "prod-web") {
		t.Fatalf("Space on production header should collapse; got:\n%s", out)
	}
}

func TestPickerArrowKeysFoldHeader(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	out := ansiRE.ReplaceAllString(p.View(), "")
	if strings.Contains(out, "prod-web") {
		t.Fatalf("Left on expanded header should collapse; got:\n%s", out)
	}
	p.Update(tea.KeyMsg{Type: tea.KeyRight})
	out = ansiRE.ReplaceAllString(p.View(), "")
	if !strings.Contains(out, "prod-web") {
		t.Fatalf("Right on collapsed header should expand; got:\n%s", out)
	}
}

func TestPickerViewportShowsTruncationIndicator(t *testing.T) {
	hosts := make([]config.Host, 0, 20)
	for i := 0; i < 20; i++ {
		hosts = append(hosts, config.Host{
			Name:     fmt.Sprintf("host-%02d", i),
			HostName: fmt.Sprintf("10.0.0.%d", i),
			Group:    "big",
		})
	}
	p := newPicker(hosts)
	p.setSize(80, 12) // short terminal forces viewport
	p.cursor = 15
	out := p.View()
	if !strings.Contains(out, "more above") && !strings.Contains(out, "more below") {
		t.Fatalf("expected truncation indicator in short terminal view:\n%s", out)
	}
}

func TestBuildRowsCollapsedSiblingUnaffected(t *testing.T) {
	hosts := []config.Host{
		{Name: "a", Group: "production"},
		{Name: "b", Group: "lab"},
	}
	collapsed := map[string]bool{"production": true}
	rows := buildRows(hosts, collapsed)
	for _, r := range rows {
		if r.kind == pickerRowHost && r.host.Name == "a" {
			t.Fatal("production host should be hidden")
		}
	}
	foundLab := false
	for _, r := range rows {
		if r.kind == pickerRowHost && r.host.Name == "b" {
			foundLab = true
		}
	}
	if !foundLab {
		t.Fatal("lab host should remain visible")
	}
}
