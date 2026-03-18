package redroidmanager

import (
	"context"
	"testing"
)

func TestNewWithEnvMapsEnvironment(t *testing.T) {
	mgr := NewWithEnv(Environment{
		DockerHost: "unix:///var/run/docker.sock",
		ADBBin:     "adb-custom",
		TarBin:     "tar-custom",
		SudoBin:    "sudo-custom",
		Sudo:       true,
		SudoPass:   "secret",
		SSHTarget:  "user@host",
		SSHArgs:    []string{"-i", "key"},
		Context:    context.Background(),
	})

	if mgr.env.ADBBin != "adb-custom" {
		t.Fatalf("ADBBin = %q", mgr.env.ADBBin)
	}
	if mgr.env.TarBin != "tar-custom" {
		t.Fatalf("TarBin = %q", mgr.env.TarBin)
	}
	if mgr.env.SSHTarget != "user@host" {
		t.Fatalf("SSHTarget = %q", mgr.env.SSHTarget)
	}
	if len(mgr.env.SSHArgs) != 2 {
		t.Fatalf("SSHArgs = %#v", mgr.env.SSHArgs)
	}
}
