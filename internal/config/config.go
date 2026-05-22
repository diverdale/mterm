package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Result is the outcome of loading all config sources.
type Result struct {
	Hosts    []Host
	Warnings []string // non-fatal problems (parse errors, etc.)
}

// Load reads ~/.ssh/config and ~/.config/mterm/hosts.yaml and merges them.
func Load() (*Result, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshPath := filepath.Join(home, ".ssh", "config")
	mtermPath := filepath.Join(home, ".config", "mterm", "hosts.yaml")
	return loadAndMerge(sshPath, mtermPath)
}

// loadAndMerge is the testable core of Load.
func loadAndMerge(sshPath, mtermPath string) (*Result, error) {
	res := &Result{}

	sshHosts, err := loadSSHConfig(sshPath)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("ssh_config: %v", err))
	}

	mf, err := loadMtermFile(mtermPath)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("mterm config: %v", err))
	}

	byName := map[string]*Host{}
	order := []string{}
	for i := range sshHosts {
		h := sshHosts[i]
		byName[h.Name] = &h
		order = append(order, h.Name)
	}

	if mf != nil {
		for _, mh := range mf.Hosts {
			if mh.Name == "" {
				res.Warnings = append(res.Warnings, "mterm config: host with empty name skipped")
				continue
			}
			if existing, ok := byName[mh.Name]; ok {
				applyOverlay(existing, mh)
				continue
			}
			h := hostFromMterm(mh)
			byName[h.Name] = &h
			order = append(order, h.Name)
		}
	}

	for _, name := range order {
		res.Hosts = append(res.Hosts, *byName[name])
	}
	sort.SliceStable(res.Hosts, func(i, j int) bool {
		a, b := res.Hosts[i], res.Hosts[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Name < b.Name
	})
	return res, nil
}

// applyOverlay attaches mterm metadata to an existing ssh_config host.
// mterm-provided non-zero values win; the ssh_config source is preserved.
func applyOverlay(h *Host, mh mtermHost) {
	h.Group = mh.Group
	if len(mh.Tags) > 0 {
		h.Tags = mh.Tags
	}
	if mh.HostName != "" {
		h.HostName = mh.HostName
	}
	if mh.User != "" {
		h.User = mh.User
	}
	if mh.Port != 0 {
		h.Port = mh.Port
	}
	for _, mf := range mh.Forwards {
		h.Forwards = append(h.Forwards, mf.toForward())
	}
}

func hostFromMterm(mh mtermHost) Host {
	port := mh.Port
	if port == 0 {
		port = 22
	}
	hostName := mh.HostName
	if hostName == "" {
		hostName = mh.Name
	}
	var forwards []Forward
	for _, mf := range mh.Forwards {
		forwards = append(forwards, mf.toForward())
	}
	return Host{
		Name:     mh.Name,
		HostName: hostName,
		User:     mh.User,
		Port:     port,
		Group:    mh.Group,
		Tags:     mh.Tags,
		Source:   SourceMterm,
		Forwards: forwards,
	}
}
