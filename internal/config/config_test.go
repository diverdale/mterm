package config

import "testing"

func TestMergeOverlayAndNewHost(t *testing.T) {
	sshPath := writeTemp(t, "config", `
Host prod-web
    HostName 10.0.1.4
    User deploy
`)
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - name: prod-web
    group: production
    tags: [web, critical]
  - name: lab-box
    hostName: 10.9.0.12
    user: dale
    port: 22
    group: lab
    tags: [scratch]
    forwards:
      - type: local
        bindPort: 8080
        dialAddr: 127.0.0.1
        dialPort: 80
`)

	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatalf("loadAndMerge: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	byName := map[string]Host{}
	for _, h := range res.Hosts {
		byName[h.Name] = h
	}

	web, ok := byName["prod-web"]
	if !ok {
		t.Fatal("prod-web missing")
	}
	if web.HostName != "10.0.1.4" || web.User != "deploy" {
		t.Fatalf("overlay must not clobber ssh_config base: %+v", web)
	}
	if web.Group != "production" || len(web.Tags) != 2 {
		t.Fatalf("overlay group/tags not applied: %+v", web)
	}
	if web.Source != SourceSSHConfig {
		t.Fatalf("overlaid host keeps ssh_config source, got %v", web.Source)
	}

	lab, ok := byName["lab-box"]
	if !ok {
		t.Fatal("lab-box missing")
	}
	if lab.Source != SourceMterm || lab.HostName != "10.9.0.12" {
		t.Fatalf("mterm-only host wrong: %+v", lab)
	}
	if len(lab.Forwards) != 1 || lab.Forwards[0].BindPort != 8080 {
		t.Fatalf("mterm forwards not parsed: %+v", lab.Forwards)
	}
}

func TestMergeBadYAMLIsWarningNotFatal(t *testing.T) {
	sshPath := writeTemp(t, "config", "Host a\n  HostName 1.2.3.4\n")
	mtermPath := writeTemp(t, "hosts.yaml", "hosts: [ this is not valid")

	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatalf("bad YAML must not be fatal, got %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("want a warning for bad YAML")
	}
	if len(res.Hosts) != 1 {
		t.Fatalf("ssh_config host must still load, got %d", len(res.Hosts))
	}
}

func TestMergeSortedByGroupThenName(t *testing.T) {
	sshPath := writeTemp(t, "config", "")
	mtermPath := writeTemp(t, "hosts.yaml", `
hosts:
  - {name: zeta, hostName: z, group: alpha}
  - {name: alpha, hostName: a, group: beta}
  - {name: beta, hostName: b, group: alpha}
`)
	res, err := loadAndMerge(sshPath, mtermPath)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	for _, h := range res.Hosts {
		order = append(order, h.Name)
	}
	want := []string{"beta", "zeta", "alpha"} // group alpha first, then name
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("sort order = %v, want %v", order, want)
		}
	}
}
