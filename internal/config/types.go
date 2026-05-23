package config

import "fmt"

type Source int

const (
	SourceSSHConfig Source = iota
	SourceMterm
)

func (s Source) String() string {
	if s == SourceMterm {
		return "mterm"
	}
	return "ssh_config"
}

type ForwardType int

const (
	ForwardLocal ForwardType = iota
	ForwardRemote
)

func (t ForwardType) String() string {
	if t == ForwardRemote {
		return "remote"
	}
	return "local"
}

// Forward is a single port-forwarding rule.
type Forward struct {
	Type     ForwardType
	BindAddr string // empty means localhost
	BindPort int
	DialAddr string
	DialPort int
}

// Host is one connectable SSH target after merging all config sources.
type Host struct {
	Name         string
	HostName     string
	User         string
	Port         int
	Group        string
	Tags         []string
	Source       Source
	IdentityFile string // parsed now, used when key-file auth lands
	ProxyJump    string // parsed now, used when jump-host support lands
	Forwards     []Forward
	BorderColor  string // hex like "#RRGGBB" or "#RGB"; "" = no override
}

// Addr returns the host:port dial string.
func (h Host) Addr() string {
	return fmt.Sprintf("%s:%d", h.HostName, h.Port)
}
