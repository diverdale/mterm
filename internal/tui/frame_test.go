package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestWindowHasExactGeometry(t *testing.T) {
	body := strings.Repeat("x\n", 18) // 19 lines of body content
	out := renderWindow(windowOpts{
		title:  "prod-web",
		body:   strings.TrimRight(body, "\n"),
		footer: "footer here",
		width:  80,
		height: 24,
	}, sty)
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("window has %d lines, want 24", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 80 {
			t.Fatalf("line %d width = %d, want 80", i, w)
		}
	}
}

func TestWindowShowsTitleAndFooter(t *testing.T) {
	out := renderWindow(windowOpts{
		title:  "prod-web",
		body:   "hello",
		footer: "the-footer",
		width:  60,
		height: 12,
	}, sty)
	if !strings.Contains(out, "prod-web") {
		t.Fatal("window must show the title")
	}
	if !strings.Contains(out, "the-footer") {
		t.Fatal("window must show the footer")
	}
}

func TestWindowWithTabStrip(t *testing.T) {
	out := renderWindow(windowOpts{
		title:    "prod-web",
		tabStrip: "1 prod-web",
		body:     "hello",
		footer:   "f",
		width:    60,
		height:   12,
	}, sty)
	if !strings.Contains(out, "1 prod-web") {
		t.Fatal("window must show the tab strip when provided")
	}
	if got := len(strings.Split(out, "\n")); got != 12 {
		t.Fatalf("window with tab strip has %d lines, want 12", got)
	}
}

func TestWindowMinHeightNoTab(t *testing.T) {
	out := renderWindow(windowOpts{
		title: "t", body: "b", footer: "f", width: 20, height: 4,
	}, sty)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("no-tab window at height 4 produced %d lines, want 4", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 20 {
			t.Fatalf("line %d width = %d, want 20", i, w)
		}
	}
}

func TestWindowShortWithTab(t *testing.T) {
	out := renderWindow(windowOpts{
		title: "t", tabStrip: "tabs", body: "b", footer: "f", width: 20, height: 6,
	}, sty)
	if got := len(strings.Split(out, "\n")); got != 6 {
		t.Fatalf("with-tab window at height 6 produced %d lines, want 6", got)
	}
}
