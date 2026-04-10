package avd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	oldArgs := os.Getenv("AVDCTL_SSH_ARGS")
	t.Cleanup(func() {
		_ = os.Setenv("AVDCTL_SSH_TARGET", oldTarget)
		_ = os.Setenv("AVDCTL_SSH_ARGS", oldArgs)
	})

	if err := os.Setenv("AVDCTL_SSH_TARGET", "ci@remote-host"); err != nil {
		t.Fatalf("set ssh target: %v", err)
	}
	if err := os.Setenv("AVDCTL_SSH_ARGS", "-p 2222 -o StrictHostKeyChecking=no"); err != nil {
		t.Fatalf("set ssh args: %v", err)
	}

	env := Detect()
	if env.SSHTarget != "ci@remote-host" {
		t.Fatalf("SSHTarget = %q", env.SSHTarget)
	}
	if len(env.SSHArgs) != 4 {
		t.Fatalf("unexpected SSHArgs length: %#v", env.SSHArgs)
	}
	if env.SSHArgs[0] != "-p" || env.SSHArgs[1] != "2222" {
		t.Fatalf("unexpected SSHArgs prefix: %#v", env.SSHArgs)
	}
}

func TestDetectSetsDefaultEmulatorSerialTimeout(t *testing.T) {
	env := Detect()

	if env.EmulatorSerialTimeout != 4*time.Minute {
		t.Fatalf("EmulatorSerialTimeout = %s, want %s", env.EmulatorSerialTimeout, 4*time.Minute)
	}
}

func TestEnvEmulatorSerialTimeoutUsesDefaultWhenUnset(t *testing.T) {
	var env Env

	if got := env.emulatorSerialTimeout(); got != 4*time.Minute {
		t.Fatalf("emulatorSerialTimeout() = %s, want %s", got, 4*time.Minute)
	}
}

func TestEnvEmulatorSerialTimeoutUsesConfiguredValue(t *testing.T) {
	env := Env{EmulatorSerialTimeout: 90 * time.Second}

	if got := env.emulatorSerialTimeout(); got != 90*time.Second {
		t.Fatalf("emulatorSerialTimeout() = %s, want %s", got, 90*time.Second)
	}
}
