package config

import (
	"bytes"
	"os"

	"gopkg.in/yaml.v3"
)

// Snippet is one saved command to send to a remote shell.
type Snippet struct {
	Name string `yaml:"name"`
	Send string `yaml:"send"`
}

// Snippets holds global and per-host snippet definitions loaded from
// snippets.yaml.
type Snippets struct {
	Global []Snippet
	ByHost map[string][]Snippet
}

// SnippetEntry is a snippet with its palette group label resolved for a
// particular host context.
type SnippetEntry struct {
	Name  string
	Send  string
	Group string
}

const snippetGroupGlobal = "Global"

// ForHost returns global snippets followed by any defined for hostName.
// Declaration order within each section is preserved.
func (s Snippets) ForHost(hostName string) []SnippetEntry {
	if s.Global == nil && s.ByHost == nil {
		return nil
	}
	var out []SnippetEntry
	for _, sn := range s.Global {
		if sn.Name == "" || sn.Send == "" {
			continue
		}
		out = append(out, SnippetEntry{
			Name:  sn.Name,
			Send:  sn.Send,
			Group: snippetGroupGlobal,
		})
	}
	for _, sn := range s.ByHost[hostName] {
		if sn.Name == "" || sn.Send == "" {
			continue
		}
		out = append(out, SnippetEntry{
			Name:  sn.Name,
			Send:  sn.Send,
			Group: hostName,
		})
	}
	return out
}

type snippetsFile struct {
	Snippets     []Snippet            `yaml:"snippets"`
	HostSnippets map[string][]Snippet `yaml:"host_snippets"`
}

// LoadSnippets reads ~/.config/mterm/snippets.yaml. A missing file returns an
// empty Snippets without error.
func LoadSnippets(path string) (Snippets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snippets{}, nil
		}
		return Snippets{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f snippetsFile
	if err := dec.Decode(&f); err != nil {
		return Snippets{}, err
	}
	return Snippets{
		Global: f.Snippets,
		ByHost: f.HostSnippets,
	}, nil
}
