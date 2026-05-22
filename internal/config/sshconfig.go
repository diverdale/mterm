package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// loadSSHConfig parses an OpenSSH config file into hosts. A missing file is not
// an error: it returns (nil, nil).
func loadSSHConfig(path string) ([]Host, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, err
	}

	var hosts []Host
	seen := map[string]bool{}
	for _, block := range cfg.Hosts {
		for _, pat := range block.Patterns {
			alias := pat.String()
			if alias == "" || strings.ContainsAny(alias, "*?!") {
				continue
			}
			if seen[alias] {
				continue
			}
			seen[alias] = true
			hosts = append(hosts, hostFromConfig(cfg, alias))
		}
	}
	return hosts, nil
}

func hostFromConfig(cfg *ssh_config.Config, alias string) Host {
	get := func(key string) string {
		v, _ := cfg.Get(alias, key)
		return v
	}
	getAll := func(key string) []string {
		v, _ := cfg.GetAll(alias, key)
		return v
	}

	port := 22
	if p, err := strconv.Atoi(get("Port")); err == nil && p > 0 {
		port = p
	}
	hostName := get("HostName")
	if hostName == "" {
		hostName = alias
	}

	var forwards []Forward
	for _, raw := range getAll("LocalForward") {
		if f, ok := parseForward(raw, ForwardLocal); ok {
			forwards = append(forwards, f)
		}
	}
	for _, raw := range getAll("RemoteForward") {
		if f, ok := parseForward(raw, ForwardRemote); ok {
			forwards = append(forwards, f)
		}
	}

	return Host{
		Name:         alias,
		HostName:     hostName,
		User:         get("User"),
		Port:         port,
		Source:       SourceSSHConfig,
		IdentityFile: get("IdentityFile"),
		ProxyJump:    get("ProxyJump"),
		Forwards:     forwards,
	}
}

// parseForward parses an OpenSSH "LocalForward"/"RemoteForward" value of the
// form "[bind:]port host:hostport".
func parseForward(raw string, typ ForwardType) (Forward, bool) {
	fields := strings.Fields(raw)
	if len(fields) != 2 {
		return Forward{}, false
	}
	bindAddr, bindPort, ok := splitHostPort(fields[0])
	if !ok {
		return Forward{}, false
	}
	dialAddr, dialPort, ok := splitHostPort(fields[1])
	if !ok {
		return Forward{}, false
	}
	return Forward{
		Type:     typ,
		BindAddr: bindAddr,
		BindPort: bindPort,
		DialAddr: dialAddr,
		DialPort: dialPort,
	}, true
}

// splitHostPort accepts "port" or "host:port" and returns (host, port).
func splitHostPort(s string) (host string, port int, ok bool) {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		host = s[:i]
		s = s[i+1:]
	}
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 {
		return "", 0, false
	}
	return host, p, true
}
