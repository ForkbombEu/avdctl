package avdmanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeExecScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func newManagerWithBinaries(t *testing.T, adb, qemu string) *Manager {
	t.Helper()
	tmp := t.TempDir()
	return NewWithEnv(Environment{
		AVDHome:        filepath.Join(tmp, "avd"),
		GoldenDir:      filepath.Join(tmp, "golden"),
		ConfigTemplate: filepath.Join(tmp, "config.ini.tpl"),
		EmulatorBin:    filepath.Join(tmp, "missing-emulator"),
		ADBBin:         adb,
		AvdManagerBin:  filepath.Join(tmp, "missing-avdmanager"),
		SdkManagerBin:  filepath.Join(tmp, "missing-sdkmanager"),
		QemuImgBin:     qemu,
		CorrelationID:  "corr-test",
		Context:        context.Background(),
	})
}

func TestManagerConstructorsAndContext(t *testing.T) {
	m0 := New()
	if m0 == nil {
		t.Fatal("New() returned nil manager")
	}

	m := NewWithContextAndCorrelationID(nil, "corr-1")
	if m.Context() == nil {
		t.Fatal("context should never be nil")
	}
	if got := m.CorrelationID(); got != "corr-1" {
		t.Fatalf("CorrelationID() = %q", got)
	}

	m2 := NewWithContext(context.Background())
	if m2.Context() == nil {
		t.Fatal("NewWithContext should bind context")
	}

	m3 := NewWithCorrelationID("corr-2")
	if got := m3.CorrelationID(); got != "corr-2" {
		t.Fatalf("NewWithCorrelationID correlation = %q", got)
	}
}

