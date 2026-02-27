package avd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultGoldenDirUsesEnvOverride(t *testing.T) {
	old := os.Getenv("AVDCTL_GOLDEN_DIR")
	t.Cleanup(func() {
		_ = os.Setenv("AVDCTL_GOLDEN_DIR", old)
	})

	want := filepath.Join(t.TempDir(), "golden")
	if err := os.Setenv("AVDCTL_GOLDEN_DIR", want); err != nil {
		t.Fatalf("set env: %v", err)
	}

	if got := DefaultGoldenDir(); got != want {
		t.Fatalf("DefaultGoldenDir() = %q, want %q", got, want)
	}
}

func TestDetectSSHSettings(t *testing.T) {
	oldTarget := os.Getenv("AVDCTL_SSH_TARGET")
	oldBin := os.Getenv("AVDCTL_SSH_BIN")
	oldArgs := os.Getenv("AVDCTL_SSH_ARGS")
	t.Cleanup(func() {
		_ = os.Setenv("AVDCTL_SSH_TARGET", oldTarget)
		_ = os.Setenv("AVDCTL_SSH_BIN", oldBin)
		_ = os.Setenv("AVDCTL_SSH_ARGS", oldArgs)
	})

	if err := os.Setenv("AVDCTL_SSH_TARGET", "ci@remote-host"); err != nil {
		t.Fatalf("set ssh target: %v", err)
	}
	if err := os.Setenv("AVDCTL_SSH_BIN", "/usr/bin/ssh"); err != nil {
		t.Fatalf("set ssh bin: %v", err)
	}
	if err := os.Setenv("AVDCTL_SSH_ARGS", "-p 2222 -o StrictHostKeyChecking=no"); err != nil {
		t.Fatalf("set ssh args: %v", err)
	}

	env := Detect()
	if env.SSHTarget != "ci@remote-host" {
		t.Fatalf("SSHTarget = %q", env.SSHTarget)
	}
	if env.SSHBin != "/usr/bin/ssh" {
		t.Fatalf("SSHBin = %q", env.SSHBin)
	}
	if len(env.SSHArgs) != 4 {
		t.Fatalf("unexpected SSHArgs length: %#v", env.SSHArgs)
	}
	if env.SSHArgs[0] != "-p" || env.SSHArgs[1] != "2222" {
		t.Fatalf("unexpected SSHArgs prefix: %#v", env.SSHArgs)
	}
}
