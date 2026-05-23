package config

import (
	"bytes"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type mtermFile struct {
	Hosts []mtermHost `yaml:"hosts"`
}

// mtermHost is the on-disk yaml form. All tags are lowercase to match yaml
// convention. Strict decoding (KnownFields true) surfaces unknown keys as
// warnings so typos don't silently drop fields.
//
// "name", "hostname", and "host" are accepted as synonyms for the friendly
// label (canonicalNameAlias folds them into Name; conflicts warn). "address"
// is the dial target (IP or DNS-resolvable hostname); empty falls back to
// Name via DNS.
type mtermHost struct {
	Name     string `yaml:"name"`
	Hostname string `yaml:"hostname"` // alias for Name
	Host     string `yaml:"host"`     // alias for Name
	Address  string `yaml:"address"`  // dial target; empty = DNS-resolve Name

	User        string         `yaml:"user"`
	Port        int            `yaml:"port"`
	Group       string         `yaml:"group"`
	Tags        []string       `yaml:"tags"`
	Forwards    []mtermForward `yaml:"forwards"`
	BorderColor string         `yaml:"bordercolor"`
	Log         *bool          `yaml:"log"` // nil = default-on; false = opt-out
}

// canonicalNameAlias folds name/hostname/host into the single Name field.
// Returns a warning string when more than one is set with conflicting values
// (the higher-priority one wins: name > hostname > host).
func (h *mtermHost) canonicalNameAlias() string {
	candidates := []struct{ key, val string }{
		{"name", h.Name},
		{"hostname", h.Hostname},
		{"host", h.Host},
	}
	var winner, winnerKey string
	var conflicts []string
	for _, c := range candidates {
		if c.val == "" {
			continue
		}
		if winner == "" {
			winner = c.val
			winnerKey = c.key
			continue
		}
		if c.val != winner {
			conflicts = append(conflicts, c.key+"="+c.val)
		}
	}
	h.Name = winner
	h.Hostname = ""
	h.Host = ""
	if len(conflicts) == 0 {
		return ""
	}
	return "multiple name aliases set (" + winnerKey + "=" + winner + " wins, ignoring " + strings.Join(conflicts, ", ") + ")"
}

type mtermForward struct {
	Type     string `yaml:"type"` // "local" | "remote"
	BindAddr string `yaml:"bindaddr"`
	BindPort int    `yaml:"bindport"`
	DialAddr string `yaml:"dialaddr"`
	DialPort int    `yaml:"dialport"`
}

// loadMtermFile parses the mterm YAML file. A missing file returns (nil, nil).
// Decoding is strict: an unknown key (typo, wrong casing, obsolete field)
// returns the decoder error so the caller can surface it as a warning instead
// of silently producing a host with missing fields.
func loadMtermFile(path string) (*mtermFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var mf mtermFile
	if err := dec.Decode(&mf); err != nil {
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
