package config

import (
	"fmt"

	"mterm/internal/appmeta"
)

type Source int

const (
	SourceSSHConfig Source = iota
	SourceMterm
)

func (s Source) String() string {
	if s == SourceMterm {
		return appmeta.Name
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
	IdentityFile    string // path to a private key file; tried before the agent
	ProxyJump       string // jump host: alias resolved against the registry, OR a literal user@host:port
	Password        string // password literal — plaintext, last in the auth chain
	PasswordCommand string // command whose trimmed stdout supplies the password (preferred over Password)
	Forwards     []Forward
	BorderColor  string // hex like "#RRGGBB" or "#RGB"; "" = no override

	// OnConnect is a list of shell commands to send to the remote right
	// after the session connects. Each gets a trailing CR. Empty list =
	// no auto-run.
	OnConnect []string

	// Log is a tri-state opt-in/out for per-session output logging:
	//   nil   → use the default (logging on)
	//   true  → explicitly on
	//   false → explicitly off
	Log *bool

	// intentOrder is the loader-assigned sequence number used to preserve
	// yaml declaration order during sort. Lower values sort first; hosts
	// originating from ssh_config (with no yaml counterpart) get values
	// after all yaml-declared hosts so they trail in the picker.
	intentOrder int
}

// Logging reports whether per-session output should be logged for this host.
// Default is on; only an explicit `log: false` disables it.
func (h Host) Logging() bool {
	if h.Log != nil {
		return *h.Log
	}
	return true
}

// Addr returns the host:port dial string.
func (h Host) Addr() string {
	return fmt.Sprintf("%s:%d", h.HostName, h.Port)
}
