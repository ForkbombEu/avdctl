package avd

import (
	"context"
	"reflect"
	"testing"
)

func TestCommandUsesLocalBinaryWithoutSSH(t *testing.T) {
	cmd := command(Env{}, "adb", "devices")
	if got := cmd.Path; got != "adb" {
		t.Fatalf("path = %q, want adb", got)
	}
	if !reflect.DeepEqual(cmd.Args, []string{"adb", "devices"}) {
		t.Fatalf("args = %#v", cmd.Args)
	}
}

func TestCommandWrapsWithSSH(t *testing.T) {
	env := Env{
		SSHTarget: "android@runner",
		SSHBin:    "ssh",
		SSHArgs:   []string{"-p", "2222"},
	}
	cmd := command(env, "adb", "-s", "emulator-5580", "emu", "kill")
	want := []string{
		"ssh",
		"-p",
		"2222",
		"android@runner",
		"sh",
		"-lc",
		"'adb' '-s' 'emulator-5580' 'emu' 'kill'",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("args = %#v, want %#v", cmd.Args, want)
	}
}

func TestCommandContextWrapsWithSSH(t *testing.T) {
	env := Env{
		SSHTarget: "android@runner",
		SSHArgs:   []string{"-o", "BatchMode=yes"},
	}
	cmd := commandContext(context.Background(), env, "adb", "devices")
	want := []string{
		"ssh",
		"-o",
		"BatchMode=yes",
		"android@runner",
		"sh",
		"-lc",
		"'adb' 'devices'",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("args = %#v, want %#v", cmd.Args, want)
	}
}
