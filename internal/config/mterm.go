package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type mtermFile struct {
	Hosts []mtermHost `yaml:"hosts"`
}

type mtermHost struct {
	Name        string         `yaml:"name"`
	HostName    string         `yaml:"hostName"`
	User        string         `yaml:"user"`
	Port        int            `yaml:"port"`
	Group       string         `yaml:"group"`
	Tags        []string       `yaml:"tags"`
	Forwards    []mtermForward `yaml:"forwards"`
	BorderColor string         `yaml:"borderColor"`
}

type mtermForward struct {
	Type     string `yaml:"type"` // "local" | "remote"
	BindAddr string `yaml:"bindAddr"`
	BindPort int    `yaml:"bindPort"`
	DialAddr string `yaml:"dialAddr"`
	DialPort int    `yaml:"dialPort"`
}

// loadMtermFile parses the mterm YAML file. A missing file returns (nil, nil).
func loadMtermFile(path string) (*mtermFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var mf mtermFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
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
