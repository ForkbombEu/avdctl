package main

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(out)
}

func TestMainHelpCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"avdctl", "--help"}
	_ = captureStdout(t, main)
}

func TestVersionCommandFallsBackToDev(t *testing.T) {
	oldArgs := os.Args
	oldVersion := version
	defer func() {
		os.Args = oldArgs
		version = oldVersion
	}()

	version = "   "
	os.Args = []string{"avdctl", "version"}

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, main)
	})
	if strings.TrimSpace(stdout) != "dev" {
		t.Fatalf("expected version fallback to dev, got %q", stdout)
	}
}

func TestStripSSHFlags(t *testing.T) {
	in := []string{
		"--ssh", "user@host",
		"--ssh-arg", "-o",
		"--ssh-arg=BatchMode=yes",
		"run", "--name", "demo",
	}
	want := []string{"run", "--name", "demo"}
	got := stripSSHFlags(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stripSSHFlags() = %#v, want %#v", got, want)
	}
}

func TestShouldDelegateOverSSH(t *testing.T) {
	root := &cobra.Command{Use: "avdctl"}
	run := &cobra.Command{Use: "run"}
	version := &cobra.Command{Use: "version"}
	help := &cobra.Command{Use: "help"}
	root.AddCommand(run, version, help)

	if !shouldDelegateOverSSH(run, "user@host") {
		t.Fatal("run command should delegate in ssh mode")
	}
	if shouldDelegateOverSSH(version, "user@host") {
		t.Fatal("version command should not delegate")
	}
	if shouldDelegateOverSSH(help, "user@host") {
		t.Fatal("help command should not delegate")
	}
	if shouldDelegateOverSSH(root, "user@host") {
		t.Fatal("root command should not delegate")
	}
	if shouldDelegateOverSSH(run, "") {
		t.Fatal("delegation should require ssh target")
	}
}

func TestShouldAllocateTTYRespectsSSHArgsOverrides(t *testing.T) {
	if shouldAllocateTTY([]string{"-T"}) {
		t.Fatal("shouldAllocateTTY should be false when -T is provided")
	}
	if shouldAllocateTTY([]string{"-t"}) {
		t.Fatal("shouldAllocateTTY should be false when -t is provided")
	}
	if shouldAllocateTTY([]string{"-tt"}) {
		t.Fatal("shouldAllocateTTY should be false when -tt is provided")
	}
}
