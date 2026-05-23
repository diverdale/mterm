package config

import (
	"bytes"
	"os"

	"gopkg.in/yaml.v3"
)

type mtermFile struct {
	Hosts []mtermHost `yaml:"hosts"`
}

// All field tags are lowercase to match yaml convention and what users
// naturally write. Strict decoding (KnownFields true) surfaces unknown keys
// loudly so typos like "hostName" or "host_name" don't get silently dropped.
type mtermHost struct {
	Name        string         `yaml:"name"`
	HostName    string         `yaml:"hostname"`
	User        string         `yaml:"user"`
	Port        int            `yaml:"port"`
	Group       string         `yaml:"group"`
	Tags        []string       `yaml:"tags"`
	Forwards    []mtermForward `yaml:"forwards"`
	BorderColor string         `yaml:"bordercolor"`
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
