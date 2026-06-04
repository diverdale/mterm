package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetFrameStyleSwitchesGlyphs(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })

	if !SetFrameStyle("double") {
		t.Fatal("SetFrameStyle(double) should succeed")
	}
	if frame.TopLeft != "╔" {
		t.Fatalf("after double, top-left = %q, want ╔", frame.TopLeft)
	}

	if !SetFrameStyle("ascii") {
		t.Fatal("SetFrameStyle(ascii) should succeed")
	}
	if frame.TopLeft != "+" {
		t.Fatalf("after ascii, top-left = %q, want +", frame.TopLeft)
	}
}

func TestSetFrameStyleUnknownIsNoOp(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })
	SetFrameStyle("rounded")
	if SetFrameStyle("does-not-exist") {
		t.Fatal("unknown style should return false")
	}
	if frame.TopLeft != "╭" {
		t.Fatalf("after failed switch, top-left changed: %q", frame.TopLeft)
	}
}

func TestSetFrameStyleCaseInsensitive(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })
	if !SetFrameStyle("DOUBLE") {
		t.Fatal("uppercase name should resolve")
	}
	if frame.TopLeft != "╔" {
		t.Fatalf("DOUBLE didn't apply: %q", frame.TopLeft)
	}
}

func TestLoadSettingsMissingIsNotError(t *testing.T) {
	if err := LoadSettings(filepath.Join(t.TempDir(), "missing.yaml")); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
}

func TestLoadSettingsAppliesFrame(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })

	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("frame: thick\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadSettings(path); err != nil {
		t.Fatal(err)
	}
	if frame.TopLeft != "┏" {
		t.Fatalf("after settings.yaml with frame:thick, top-left = %q", frame.TopLeft)
	}
}

func TestLoadSettingsUnknownFrameNameErrors(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })

	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("frame: not-a-real-style\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := LoadSettings(path)
	if err == nil {
		t.Fatal("unknown frame name should return an error")
	}
}

func TestToggleMinimalFrameRoundTrips(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })

	SetFrameStyle("rounded")
	if got := ToggleMinimalFrame(); got != "minimal" {
		t.Fatalf("first toggle from rounded → %q, want minimal", got)
	}
	if frame.TopLeft != " " {
		t.Fatalf("expected blanked frame after toggle, top-left = %q", frame.TopLeft)
	}
	if got := ToggleMinimalFrame(); got != "rounded" {
		t.Fatalf("second toggle → %q, want rounded (restore)", got)
	}
	if frame.TopLeft != "╭" {
		t.Fatalf("expected rounded restored, top-left = %q", frame.TopLeft)
	}
}

func TestToggleMinimalFrameRemembersNonMinimalStyle(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })

	SetFrameStyle("thick")
	ToggleMinimalFrame() // thick → minimal
	if frame.TopLeft != " " {
		t.Fatalf("expected minimal, top-left = %q", frame.TopLeft)
	}
	ToggleMinimalFrame() // minimal → thick (not rounded)
	if frame.TopLeft != "┏" {
		t.Fatalf("expected thick restored, top-left = %q", frame.TopLeft)
	}
}

func TestToggleMinimalFrameStartingFromMinimalFallsBackToRounded(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })

	// Simulate a user whose settings.yaml has `frame: minimal` — first
	// press of ^B m should still produce visible chrome.
	SetFrameStyle("minimal")
	if got := ToggleMinimalFrame(); got != "rounded" {
		t.Fatalf("from minimal-as-default → %q, want rounded fallback", got)
	}
}

func TestCurrentFrameNameTracksSetFrameStyle(t *testing.T) {
	t.Cleanup(func() { SetFrameStyle("rounded") })

	SetFrameStyle("double")
	if got := CurrentFrameName(); got != "double" {
		t.Fatalf("CurrentFrameName = %q, want double", got)
	}
	SetFrameStyle("ASCII")
	if got := CurrentFrameName(); got != "ascii" {
		t.Fatalf("CurrentFrameName after ASCII = %q, want ascii (lowercased)", got)
	}
}
