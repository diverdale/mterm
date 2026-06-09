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

// Load reads ~/.ssh/config and the app hosts.yaml and merges them. The
// nil colors map disables named-color resolution for bordercolor — use
// LoadWithColors to also accept "limegreen" / "prodred" style names.
func Load() (*Result, error) {
	return LoadWithColors(nil)
}

// LoadWithColors is Load + a name→hex map for resolving non-hex
// bordercolor values. Pass internal/colors.Open(...)'s output.
func LoadWithColors(colors map[string]string) (*Result, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshPath := filepath.Join(home, ".ssh", "config")
	hostsPath, err := HostsFile()
	if err != nil {
		return nil, err
	}
	return loadAndMerge(sshPath, hostsPath, colors)
}

// loadAndMerge is the testable core of Load. The colors map (nil OK)
// gives bordercolor named-color resolution. Pipeline:
//  1. Read ssh_config (file order) and hosts.yaml (decoder-strict).
//  2. Collect all yaml-declared hosts in declaration order with monotonic
//     intentOrder — nested groups walked depth-first, then flat hosts.
//  3. Walk ssh_config in its file order, applying any yaml overlay matched
//     by name; ssh_config-only hosts get an intentOrder continuing from
//     where yaml left off (so they trail in their group).
//  4. Append yaml-only hosts (those without an ssh_config base) last in
//     the byName/result population, with their already-set intentOrder.
//  5. Validate per-host bordercolor (warn + clear on invalid).
//  6. Sort hierarchically: groups appear in the order their first host
//     appeared; within a group, hosts in intentOrder; sub-groups inherit
//     their parent's contiguity.
func loadAndMerge(sshPath, mtermPath string, colors map[string]string) (*Result, error) {
	res := &Result{}

	sshHosts, err := loadSSHConfig(sshPath)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("ssh_config: %v", err))
	}

	mf, err := loadMtermFile(mtermPath)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("mterm config: %v", err))
	}

	// Plaintext passwords live in this file; if it's group- or world-readable,
	// warn loudly. Skip on missing file (no secrets to leak) and on non-regular
	// special files (sockets, devices) — we only care about the regular config.
	if mf != nil && hasPlaintextSecrets(mf) {
		if perm, ok := readableByOthers(mtermPath); ok {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("mterm config: %s contains plaintext password(s) but is mode %#o "+
					"(group/world readable) — chmod 600", mtermPath, perm))
		}
	}

	// 1. Gather yaml-declared hosts in declaration order.
	var yamlHosts []mtermHost
	counter := 0
	if mf != nil {
		collectNestedHosts(mf.Groups, "", &counter, &yamlHosts)
		for i := range mf.Hosts {
			mh := mf.Hosts[i]
			mh.intentOrder = counter
			counter++
			yamlHosts = append(yamlHosts, mh)
		}
	}

	// Resolve name aliases and skip empty-name entries up front.
	resolved := yamlHosts[:0]
	for _, mh := range yamlHosts {
		if warn := mh.canonicalNameAlias(); warn != "" {
			res.Warnings = append(res.Warnings, "mterm config: "+warn)
		}
		if mh.Name == "" {
			res.Warnings = append(res.Warnings, "mterm config: host with empty name skipped")
			continue
		}
		resolved = append(resolved, mh)
	}
	yamlHosts = resolved

	yamlByName := map[string]*mtermHost{}
	for i := range yamlHosts {
		yamlByName[yamlHosts[i].Name] = &yamlHosts[i]
	}

	// 2. Walk ssh_config; merge overlay or assign trailing intentOrder.
	var out []Host
	seenYaml := map[string]bool{}
	for _, sh := range sshHosts {
		if mh, ok := yamlByName[sh.Name]; ok {
			applyOverlay(&sh, *mh)
			sh.intentOrder = mh.intentOrder
			seenYaml[mh.Name] = true
		} else {
			sh.intentOrder = counter
			counter++
		}
		out = append(out, sh)
	}

	// 3. Append yaml-only hosts (no ssh_config base).
	for _, mh := range yamlHosts {
		if seenYaml[mh.Name] {
			continue
		}
		out = append(out, hostFromMterm(mh))
	}

	// 4. Validate bordercolor; collect into result.
	for _, h := range out {
		if h.BorderColor != "" {
			parsed, err := parseBorderColor(h.BorderColor, colors)
			if err != nil {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("mterm config: host %q bordercolor %q: %v", h.Name, h.BorderColor, err))
				h.BorderColor = ""
			} else {
				h.BorderColor = parsed
			}
		}
		res.Hosts = append(res.Hosts, h)
	}

	// 5. Hierarchical sort: group prefixes compared by min intentOrder of
	// any host beneath that prefix; within-group hosts by intentOrder.
	prefixMin := map[string]int{}
	for _, h := range res.Hosts {
		segs := GroupSegments(h.Group)
		for k := 1; k <= len(segs); k++ {
			prefix := segs[k-1]
			if k > 1 {
				prefix = ""
				for j := 0; j < k; j++ {
					if j > 0 {
						prefix += "/"
					}
					prefix += segs[j]
				}
			}
			if cur, ok := prefixMin[prefix]; !ok || h.intentOrder < cur {
				prefixMin[prefix] = h.intentOrder
			}
		}
	}
	joinPrefix := func(segs []string, k int) string {
		s := ""
		for i := 0; i < k; i++ {
			if i > 0 {
				s += "/"
			}
			s += segs[i]
		}
		return s
	}
	sort.SliceStable(res.Hosts, func(i, j int) bool {
		a, b := res.Hosts[i], res.Hosts[j]
		asegs := GroupSegments(a.Group)
		bsegs := GroupSegments(b.Group)
		// Ungrouped hosts always trail grouped ones — user-organized groups
		// take precedence over the ssh_config-style flat bucket.
		if len(asegs) == 0 && len(bsegs) > 0 {
			return false
		}
		if len(asegs) > 0 && len(bsegs) == 0 {
			return true
		}
		n := min(len(asegs), len(bsegs))
		for k := 0; k < n; k++ {
			if asegs[k] != bsegs[k] {
				return prefixMin[joinPrefix(asegs, k+1)] < prefixMin[joinPrefix(bsegs, k+1)]
			}
		}
		// One path is a prefix of the other → shorter (more general) first.
		if len(asegs) != len(bsegs) {
			return len(asegs) < len(bsegs)
		}
		return a.intentOrder < b.intentOrder
	})

	return res, nil
}

