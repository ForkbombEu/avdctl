package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	core "github.com/forkbombeu/avdctl/internal/avd"
	ioscore "github.com/forkbombeu/avdctl/internal/ios"
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

func TestRootHelpMentionsPlatformAwareCommands(t *testing.T) {
	root := newRootCommand("dev")
	root.SetArgs([]string{"--help"})

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)

	if err := root.Execute(); err != nil {
		t.Fatalf("help execution failed: %v", err)
	}

	help := stdout.String()
	for _, needle := range []string{
		"list and ps include both Android and iOS devices",
		"run, status, stop, and delete auto-detect the platform by name/reference",
		"list, init-base, run, clone, delete, ps, status, stop",
		"avdctl run ios --name base-ios",
		"avdctl run redroid --name redroid15 --data-dir ~/redroid-data --data-tar ~/redroid-data.tar",
		"run              Start a device; auto-detect android/ios by name, or use `run android|ios|redroid`",
	} {
		if !strings.Contains(help, needle) {
			t.Fatalf("help output missing %q\n%s", needle, help)
		}
	}
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

func TestSharedCommandsExposePlatformSubcommands(t *testing.T) {
	root := newRootCommand("dev")
	for _, name := range []string{"list", "init-base", "run", "clone", "delete", "ps", "status", "stop"} {
		cmd, _, err := root.Find([]string{name, "android"})
		if err != nil {
			t.Fatalf("find %s android: %v", name, err)
		}
		if cmd.Name() != "android" {
			t.Fatalf("expected %s android subcommand, got %s", name, cmd.Name())
		}

		cmd, _, err = root.Find([]string{name, "ios"})
		if err != nil {
			t.Fatalf("find %s ios: %v", name, err)
		}
		if cmd.Name() != "ios" {
			t.Fatalf("expected %s ios subcommand, got %s", name, cmd.Name())
		}
	}

	for _, name := range []string{"run", "delete", "stop"} {
		cmd, _, err := root.Find([]string{name, "redroid"})
		if err != nil {
			t.Fatalf("find %s redroid: %v", name, err)
		}
		if cmd.Name() != "redroid" {
			t.Fatalf("expected %s redroid subcommand, got %s", name, cmd.Name())
		}
	}
}

func TestIOSCommandFailsOnNonDarwinBuild(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("non-darwin guard is only meaningful for non-darwin builds")
	}

	root := newRootCommand("dev")
	root.SetArgs([]string{"list", "ios"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected ios command to fail on non-darwin build")
	}
	if !strings.Contains(err.Error(), "macOS build of avdctl") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoTopLevelRedroidCommand(t *testing.T) {
	root := newRootCommand("dev")
	cmd, _, err := root.Find([]string{"redroid"})
	if err == nil && cmd != nil && cmd.Name() == "redroid" {
		t.Fatal("did not expect a top-level redroid command")
	}
}

func TestPlatformListWithoutPlatformCombinesBothJSON(t *testing.T) {
	restore := restorePlatformHelperStubs()
	t.Cleanup(restore)

	androidListFn = func(core.Env) ([]core.Info, error) {
		return []core.Info{{Name: "pixel-a"}}, nil
	}
	iosEnsureSupportedFn = func() error { return nil }
	iosListFn = func(ioscore.Env) ([]ioscore.Info, error) {
		return []ioscore.Info{{Name: "iphone-a", UDID: "ios-1"}}, nil
	}

	root := newRootCommand("dev")
	root.SetArgs([]string{"list", "--json"})

	stdout := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("list execution failed: %v", err)
		}
	})
	for _, needle := range []string{`"android"`, `"ios"`, `"pixel-a"`, `"iphone-a"`} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("combined list output missing %q\n%s", needle, stdout)
		}
	}
}

