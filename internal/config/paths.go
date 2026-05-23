package config

import (
	"os"
	"path/filepath"

	"mterm/internal/appmeta"
)

// ConfigDir returns the app's config root, e.g. ~/.config/mterm.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", appmeta.DirName), nil
}

// HostsFile returns the path to hosts.yaml under ConfigDir.
func HostsFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hosts.yaml"), nil
}

// LogsDir returns the root directory under which per-host session logs live.
// Per-host subdirs are created on demand by the logger.
func LogsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs"), nil
}

// BootstrapConfigDir creates ConfigDir() if it does not yet exist. Idempotent.
// Called at startup so a fresh install can drop hosts.yaml in place without
// the user having to mkdir first.
func BootstrapConfigDir() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o700)
}
