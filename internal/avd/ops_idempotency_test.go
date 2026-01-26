// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCloneFromGoldenIdempotent(t *testing.T) {
	env := newTestEnv(t)
	baseName := "base-a35"
	cloneName := "clone-one"
	makeBaseAVD(t, env, baseName)
	goldenDir := makeGoldenDir(t)

	if _, err := CloneFromGolden(env, baseName, cloneName, goldenDir); err != nil {
		t.Fatalf("clone first: %v", err)
	}
	if _, err := CloneFromGolden(env, baseName, cloneName, goldenDir); err != nil {
		t.Fatalf("clone second: %v", err)
	}

	goldenFile := filepath.Join(goldenDir, "userdata-qemu.img")
	if err := os.WriteFile(goldenFile, []byte("changed"), 0o644); err != nil {
		t.Fatalf("update golden: %v", err)
	}
	if _, err := CloneFromGolden(env, baseName, cloneName, goldenDir); err == nil {
		t.Fatalf("expected clone conflict after golden change")
	}
}

func TestDeleteIdempotent(t *testing.T) {
	env := newTestEnv(t)
	if err := Delete(env, "missing"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}

	makeBaseAVD(t, env, "temp-avd")
	if err := Delete(env, "temp-avd"); err != nil {
		t.Fatalf("delete existing: %v", err)
	}
	if err := Delete(env, "temp-avd"); err != nil {
		t.Fatalf("delete again: %v", err)
	}
}

func TestStopIdempotent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires /proc")
	}
	env := newTestEnv(t)
	serial := "emulator-5580"
	if err := StopBySerial(env, serial); err != nil {
		t.Fatalf("stop first: %v", err)
	}
	if err := StopBySerial(env, serial); err != nil {
		t.Fatalf("stop second: %v", err)
	}
}

func TestConcurrentStopIdempotent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires /proc")
	}
	env := newTestEnv(t)
	serial := "emulator-5582"

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- StopBySerial(env, serial)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("stop error: %v", err)
		}
	}
}

func TestCleanupOrphans(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires /proc")
	}
	env := newTestEnv(t)

	orphanClone := filepath.Join(env.AVDHome, "orphan.avd")
	if err := os.MkdirAll(orphanClone, 0o755); err != nil {
		t.Fatalf("mkdir orphan clone: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphanClone, cloneFingerprintFilename), []byte("fingerprint"), 0o644); err != nil {
		t.Fatalf("write fingerprint: %v", err)
	}

	cmd := startDummyEmulator(t, env.AVDHome, "orphan-proc", 5590)
	defer stopDummyProcess(cmd)

	report, err := CleanupOrphans(env, false)
	if err != nil {
		t.Fatalf("cleanup scan: %v", err)
	}
	if len(report.OrphanedProcesses) != 1 {
		t.Fatalf("expected 1 orphan process, got %d", len(report.OrphanedProcesses))
	}
	if len(report.OrphanedAVDs) != 1 {
		t.Fatalf("expected 1 orphan avd, got %d", len(report.OrphanedAVDs))
	}

	report, err = CleanupOrphans(env, true)
	if err != nil {
		t.Fatalf("cleanup force: %v", err)
	}
	if len(report.OrphanedProcesses) != 1 || len(report.OrphanedAVDs) != 1 {
		t.Fatalf("unexpected report after force cleanup")
	}

	if _, err := os.Stat(orphanClone); !os.IsNotExist(err) {
		t.Fatalf("expected orphan clone removed")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if findEmulatorPID(5590) == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pid := findEmulatorPID(5590); pid != 0 {
		t.Fatalf("expected orphan process stopped, pid=%d", pid)
	}
}

func newTestEnv(t *testing.T) Env {
	t.Helper()
	root := t.TempDir()
	adbPath := filepath.Join(root, "adb")
	stub := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(adbPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("write adb stub: %v", err)
	}
	return Env{
		AVDHome: root,
		ADB:     adbPath,
	}
}

func makeBaseAVD(t *testing.T, env Env, name string) {
	t.Helper()
	avdDir := filepath.Join(env.AVDHome, name+".avd")
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	cfg := []byte("hw.device.name=pixel_6\n")
	if err := os.WriteFile(filepath.Join(avdDir, "config.ini"), cfg, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func makeGoldenDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir golden: %v", err)
	}
	files := []string{"userdata-qemu.img", "encryptionkey.img", "cache.img", "sdcard.img"}
	for i, name := range files {
		payload := []byte("data-" + strconv.Itoa(i))
		if err := os.WriteFile(filepath.Join(dir, name), payload, 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}
	return dir
}

func startDummyEmulator(t *testing.T, dir string, name string, port int) *os.Process {
	t.Helper()
	emuPath := filepath.Join(dir, "emulator")
	script := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile true; do sleep 1; done\n"
	if err := os.WriteFile(emuPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write emulator stub: %v", err)
	}
	cmd := execCommand(emuPath, "-avd", name, "-port", strconv.Itoa(port))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dummy emulator: %v", err)
	}
	return cmd.Process
}

func stopDummyProcess(proc *os.Process) {
	if proc == nil {
		return
	}
	_ = proc.Signal(os.Interrupt)
	_, _ = proc.Wait()
}

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
