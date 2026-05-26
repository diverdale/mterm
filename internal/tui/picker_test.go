package tui

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	// cursor starts at 0 (prod-web)
	p.moveCursor(-1)
	h, ok := p.selected()
	if !ok || h.Name != "lab-box" {
		t.Fatalf("moveCursor(-1) from 0 = %v,%v; want lab-box,true", h.Name, ok)
	}
	p.moveCursor(1)
	h, ok = p.selected()
	if !ok || h.Name != "prod-web" {
		t.Fatalf("moveCursor(1) from last = %v,%v; want prod-web,true", h.Name, ok)
	}
}

func TestPickerUpdateKeyDispatch(t *testing.T) {
	p := newPicker(sampleHosts())

	// KeyDown moves cursor to second host
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	h, ok := p.selected()
	if !ok || h.Name != "prod-db" {
		t.Fatalf("after KeyDown selected = %v,%v; want prod-db,true", h.Name, ok)
	}

	// KeyEnter returns a command that emits pickerChosenMsg
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

	// KeyEsc returns a command that emits pickerCancelledMsg
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
	// nil host list
	p := newPicker(nil)
	s := p.View()
	if s == "" {
		t.Fatal("View() with nil hosts returned empty string")
	}

	// non-nil hosts but query filters all out
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

func TestPickerSelectionBarIsFullWidth(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	out := p.View()
	// The cursor starts at row 0; find the rendered line for the first host
	// and confirm it spans the full picker width.
	h0 := p.visibleHosts()[0]
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, h0.Name) {
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
	// Group headers are uppercased and led by ▸ glyph.
	for _, want := range []string{"▸ PRODUCTION", "▸ LAB"} {
		if !strings.Contains(out, want) {
			t.Fatalf("picker body missing group header %q:\n%s", want, out)
		}
	}
	// The selected host's row carries styling (ANSI escape codes), since
	// the cursor row is a selection bar.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("picker body has no styling at all:\n%s", out)
	}
}

func TestPickerGroupHeaderHasBlankLineBetweenGroups(t *testing.T) {
	// Two non-empty groups → there should be a blank line between the last
	// host of group A and the header of group B (visual separation).
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	out := p.View()

	lines := strings.Split(out, "\n")
	prodIdx, labIdx := -1, -1
	for i, ln := range lines {
		if strings.Contains(ln, "▸ PRODUCTION") {
			prodIdx = i
		}
		if strings.Contains(ln, "▸ LAB") {
			labIdx = i
		}
	}
	if prodIdx < 0 || labIdx < 0 {
		t.Fatalf("missing one of the two group headers; lines:\n%s", out)
	}
	if labIdx <= prodIdx+2 {
		t.Fatalf("expected a blank line between groups; LAB at %d, PRODUCTION at %d", labIdx, prodIdx)
	}
	// Verify the line immediately preceding the LAB header is blank.
	if strings.TrimSpace(lines[labIdx-1]) != "" {
		t.Fatalf("line before LAB header is not blank: %q", lines[labIdx-1])
	}
}

func TestPathSegmentsCleansPath(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"Home", []string{"Home"}},
		{"Home/Media", []string{"Home", "Media"}},
		{"/Home/Media/", []string{"Home", "Media"}},
		{"Home//Media", []string{"Home", "Media"}},
		{"  Home  /  Media  ", []string{"Home", "Media"}},
		{"a/b/c/d", []string{"a", "b", "c", "d"}},
	}
	for _, tc := range cases {
		got := pathSegments(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("pathSegments(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestCommonPrefixLen(t *testing.T) {
	cases := []struct {
		a, b []string
		want int
	}{
		{nil, nil, 0},
		{[]string{"x"}, nil, 0},
		{[]string{"Home"}, []string{"Home"}, 1},
		{[]string{"Home", "Media"}, []string{"Home", "Dev"}, 1},
		{[]string{"Home", "Media"}, []string{"Home", "Media"}, 2},
		{[]string{"Home", "Media"}, []string{"Work", "Media"}, 0},
		{[]string{"A", "B", "C"}, []string{"A", "B"}, 2},
	}
	for _, tc := range cases {
		if got := commonPrefixLen(tc.a, tc.b); got != tc.want {
			t.Errorf("commonPrefixLen(%v, %v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
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

	// All four expected headers must appear.
	for _, want := range []string{"▸ HOME", "▸ MEDIA", "▸ DEVELOPMENT", "▸ WORK", "▸ PROJECT1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing header %q in:\n%s", want, out)
		}
	}

	// Sub-group headers must be indented (2 spaces per depth level).
	for _, want := range []string{"  ▸ MEDIA", "  ▸ DEVELOPMENT", "  ▸ PROJECT1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing indented header %q in:\n%s", want, out)
		}
	}
}

func TestPickerSiblingsShareSingleParentHeader(t *testing.T) {
	// Home/Media and Home/Development should produce ONE "▸ HOME" header,
	// then both sub-group headers under it.
	hosts := []config.Host{
		{Name: "a", Group: "Home/Media"},
		{Name: "b", Group: "Home/Development"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	out := p.View()
	if got := strings.Count(out, "▸ HOME"); got != 1 {
		t.Fatalf("expected exactly one HOME header, got %d in:\n%s", got, out)
	}
}

func TestPickerNestedHostRowIndented(t *testing.T) {
	// Two hosts at the same depth so we can check the cursor and a
	// non-cursor row separately.
	hosts := []config.Host{
		{Name: "alpha", HostName: "1.1.1.1", Group: "Home/Media"},
		{Name: "beta", HostName: "2.2.2.2", Group: "Home/Media"},
	}
	p := newPicker(hosts)
	p.setSize(80, 24)
	out := p.View()
	stripped := ansiRE.ReplaceAllString(out, "")

	// Depth 2 → 2*2 = 4 spaces of indent, then "> " for the cursor row or
	// "  " for non-cursor rows.
	if !strings.Contains(stripped, "    > alpha") {
		t.Fatalf("cursor row at depth 2 should start with %q; stripped:\n%s", "    > alpha", stripped)
	}
	if !strings.Contains(stripped, "      beta") {
		t.Fatalf("non-cursor row at depth 2 should have 6 leading spaces; stripped:\n%s", stripped)
	}
}
