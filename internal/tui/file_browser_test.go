package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFormatBytesRanges(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1024*1024 + 512*1024, "1.5 MB"},
		{1024 * 1024 * 1024 * 3, "3.0 GB"},
	}
	for _, tc := range cases {
		if got := formatBytes(tc.in); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParentPathLocalAndRemote(t *testing.T) {
	cases := []struct {
		in     string
		remote bool
		want   string
	}{
		{"/", true, "/"},
		{"/home", true, "/"},
		{"/home/dale", true, "/home"},
		{"/home/dale/", true, "/home"},
		{"/", false, "/"},
		{"/Users/dale", false, "/Users"},
	}
	for _, tc := range cases {
		if got := parentPath(tc.in, tc.remote); got != tc.want {
			t.Errorf("parentPath(%q, remote=%v) = %q, want %q", tc.in, tc.remote, got, tc.want)
		}
	}
}

func TestSortFileEntriesDirsFirstThenAlpha(t *testing.T) {
	in := []fileEntry{
		{Name: "Zfile", IsDir: false},
		{Name: "alpha-dir", IsDir: true},
		{Name: "AFile", IsDir: false},
		{Name: "beta-dir", IsDir: true},
	}
	sortFileEntries(in)
	want := []string{"alpha-dir", "beta-dir", "AFile", "Zfile"}
	got := []string{in[0].Name, in[1].Name, in[2].Name, in[3].Name}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort: got %v, want %v", got, want)
		}
	}
}

func TestUpdateTabSwitchesActivePane(t *testing.T) {
	m := newFileBrowserModel(nil)
	if m.active != sideLocal {
		t.Fatalf("default active = %v, want sideLocal", m.active)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.active != sideRemote {
		t.Fatalf("after Tab, active = %v, want sideRemote", m.active)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.active != sideLocal {
		t.Fatalf("after second Tab, active = %v, want sideLocal", m.active)
	}
}

func TestUpdateArrowsMoveCursorInActivePane(t *testing.T) {
	m := newFileBrowserModel(nil)
	m.localEntries = []fileEntry{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.localCursor != 1 {
		t.Fatalf("after Down, cursor = %d, want 1", m.localCursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // beyond — should clamp
	if m.localCursor != 2 {
		t.Fatalf("after 3xDown on 3 entries, cursor = %d, want 2", m.localCursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.localCursor != 1 {
		t.Fatalf("after Up, cursor = %d, want 1", m.localCursor)
	}
}

func TestEscWithoutTransferEmitsCloseMsg(t *testing.T) {
	m := newFileBrowserModel(nil)
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc should return a cmd emitting fileBrowserClosedMsg")
	}
	if _, ok := cmd().(fileBrowserClosedMsg); !ok {
		t.Fatalf("Esc cmd() = %T, want fileBrowserClosedMsg", cmd())
	}
}

func TestEscDuringTransferCancelsInsteadOfClosing(t *testing.T) {
	m := newFileBrowserModel(nil)
	canceled := false
	m.transfer = &transferState{
		cancel: func() { canceled = true },
	}
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc during transfer must not emit close msg")
	}
	if !canceled {
		t.Fatal("Esc during transfer should call transfer.cancel()")
	}
}

func TestRefreshKeyEmitsRefreshMsg(t *testing.T) {
	m := newFileBrowserModel(nil)
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("r should emit refresh cmd")
	}
	if _, ok := cmd().(fileBrowserRefreshMsg); !ok {
		t.Fatalf("r cmd() = %T, want fileBrowserRefreshMsg", cmd())
	}
}

func TestHiddenToggleAlsoEmitsRefresh(t *testing.T) {
	m := newFileBrowserModel(nil)
	before := m.showHidden
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if m.showHidden == before {
		t.Fatal("'.' should toggle showHidden")
	}
	if cmd == nil {
		t.Fatal("'.' should also emit a refresh cmd")
	}
	if _, ok := cmd().(fileBrowserRefreshMsg); !ok {
		t.Fatalf("'.' cmd() = %T, want fileBrowserRefreshMsg", cmd())
	}
}

func TestViewContainsBothPanes(t *testing.T) {
	m := newFileBrowserModel(nil)
	m.localPath = "/local/path"
	m.remotePath = "/remote/path"
	m.termW, m.termH = 100, 30
	m.localEntries = []fileEntry{{Name: "alpha", IsDir: false, Size: 1234}}
	m.remoteEntries = []fileEntry{{Name: "beta-dir", IsDir: true}}
	out := m.View()
	for _, want := range []string{"LOCAL", "REMOTE", "alpha", "beta-dir", "1.2 KB"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q in output", want)
		}
	}
}
