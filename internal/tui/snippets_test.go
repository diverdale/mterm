package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mterm/internal/config"
)

func TestSnippetPayloadAppendsEnter(t *testing.T) {
	got := string(snippetPayload("show version"))
	if got != "show version\r" {
		t.Fatalf("got %q", got)
	}
}

func TestSnippetPayloadPreservesTrailingNewline(t *testing.T) {
	got := string(snippetPayload("line1\nline2\n"))
	if got != "line1\nline2\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSnippetsModelFilter(t *testing.T) {
	m := newSnippetsModel([]config.SnippetEntry{
		{Name: "uptime", Send: "uptime", Group: "Global"},
		{Name: "show version", Send: "show version", Group: "router"},
	})
	m.setQuery("show")
	v := m.visible()
	if len(v) != 1 || v[0].Name != "show version" {
		t.Fatalf("filter show = %+v", v)
	}
}

func TestSnippetsModelEnterEmitsChosen(t *testing.T) {
	m := newSnippetsModel([]config.SnippetEntry{
		{Name: "uptime", Send: "uptime", Group: "Global"},
	})
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg, ok := cmd().(snippetChosenMsg)
	if !ok || msg.send != "uptime" {
		t.Fatalf("got %T %+v", cmd(), msg)
	}
}

func TestSnippetsViewShowsGroups(t *testing.T) {
	m := newSnippetsModel([]config.SnippetEntry{
		{Name: "uptime", Send: "uptime", Group: "Global"},
		{Name: "reload", Send: "reload confirm", Group: "prod"},
	})
	out := m.View()
	if !strings.Contains(out, "Global") || !strings.Contains(out, "prod") {
		t.Fatalf("missing groups:\n%s", out)
	}
}

func TestAppPrefixEOpensSnippets(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "prod-router"}, 80, 24)}
	app.active = 0
	app.SetSnippets(config.Snippets{
		Global: []config.Snippet{{Name: "uptime", Send: "uptime"}},
		ByHost: map[string][]config.Snippet{
			"prod-router": {{Name: "show ver", Send: "show version"}},
		},
	})

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	if app.mode != modeSnippets {
		t.Fatalf("mode = %v, want modeSnippets", app.mode)
	}
	if app.snippets == nil || len(app.snippets.all) != 2 {
		t.Fatalf("snippets not built for host: %+v", app.snippets)
	}
	if app.prevMode != modeSession {
		t.Fatalf("prevMode = %v", app.prevMode)
	}
}

func TestAppPrefixEWithoutSessionSurfacesHint(t *testing.T) {
	app := newTestApp()
	app.mode = modePicker
	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if app.mode != modePicker {
		t.Fatalf("mode = %v, want picker", app.mode)
	}
	if app.statusMsg != "open a session first" {
		t.Fatalf("statusMsg = %q", app.statusMsg)
	}
}

func TestAppSnippetChosenSendsToSession(t *testing.T) {
	app := newTestApp()
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	app.tabs = []*sessionTab{tab}
	app.active = 0
	app.mode = modeSnippets
	app.prevMode = modeSession
	app.snippets = newSnippetsModel([]config.SnippetEntry{
		{Name: "x", Send: "echo hi", Group: "Global"},
	})

	// No attached session — sendInput is a no-op but mode should restore.
	app.update(snippetChosenMsg{send: "echo hi"})
	if app.mode != modeSession {
		t.Fatalf("mode = %v after send", app.mode)
	}
}