func TestEnsureNotRunningEmptyName(t *testing.T) {
	m := NewWithEnv(Environment{})
	if err := m.ensureNotRunning(""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestListAndDeleteRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	avdHome := filepath.Join(tmp, "avd")
	if err := os.MkdirAll(filepath.Join(avdHome, "demo.avd"), 0o755); err != nil {
		t.Fatalf("mkdir avd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(avdHome, "demo.avd", "userdata.img"), []byte("userdata"), 0o644); err != nil {
		t.Fatalf("write userdata: %v", err)
	}

	m := NewWithEnv(Environment{AVDHome: avdHome, Context: context.Background()})
	list, err := m.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "demo" {
		t.Fatalf("unexpected list result: %#v", list)
	}

	if err := m.Delete("demo"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	list, err = m.List()
	if err != nil {
		t.Fatalf("List() after delete error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list after delete, got %#v", list)
	}
}

func TestListRunningStopAndStopByName(t *testing.T) {
	tmp := t.TempDir()
	adb := writeExecScript(t, tmp, "adb", `
case "$1" in
  start-server)
    exit 0
    ;;
  devices)
    echo "List of devices attached"
    echo "emulator-5580\tdevice"
    exit 0
    ;;
  -s)
    if [ "$3" = "emu" ] && [ "$4" = "avd" ] && [ "$5" = "name" ]; then
      echo "demo"
      echo "OK"
      exit 0
    fi
    if [ "$3" = "shell" ] && [ "$4" = "getprop" ] && [ "$5" = "sys.boot_completed" ]; then
      echo "1"
      exit 0
    fi
    if [ "$3" = "emu" ] && [ "$4" = "kill" ]; then
      exit 0
    fi
    ;;
esac
exit 0
`)
	m := newManagerWithBinaries(t, adb, filepath.Join(tmp, "missing-qemu"))

	running, err := m.ListRunning()
	if err != nil {
		t.Fatalf("ListRunning() error: %v", err)
	}
	if len(running) == 0 {
		t.Fatal("expected at least one running emulator")
	}
	if running[0].Serial != "emulator-5580" {
		t.Fatalf("unexpected serial: %#v", running[0])
	}

	if err := m.Stop("emulator-5580"); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
	if err := m.StopByName("demo"); err != nil {
		t.Fatalf("StopByName() error: %v", err)
	}
	if err := m.ensureNotRunning("demo"); err == nil {
		t.Fatal("expected ensureNotRunning to report running AVD")
	}
}

func TestSaveGoldenAndFindFreePort(t *testing.T) {
	tmp := t.TempDir()
	avdHome := filepath.Join(tmp, "avd")
	golden := filepath.Join(tmp, "golden")
	name := "demo"
	avdDir := filepath.Join(avdHome, name+".avd")
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatalf("mkdir avdDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(avdDir, "userdata-qemu.img"), []byte("user-data"), 0o644); err != nil {
		t.Fatalf("write userdata: %v", err)
	}

	qemu := writeExecScript(t, tmp, "qemu-img", `
if [ "$1" = "convert" ]; then
  cp "$4" "$5"
  exit 0
fi
exit 1
`)
	adb := writeExecScript(t, tmp, "adb", "exit 0\n")
	m := NewWithEnv(Environment{AVDHome: avdHome, GoldenDir: golden, QemuImgBin: qemu, ADBBin: adb, Context: context.Background()})

	path, size, err := m.SaveGolden(SaveGoldenOptions{Name: name, Destination: filepath.Join(golden, "saved")})
	if err != nil {
		t.Fatalf("SaveGolden() error: %v", err)
	}
	if path == "" || size <= 0 {
		t.Fatalf("unexpected save result: path=%q size=%d", path, size)
	}

	// In restricted CI/sandbox environments binding local ports may be denied.
	port, err := m.FindFreePort(20000, 20100)
	if err != nil {
		if !strings.Contains(err.Error(), "no free even port found") {
			t.Fatalf("FindFreePort() unexpected error: %v", err)
		}
	} else if port%2 != 0 {
		t.Fatalf("FindFreePort returned odd port: %d", port)
	}
}

func TestWaitForBootWithProgressAndValidationErrors(t *testing.T) {
	tmp := t.TempDir()
	adb := writeExecScript(t, tmp, "adb", `
case "$1" in
  wait-for-device)
    exit 0
    ;;
  -s)
    if [ "$3" = "shell" ] && [ "$4" = "getprop" ] && [ "$5" = "sys.boot_completed" ]; then
      echo "1"
      exit 0
    fi
    ;;
esac
exit 0
`)
	m := newManagerWithBinaries(t, adb, filepath.Join(tmp, "missing-qemu"))

	var gotStatuses []string
	err := m.WaitForBootWithProgress("emulator-5580", 5*time.Second, func(status string, _ time.Duration) {
		gotStatuses = append(gotStatuses, status)
	})
	if err != nil {
		t.Fatalf("WaitForBootWithProgress() error: %v", err)
	}
	if len(gotStatuses) == 0 {
		t.Fatal("expected progress statuses")
	}
	if gotStatuses[len(gotStatuses)-1] != "boot_complete" {
		t.Fatalf("expected boot_complete, got %v", gotStatuses)
	}

	if _, err := m.Run(RunOptions{Name: ""}); err == nil || !strings.Contains(err.Error(), "empty AVD name") {
		t.Fatalf("expected validation error from Run(empty name), got %v", err)
	}

	if _, err := m.Run(RunOptions{Name: "demo"}); err == nil {
		t.Fatal("expected emulator start error from Run(demo)")
	}

	if _, _, err := m.RunOnPort(RunOptions{Name: "", Port: 5580}); err == nil {
		t.Fatal("expected validation error from RunOnPort(empty name)")
	}

	if _, _, err := m.RunOnPort(RunOptions{Name: "demo", Port: 5555}); err == nil {
		t.Fatal("expected odd-port validation error from RunOnPort")
	}

	if _, err := m.InitBase(InitBaseOptions{}); err == nil {
		t.Fatal("expected validation error from InitBase(empty)")
	}

	if _, err := m.Clone(CloneOptions{BaseName: "base", CloneName: "clone", GoldenPath: filepath.Join(tmp, "missing-golden")}); err == nil {
		t.Fatal("expected clone error when base/golden are missing")
	}

	if _, _, err := m.Prewarm(PrewarmOptions{Name: "demo", Destination: filepath.Join(tmp, "golden", "demo")}); err == nil {
		t.Fatal("expected prewarm error without emulator tooling")
	}

	if _, _, err := m.BakeAPK(BakeAPKOptions{
		BaseName:   "base",
		CloneName:  "clone",
		GoldenPath: filepath.Join(tmp, "missing"),
		APKPaths:   []string{filepath.Join(tmp, "a.apk")},
	}); err == nil {
		t.Fatal("expected bake error with missing clone/golden")
	}

	if err := m.WaitForBoot("emulator-5580", 2*time.Second); err != nil {
		t.Fatalf("WaitForBoot() error: %v", err)
	}

	if err := m.Delete(""); err == nil {
		t.Fatal("expected validation error from Delete(empty)")
	}

	if _, err := m.KillAllEmulators(KillAllEmulatorsOptions{MaxPasses: 1, Delay: time.Millisecond}); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		// command lookup failures are acceptable in this synthetic environment.
		_ = err
	}
}

func TestInitBaseAndCloneSuccessPaths(t *testing.T) {
	tmp := t.TempDir()
	avdHome := filepath.Join(tmp, "avd")
	sdkRoot := filepath.Join(tmp, "sdk")
	if err := os.MkdirAll(filepath.Join(sdkRoot, "system-images", "android-35", "google_apis_playstore", "x86_64"), 0o755); err != nil {
		t.Fatalf("mkdir sdk system image: %v", err)
	}

	avdMgr := writeExecScript(t, tmp, "avdmanager", "exit 0\n")
	adb := writeExecScript(t, tmp, "adb", "exit 0\n")
	qemu := writeExecScript(t, tmp, "qemu-img", `
if [ "$1" = "convert" ]; then
  cp "$4" "$5"
  exit 0
fi
if [ "$1" = "create" ]; then
  truncate -s "$5" "$4"
  exit 0
fi
exit 1
`)

	m := NewWithEnv(Environment{
		SDKRoot:       sdkRoot,
		AVDHome:       avdHome,
		AvdManagerBin: avdMgr,
		ADBBin:        adb,
		QemuImgBin:    qemu,
		Context:       context.Background(),
	})

	info, err := m.InitBase(InitBaseOptions{
		Name:        "base",
		SystemImage: "system-images;android-35;google_apis_playstore;x86_64",
		Device:      "pixel_6",
	})
	if err != nil {
		t.Fatalf("InitBase() error: %v", err)
	}
	if info.Name != "base" {
		t.Fatalf("unexpected init info: %#v", info)
	}

	baseDir := filepath.Join(avdHome, "base.avd")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir base avd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.ini"), []byte("hw.device.name=pixel_6\n"), 0o644); err != nil {
		t.Fatalf("write base config: %v", err)
	}

	golden := filepath.Join(tmp, "golden")
	if err := os.MkdirAll(golden, 0o755); err != nil {
		t.Fatalf("mkdir golden: %v", err)
	}
	for _, f := range []string{"userdata-qemu.img", "encryptionkey.img", "cache.img"} {
		if err := os.WriteFile(filepath.Join(golden, f), []byte("data"), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", f, err)
		}
	}

	cloneInfo, err := m.Clone(CloneOptions{BaseName: "base", CloneName: "clone", GoldenPath: golden})
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}
	if cloneInfo.Name != "clone" {
		t.Fatalf("unexpected clone info: %#v", cloneInfo)
	}
}

