package avd

import (
	"reflect"
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

func TestBuildRemoteArgvWithoutExtraEnv(t *testing.T) {
	got := buildRemoteArgv(nil, "adb", "-s", "emulator-5580", "emu", "kill")
	want := []string{"adb", "-s", "emulator-5580", "emu", "kill"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRemoteArgv() = %#v, want %#v", got, want)
	}
}

func TestBuildRemoteArgvWithExtraEnv(t *testing.T) {
	got := buildRemoteArgv([]string{"QEMU_FILE_LOCKING=off", "ADB_VENDOR_KEYS=/dev/null"}, "emulator", "-avd", "demo")
	want := []string{"env", "QEMU_FILE_LOCKING=off", "ADB_VENDOR_KEYS=/dev/null", "emulator", "-avd", "demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildRemoteArgv() = %#v, want %#v", got, want)
	}
}
