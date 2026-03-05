package sshclient

import "testing"

func TestSplitTarget(t *testing.T) {
	host, port, user, err := splitTarget("alice@example.com:2222")
	if err != nil {
		t.Fatalf("splitTarget error: %v", err)
	}
	if host != "example.com" || port != 2222 || user != "alice" {
		t.Fatalf("unexpected split: host=%q port=%d user=%q", host, port, user)
	}
}

func TestParseArgs(t *testing.T) {
	got, err := parseArgs([]string{
		"-p", "2200",
		"-l", "bob",
		"-i", "~/.ssh/id_ed25519",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=~/.ssh/kh",
	})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if got.Port != 2200 || got.User != "bob" {
		t.Fatalf("unexpected basics: %#v", got)
	}
	if len(got.IdentityFiles) != 1 {
		t.Fatalf("missing identity files: %#v", got.IdentityFiles)
	}
	if !got.InsecureIgnoreHost {
		t.Fatalf("StrictHostKeyChecking=no should set insecure mode")
	}
	if len(got.KnownHostsFiles) != 1 {
		t.Fatalf("missing known_hosts files: %#v", got.KnownHostsFiles)
	}
}