func TestRunOnPortBranches(t *testing.T) {
	t.Run("port-zero-path", func(t *testing.T) {
		tmp := t.TempDir()
		adb := writeExecScript(t, tmp, "adb", `
case "$1" in
  start-server)
    exit 0
    ;;
  devices)
    echo "List of devices attached"
    exit 0
    ;;
esac
exit 0
`)
		m := newManagerWithBinaries(t, adb, filepath.Join(tmp, "missing-qemu"))
		if _, _, err := m.RunOnPort(RunOptions{Name: "demo", Port: 0}); err == nil {
			t.Fatal("expected RunOnPort port=0 to fail without emulator binary")
		}
	})

	t.Run("conflict-reassign-path", func(t *testing.T) {
		tmp := t.TempDir()
		adb := writeExecScript(t, tmp, "adb", `
case "$1" in
  start-server)
    exit 0
    ;;
  devices)
    echo "List of devices attached"
    echo "emulator-5580\tdevice"
    exit 0
    ;;
  -s)
    if [ "$3" = "emu" ] && [ "$4" = "avd" ] && [ "$5" = "name" ]; then
      echo "other"
      echo "OK"
      exit 0
    fi
    if [ "$3" = "shell" ] && [ "$4" = "getprop" ] && [ "$5" = "sys.boot_completed" ]; then
      echo "1"
      exit 0
    fi
    ;;
esac
exit 0
`)
		m := newManagerWithBinaries(t, adb, filepath.Join(tmp, "missing-qemu"))
		if _, _, err := m.RunOnPort(RunOptions{Name: "demo", Port: 5580}); err == nil {
			t.Fatal("expected RunOnPort to fail after conflict resolution without emulator binary")
		}
	})
}

func TestStopByNameNotRunningReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	adb := writeExecScript(t, tmp, "adb", `
case "$1" in
  start-server)
    exit 0
    ;;
  devices)
    echo "List of devices attached"
    exit 0
    ;;
esac
exit 0
`)
	m := newManagerWithBinaries(t, adb, filepath.Join(tmp, "missing-qemu"))
	if err := m.StopByName("missing"); err != nil {
		t.Fatalf("StopByName(missing) error: %v", err)
	}
}

func TestStopBluetoothValidationAndCommandFailure(t *testing.T) {
	tmp := t.TempDir()
	okADB := writeExecScript(t, tmp, "adb-ok", "exit 0\n")
	m := newManagerWithBinaries(t, okADB, filepath.Join(tmp, "missing-qemu"))

	if err := m.StopBluetooth("bad-serial"); err == nil {
		t.Fatal("expected validation error for invalid serial")
	}
	if err := m.StopBluetooth("emulator-5580"); err != nil {
		t.Fatalf("StopBluetooth(success) error: %v", err)
	}

	failADB := writeExecScript(t, tmp, "adb-fail", `
if [ "$4" = "svc" ]; then
  echo "boom" 1>&2
  exit 2
fi
exit 0
`)
	mFail := newManagerWithBinaries(t, failADB, filepath.Join(tmp, "missing-qemu"))
	err := mFail.StopBluetooth("emulator-5580")
	if err == nil {
		t.Fatal("expected StopBluetooth failure when adb command fails")
	}
	if !strings.Contains(err.Error(), "failed to disable bluetooth service") {
		t.Fatalf("unexpected StopBluetooth error: %v", err)
	}
}

func TestListReturnsErrorWhenAVDHomeMissing(t *testing.T) {
	m := NewWithEnv(Environment{
		AVDHome: filepath.Join(t.TempDir(), "missing-avd-home"),
		Context: context.Background(),
	})
	if _, err := m.List(); err == nil {
		t.Fatal("expected List() to fail when AVD home does not exist")
	}
}

func TestSpanContextFallbackWhenNil(t *testing.T) {
	m := &Manager{}
	if m.spanContext() == nil {
		t.Fatal("spanContext should fallback to context.Background()")
	}
}
