package avd

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCommandUsesLocalBinaryWithoutSSH(t *testing.T) {
	cmd := commandWithEnv(nil, "adb", "devices")
	if got := cmd.Args[0]; got != "adb" {
		t.Fatalf("args[0] = %q, want adb", got)
	}
	if !reflect.DeepEqual(cmd.Args, []string{"adb", "devices"}) {
		t.Fatalf("args = %#v", cmd.Args)
	}
}

func TestCommandWithEnvSetsExtraEnv(t *testing.T) {
	cmd := commandWithEnv([]string{"QEMU_FILE_LOCKING=off"}, "adb", "devices")
	if got := cmd.Args[0]; got != "adb" {
		t.Fatalf("args[0] = %q, want adb", got)
	}
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, "QEMU_FILE_LOCKING=off") {
		t.Fatalf("extra env not propagated: %v", cmd.Env)
	}
}

func TestCommandContextWithEnvSetsExtraEnv(t *testing.T) {
	cmd := commandContextWithEnv(context.Background(), []string{"ADB_VENDOR_KEYS=/dev/null"}, "adb", "devices")
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, "ADB_VENDOR_KEYS=/dev/null") {
		t.Fatalf("extra env not propagated: %v", cmd.Env)
	}
}