// applyOverlay attaches mterm metadata to an existing ssh_config host.
// mterm-provided non-zero values win; the ssh_config source is preserved.
func applyOverlay(h *Host, mh mtermHost) {
	if mh.Group != "" {
		h.Group = mh.Group
	}
	if len(mh.Tags) > 0 {
		h.Tags = mh.Tags
	}
	if mh.Address != "" {
		h.HostName = mh.Address
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
	if mh.BorderColor != "" {
		h.BorderColor = mh.BorderColor
	}
	if mh.ProxyJump != "" {
		h.ProxyJump = mh.ProxyJump
	}
	if mh.Password != "" {
		h.Password = mh.Password
	}
	if mh.PasswordCommand != "" {
		h.PasswordCommand = mh.PasswordCommand
	}
	if mh.IdentityFile != "" {
		h.IdentityFile = mh.IdentityFile
	}
	if len(mh.OnConnect) > 0 {
		h.OnConnect = mh.OnConnect
	}
	if mh.Log != nil {
		h.Log = mh.Log
	}
}

func hostFromMterm(mh mtermHost) Host {
	port := mh.Port
	if port == 0 {
		port = 22
	}
	hostName := mh.Address
	if hostName == "" {
		hostName = mh.Name
	}
	var forwards []Forward
	for _, mf := range mh.Forwards {
		forwards = append(forwards, mf.toForward())
	}
	return Host{
		Name:         mh.Name,
		HostName:     hostName,
		User:         mh.User,
		Port:         port,
		Group:        mh.Group,
		Tags:         mh.Tags,
		Source:       SourceMterm,
		Forwards:     forwards,
		BorderColor:  mh.BorderColor,
		IdentityFile:    mh.IdentityFile,
		OnConnect:       mh.OnConnect,
		ProxyJump:       mh.ProxyJump,
		Password:        mh.Password,
		PasswordCommand: mh.PasswordCommand,
		Log:             mh.Log,
		intentOrder:     mh.intentOrder,
	}
}

// hasPlaintextSecrets reports whether the loaded mterm config carries any
// literal `password:` entry. `password_command:` is not a secret on disk
// — the command runs at connect time and the credential never touches the
// yaml.
func hasPlaintextSecrets(mf *mtermFile) bool {
	for _, h := range mf.Hosts {
		if h.Password != "" {
			return true
		}
	}
	return groupsHavePlaintextSecrets(mf.Groups)
}

func groupsHavePlaintextSecrets(groups []mtermGroup) bool {
	for _, g := range groups {
		for _, h := range g.Hosts {
			if h.Password != "" {
				return true
			}
		}
		if groupsHavePlaintextSecrets(g.Groups) {
			return true
		}
	}
	return false
}

// readableByOthers reports whether path is a regular file readable by
// group or world. Returns (perm-bits, true) when a warning is warranted,
// or (0, false) otherwise (file missing, special file, or 0600-style
// owner-only).
func readableByOthers(path string) (os.FileMode, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, false
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		return perm, true
	}
	return 0, false
}
