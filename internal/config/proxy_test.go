package config

import (
	"reflect"
	"testing"
)

func TestResolveProxyJumpEmptyReturnsNil(t *testing.T) {
	h, err := ResolveProxyJump("", nil)
	if err != nil {
		t.Fatalf("empty value should not error: %v", err)
	}
	if h != nil {
		t.Fatalf("empty value should return nil host, got %+v", h)
	}
}

func TestResolveProxyJumpAliasLookup(t *testing.T) {
	hosts := []Host{
		{Name: "other", HostName: "1.2.3.4", Port: 22},
		{Name: "jump-host", HostName: "jump.example.com", Port: 2222, User: "alice", IdentityFile: "~/.ssh/jump-key"},
	}
	got, err := ResolveProxyJump("jump-host", hosts)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("alias lookup should return a host")
	}
	if got.HostName != "jump.example.com" || got.Port != 2222 || got.User != "alice" || got.IdentityFile != "~/.ssh/jump-key" {
		t.Fatalf("alias lookup didn't copy fields: %+v", got)
	}
}

func TestResolveProxyJumpAliasReturnsCopy(t *testing.T) {
	hosts := []Host{
		{Name: "jump", HostName: "jump.example.com", Port: 22},
	}
	got, _ := ResolveProxyJump("jump", hosts)
	got.HostName = "tampered"
	if hosts[0].HostName == "tampered" {
		t.Fatal("ResolveProxyJump must return a copy, not a pointer into the slice")
	}
}

func TestResolveProxyJumpLiteralHostnameOnly(t *testing.T) {
	got, err := ResolveProxyJump("bastion.example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := &Host{
		Name:     "bastion.example.com",
		HostName: "bastion.example.com",
		Port:     22,
		Source:   SourceMterm,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestResolveProxyJumpLiteralWithUser(t *testing.T) {
	got, err := ResolveProxyJump("alice@bastion.example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.User != "alice" || got.HostName != "bastion.example.com" || got.Port != 22 {
		t.Fatalf("got %+v, want user=alice host=bastion.example.com port=22", got)
	}
}

func TestResolveProxyJumpLiteralWithUserAndPort(t *testing.T) {
	got, err := ResolveProxyJump("alice@bastion.example.com:2222", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.User != "alice" || got.HostName != "bastion.example.com" || got.Port != 2222 {
		t.Fatalf("got %+v, want user=alice host=bastion.example.com port=2222", got)
	}
}

func TestResolveProxyJumpLiteralPortOnly(t *testing.T) {
	got, err := ResolveProxyJump("bastion.example.com:2222", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.User != "" || got.Port != 2222 {
		t.Fatalf("got %+v, want user=\"\" port=2222", got)
	}
}

func TestResolveProxyJumpLiteralErrors(t *testing.T) {
	cases := []struct {
		in   string
		name string
	}{
		{"@bastion.example.com", "empty user"},
		{"bastion.example.com:bad", "non-numeric port"},
		{"bastion.example.com:0", "port zero"},
		{"bastion.example.com:99999", "port out of range"},
		{":2222", "empty hostname"},
	}
	for _, tc := range cases {
		_, err := ResolveProxyJump(tc.in, nil)
		if err == nil {
			t.Errorf("%s (%q): expected error, got nil", tc.name, tc.in)
		}
	}
}

func TestResolveProxyJumpAliasWinsOverLiteralFormMatch(t *testing.T) {
	// If a host happens to be named "host:22" (unusual but legal), it
	// should still be resolved as an alias rather than being parsed as
	// a hostname-with-port literal.
	hosts := []Host{
		{Name: "host:22", HostName: "real.example.com", Port: 4444},
	}
	got, err := ResolveProxyJump("host:22", hosts)
	if err != nil {
		t.Fatal(err)
	}
	if got.HostName != "real.example.com" || got.Port != 4444 {
		t.Fatalf("alias-with-colon-name should match registry, got %+v", got)
	}
}