func TestPlatformRunWithoutPlatformPrefersAndroidOnNameCollision(t *testing.T) {
	restore := restorePlatformHelperStubs()
	t.Cleanup(restore)

	androidListFn = func(core.Env) ([]core.Info, error) {
		return []core.Info{{Name: "shared"}}, nil
	}
	androidRunAVDFn = func(core.Env, string, time.Duration) (string, error) {
		return "emulator-5580", nil
	}
	iosEnsureSupportedFn = func() error { return nil }
	iosListFn = func(ioscore.Env) ([]ioscore.Info, error) {
		return []ioscore.Info{{Name: "shared", UDID: "ios-shared"}}, nil
	}
	iosRunFn = func(ioscore.Env, string) (ioscore.ProcInfo, error) {
		t.Fatal("ios run should not be called when android name collides")
		return ioscore.ProcInfo{}, nil
	}

	root := newRootCommand("dev")
	root.SetArgs([]string{"run", "--name", "shared"})

	if err := root.Execute(); err != nil {
		t.Fatalf("run execution failed: %v", err)
	}
}

func TestPlatformDeleteWithoutPlatformFallsBackToIOS(t *testing.T) {
	restore := restorePlatformHelperStubs()
	t.Cleanup(restore)

	androidListFn = func(core.Env) ([]core.Info, error) {
		return nil, nil
	}
	iosEnsureSupportedFn = func() error { return nil }
	iosListFn = func(ioscore.Env) ([]ioscore.Info, error) {
		return []ioscore.Info{{Name: "ios-only", UDID: "ios-1"}}, nil
	}

	deleted := ""
	iosDeleteFn = func(_ ioscore.Env, ref string) error {
		deleted = ref
		return nil
	}

	root := newRootCommand("dev")
	root.SetArgs([]string{"delete", "ios-only"})

	stdout := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("delete execution failed: %v", err)
		}
	})
	if deleted != "ios-only" {
		t.Fatalf("expected ios delete to receive ios-only, got %q", deleted)
	}
	if !strings.Contains(stdout, "Deleted ios-only") {
		t.Fatalf("unexpected delete output: %s", stdout)
	}
}

func TestPlatformStatusAllWithoutPlatformCombinesBoth(t *testing.T) {
	restore := restorePlatformHelperStubs()
	t.Cleanup(restore)

	androidListFn = func(core.Env) ([]core.Info, error) {
		return []core.Info{{Name: "pixel-a"}}, nil
	}
	androidListRunningFn = func(core.Env) ([]core.ProcInfo, error) {
		return []core.ProcInfo{{Name: "pixel-a", Serial: "emulator-5580", Port: 5580, PID: 10, Booted: true}}, nil
	}
	iosEnsureSupportedFn = func() error { return nil }
	iosListFn = func(ioscore.Env) ([]ioscore.Info, error) {
		return []ioscore.Info{{Name: "iphone-a", UDID: "ios-1", State: "Shutdown"}}, nil
	}

	root := newRootCommand("dev")
	root.SetArgs([]string{"status", "--all"})

	stdout := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("status execution failed: %v", err)
		}
	})
	for _, needle := range []string{"Android", "pixel-a", "iOS", "iphone-a"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("combined status output missing %q\n%s", needle, stdout)
		}
	}
}

func TestPlatformStopWithoutPlatformFallsBackToIOS(t *testing.T) {
	restore := restorePlatformHelperStubs()
	t.Cleanup(restore)

	androidListFn = func(core.Env) ([]core.Info, error) {
		return nil, nil
	}
	iosEnsureSupportedFn = func() error { return nil }
	iosListFn = func(ioscore.Env) ([]ioscore.Info, error) {
		return []ioscore.Info{{Name: "ios-only", UDID: "ios-1"}}, nil
	}

	stopped := ""
	iosStopFn = func(_ ioscore.Env, ref string) error {
		stopped = ref
		return nil
	}

	root := newRootCommand("dev")
	root.SetArgs([]string{"stop", "--name", "ios-only"})

	stdout := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("stop execution failed: %v", err)
		}
	})
	if stopped != "ios-only" {
		t.Fatalf("expected ios stop to receive ios-only, got %q", stopped)
	}
	if !strings.Contains(stdout, "Stopped ios-only") {
		t.Fatalf("unexpected stop output: %s", stdout)
	}
}
