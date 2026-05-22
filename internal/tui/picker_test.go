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
