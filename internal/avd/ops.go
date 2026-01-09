// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

type Info struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Userdata  string `json:"userdata"`
	SizeBytes int64  `json:"size_bytes"`
}

func run(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v failed: %v\n%s", bin, args, err, buf.String())
	}
	return nil
}

func List(env Env) ([]Info, error) {
	_, span := startSpan(env, "avd.List")
	defer span.End()
	entries, err := os.ReadDir(env.AVDHome)
	if err != nil {
		recordSpanError(span, err)
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".avd") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".avd")
		dir := filepath.Join(env.AVDHome, e.Name())
		ud := filepath.Join(dir, "userdata-qemu.img.qcow2")
		if _, err := os.Stat(ud); err != nil {
			ud = filepath.Join(dir, "userdata.img")
		}
		var sz int64
		if st, err := os.Stat(ud); err == nil {
			sz = st.Size()
		}
		out = append(out, Info{Name: name, Path: dir, Userdata: ud, SizeBytes: sz})
	}
	return out, nil
}

func ensureSysImg(env Env, pkg string) error {
	if env.SDKRoot != "" {
		// quick existence probe
		parts := strings.Split(pkg, ";")
		if len(parts) >= 3 {
			p := filepath.Join(env.SDKRoot, "system-images", parts[1], parts[2], "x86_64")
			if _, err := os.Stat(p); err == nil {
				return nil
			}
		}
	}
	// install via sdkmanager
	// accept licenses if needed
	_ = run(env.SdkManager, "--licenses")
	return run(env.SdkManager, pkg)
}

func InitBase(env Env, name, sysImage, device string) (Info, error) {
	if name == "" {
		return Info{}, errors.New("empty AVD name")
	}
	if err := os.MkdirAll(env.AVDHome, 0o755); err != nil {
		return Info{}, err
	}
	if err := ensureSysImg(env, sysImage); err != nil {
		return Info{}, fmt.Errorf("failed to ensure system image: %w", err)
	}
	cmd := exec.Command(env.AvdMgr, "create", "avd",
		"-n", name, "-k", sysImage, "-d", device, "--force")
	cmd.Stdin = strings.NewReader("no\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Info{}, fmt.Errorf("avdmanager create: %v\n%s", err, out)
	}
	return infoOf(env, name)
}

// SaveGolden exports an AVD's writable images (userdata, encryptionkey, cache) to a golden directory.
// Converts qcow2 overlays to raw IMG format to prevent Android emulator from re-creating overlays on boot.
// Returns the golden directory path and total size.
func SaveGolden(env Env, name, dest string) (string, int64, error) {
	avdPath := filepath.Join(env.AVDHome, name+".avd")

	// Create golden directory
	goldenDir := dest
	if filepath.Ext(dest) == ".qcow2" {
		// Legacy single-file mode: create directory instead
		goldenDir = strings.TrimSuffix(dest, ".qcow2")
	}
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		return "", 0, err
	}

	// List of writable images to save (base name)
	images := []string{"userdata-qemu.img", "encryptionkey.img", "cache.img", "sdcard.img"}
	var totalSize int64

	for _, img := range images {
		// Prefer qcow2 overlay (has customizations), fallback to raw
		src := filepath.Join(avdPath, img+".qcow2")
		if _, err := os.Stat(src); err != nil {
			src = filepath.Join(avdPath, img)
			if _, err2 := os.Stat(src); err2 != nil {
				continue // Skip if not found
			}
		}

		// Convert to raw IMG (not qcow2) to prevent emulator from creating overlays
		dstFile := filepath.Join(goldenDir, img)
		tmp := dstFile + ".tmp"
		if err := run(env.QemuImg, "convert", "-O", "raw", src, tmp); err != nil {
			return "", 0, fmt.Errorf("convert %s: %w", img, err)
		}
		if err := os.Rename(tmp, dstFile); err != nil {
			return "", 0, err
		}
		if st, err := os.Stat(dstFile); err == nil {
			totalSize += st.Size()
		}
	}

	return goldenDir, totalSize, nil
}

// CloneFromGolden creates a new AVD directory by copying raw IMG files from golden directory.
// Uses full file copy (not QCOW2 overlays) to preserve all customizations independently.
// It symlinks the base AVD's read-only files (system images, ROMs) and copies writable images.
// Cloning takes time proportional to golden image size but ensures full isolation.
func CloneFromGolden(env Env, base, name, golden string) (Info, error) {
	_, span := startSpan(
		env,
		"avd.CloneFromGolden",
		attribute.String("base", base),
		attribute.String("clone", name),
	)
	defer span.End()
	logEvent(
		env,
		"avd clone start",
		"base",
		base,
		"clone",
		name,
		"golden_path",
		golden,
	)
	baseDir := filepath.Join(env.AVDHome, base+".avd")
	cloneDir := filepath.Join(env.AVDHome, name+".avd")

	if _, err := os.Stat(baseDir); err != nil {
		recordSpanError(span, err)
		return Info{}, fmt.Errorf("base AVD not found: %w", err)
	}
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		recordSpanError(span, err)
		return Info{}, err
	}

	// Resolve golden path (can be directory or legacy .qcow2 file)
	goldenDir := golden
	if filepath.Ext(golden) == ".qcow2" {
		goldenDir = filepath.Dir(golden)
	}
	absGoldenDir, err := filepath.Abs(goldenDir)
	if err != nil {
		recordSpanError(span, err)
		return Info{}, fmt.Errorf("resolve golden path: %w", err)
	}

	// ---------------------------------------------------------------------
	// 1. Copy or template the config.ini and disable qcow2
	// ---------------------------------------------------------------------
	tpl := os.Getenv("AVDCTL_CONFIG_TEMPLATE")
	dstCfg := filepath.Join(cloneDir, "config.ini")
	var cfgBytes []byte

	switch {
	case tpl != "":
		cfgBytes, err = os.ReadFile(tpl)
		if err != nil {
			recordSpanError(span, err)
			return Info{}, fmt.Errorf("read template: %w", err)
		}
	default:
		cfgBytes, err = os.ReadFile(filepath.Join(baseDir, "config.ini"))
		if err != nil {
			recordSpanError(span, err)
			return Info{}, fmt.Errorf("read base config: %w", err)
		}
	}

	cfgBytes = sanitizeConfigINI(cfgBytes)
	// Disable QCOW2 overlays (use raw IMG full copies)
	cfgStr := string(cfgBytes)
	cfgStr = strings.ReplaceAll(cfgStr, "userdata.useQcow2=yes", "userdata.useQcow2=no")
	if !strings.Contains(cfgStr, "userdata.useQcow2") {
		cfgStr += "\nuserdata.useQcow2=no\n"
	}
	if err := os.WriteFile(dstCfg, []byte(cfgStr), 0o644); err != nil {
		recordSpanError(span, err)
		return Info{}, fmt.Errorf("write clone config: %w", err)
	}

	// ---------------------------------------------------------------------
	// 2. Symlink read-only artifacts from base to clone
	// ---------------------------------------------------------------------
	err = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == baseDir {
			return nil
		}
		rel, _ := filepath.Rel(baseDir, path)

		// Skip: snapshots, cache*, userdata*, encryptionkey*, config.ini, locks
		if strings.HasPrefix(rel, "snapshots") ||
			strings.HasPrefix(rel, "cache") ||
			strings.HasPrefix(rel, "userdata") ||
			strings.HasPrefix(rel, "encryptionkey") ||
			rel == "config.ini" ||
			strings.HasSuffix(rel, ".lock") {
			return nil
		}

		dst := filepath.Join(cloneDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		// Symlink
		if err := os.Symlink(path, dst); err != nil && !os.IsExist(err) {
			return err
		}
		return nil
	})
	if err != nil {
		recordSpanError(span, err)
		return Info{}, err
	}

	// ---------------------------------------------------------------------
	// 3. Copy raw IMG files from golden directory (full copy, no overlays)
	// ---------------------------------------------------------------------
	images := []string{"userdata-qemu.img", "encryptionkey.img", "cache.img", "sdcard.img"}
	for _, img := range images {
		goldenFile := filepath.Join(absGoldenDir, img)
		if _, err := os.Stat(goldenFile); err != nil {
			// If sdcard.img is missing, create it from config.ini sdcard.size
			if img == "sdcard.img" {
				if err := createSDCard(env, cloneDir, dstCfg); err != nil {
					recordSpanError(span, err)
					return Info{}, fmt.Errorf("create sdcard: %w", err)
				}
			}
			continue // Skip if golden image doesn't exist
		}

		dstFile := filepath.Join(cloneDir, img)
		// Stream copy to avoid loading entire file into memory
		src, err := os.Open(goldenFile)
		if err != nil {
			recordSpanError(span, err)
			return Info{}, fmt.Errorf("open golden %s: %w", img, err)
		}
		dst, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			src.Close()
			recordSpanError(span, err)
			return Info{}, fmt.Errorf("create clone %s: %w", img, err)
		}
		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			recordSpanError(span, err)
			return Info{}, fmt.Errorf("copy %s: %w", img, err)
		}
	}

	// ---------------------------------------------------------------------
	// 4. Remove stale snapshot dirs and qcow2 overlays if any
	// ---------------------------------------------------------------------
	_ = os.RemoveAll(filepath.Join(cloneDir, "snapshots"))

	// Remove any leftover qcow2 overlay files to ensure clean raw IMG usage
	qcow2Files, _ := filepath.Glob(filepath.Join(cloneDir, "*.qcow2"))
	for _, f := range qcow2Files {
		_ = os.Remove(f)
	}

	// ---------------------------------------------------------------------
	// 5. Create the .ini file
	// ---------------------------------------------------------------------
	ini := filepath.Join(env.AVDHome, name+".ini")
	body := fmt.Sprintf(
		"avd.ini.encoding=UTF-8\npath=%s\npath.rel=avd/%s\n",
		cloneDir, name+".avd",
	)
	if err := os.WriteFile(ini, []byte(body), 0o644); err != nil {
		return Info{}, err
	}

	// ---------------------------------------------------------------------
	// 6. Report size & info
	// ---------------------------------------------------------------------
	// Check for raw IMG first (full copy), fallback to QCOW2 overlay
	userdata := filepath.Join(cloneDir, "userdata-qemu.img")
	fi, err := os.Stat(userdata)
	if err != nil {
		userdata = filepath.Join(cloneDir, "userdata-qemu.img.qcow2")
		fi, err = os.Stat(userdata)
		if err != nil {
			recordSpanError(span, err)
			return Info{}, fmt.Errorf("stat userdata: %w", err)
		}
	}
	info := Info{
		Name:      name,
		Path:      cloneDir,
		Userdata:  userdata,
		SizeBytes: fi.Size(),
	}
	logEvent(
		env,
		"avd clone finished",
		"clone",
		name,
		"path",
		cloneDir,
		"userdata",
		userdata,
		"size_bytes",
		fi.Size(),
	)
	return info, nil
}

func sanitizeConfigINI(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(l, "QuickBoot.mode=") ||
			strings.HasPrefix(l, "snapshot.present=") ||
			strings.HasPrefix(l, "fastboot.") ||
			strings.HasPrefix(l, "disk.dataPartition.") ||
			strings.HasPrefix(l, "userdata.useQcow2=") ||
			strings.HasPrefix(l, "firstboot.") {
			continue
		}
		out = append(out, l)
	}
	out = append(out, "QuickBoot.mode=disabled")
	out = append(out, "snapshot.present=false")
	out = append(out, "fastboot.forceColdBoot=yes")
	out = append(out, "userdata.useQcow2=yes")
	return []byte(strings.Join(out, "\n"))
}

func StartEmulator(env Env, name string, extraArgs ...string) (*exec.Cmd, error) {
	_, span := startSpan(
		env,
		"avd.StartEmulator",
		attribute.String("name", name),
	)
	defer span.End()
	args := []string{
		"-avd", name,
		"-no-window",
		"-no-boot-anim",
		"-no-snapshot",
		"-no-snapshot-load",
		"-no-snapshot-save",
		"-skip-adb-auth",
		"-no-metrics",
		"-no-location-ui",
		"-no-audio",
		"-read-only",
		"-gpu", "swiftshader_indirect",
		"-logcat", "*:S",
	}

	args = append(args, extraArgs...)
	cmd := exec.Command(env.Emulator, args...)
	cmd.Env = append(os.Environ(), "QEMU_FILE_LOCKING=off", "ADB_VENDOR_KEYS=/dev/null")
	if err := cmd.Start(); err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("emulator start: %w", err)
	}
	span.SetAttributes(attribute.Int("pid", cmd.Process.Pid))
	return cmd, nil
}

func GuessEmulatorSerial(env Env) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command(env.ADB, "devices")
	cmd.Stdout = &buf
	_ = cmd.Run()
	for _, line := range strings.Split(buf.String(), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.HasPrefix(f[0], "emulator-") && f[1] == "device" {
			return f[0], nil
		}
	}
	return "", errors.New("no emulator device found")
}

func WaitForBoot(env Env, serial string, timeout time.Duration) error {
	_, span := startSpan(
		env,
		"avd.WaitForBoot",
		attribute.String("serial", serial),
		attribute.String("timeout", timeout.String()),
	)
	defer span.End()
	deadline := time.Now().Add(timeout)
	_ = run(env.ADB, "wait-for-device")

	lastError := ""
	for time.Now().Before(deadline) {
		var out bytes.Buffer
		var errOut bytes.Buffer
		cmd := exec.Command(env.ADB, "-s", serial, "shell", "getprop", "sys.boot_completed")
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		err := cmd.Run()

		bootCompleted := strings.TrimSpace(out.String())
		if bootCompleted == "1" {
			time.Sleep(2 * time.Second)
			span.SetAttributes(attribute.Bool("boot_completed", true))
			return nil
		}

		// Track last error for better diagnostics
		if err != nil {
			lastError = errOut.String()
			if lastError == "" {
				lastError = err.Error()
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Provide helpful error message
	errMsg := fmt.Sprintf("boot timeout after %s (adb could not confirm boot completion)", timeout)
	if lastError != "" {
		errMsg += fmt.Sprintf("\nLast ADB error: %s", strings.TrimSpace(lastError))
	}
	errMsg += fmt.Sprintf("\nHint: Check if emulator is still running and adb can see it: adb devices")
	errMsg += fmt.Sprintf("\nNote: The emulator may have booted successfully but ADB lost connection.")

	logEvent(
		env,
		"wait for boot timeout",
		"serial",
		serial,
		"timeout",
		timeout.String(),
		"adb_error",
		strings.TrimSpace(lastError),
	)
	recordSpanError(span, fmt.Errorf("boot timeout after %s", timeout))
	return fmt.Errorf("%s", errMsg)
}

func KillEmulator(env Env, serial string) {
	_ = exec.Command(env.ADB, "-s", serial, "emu", "kill").Run()
	time.Sleep(1 * time.Second)
}

func PrewarmGolden(env Env, name, dest string, extra time.Duration, bootTimeout time.Duration) (string, int64, error) {
	// Restart ADB server to clear stale state
	_ = exec.Command(env.ADB, "kill-server").Run()
	time.Sleep(1 * time.Second)
	ensureADB(env)

	// Find a free port dynamically to avoid conflicts
	port, err := FindFreeEvenPort(5580, 5800)
	if err != nil {
		return "", 0, fmt.Errorf("no free port available for prewarming: %w", err)
	}
	cmd, serial, logPath, err := StartEmulatorOnPort(env, name, port)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = cmd.Process.Kill() }()

	// Wait until adb sees that specific emulator serial
	if err := waitForEmulatorSerial(env, serial, 60*time.Second); err != nil {
		return "", 0, fmt.Errorf("ADB failed to detect emulator serial %s: %w\nEmulator log: %s\nNote: The emulator may still be starting. Check the log file for details.", serial, err, logPath)
	}

	// Now wait for Android to finish booting
	if err := WaitForBoot(env, serial, bootTimeout); err != nil {
		// Check if userdata was created (indicates boot likely succeeded)
		avdPath := filepath.Join(env.AVDHome, name+".avd")
		userdata1 := filepath.Join(avdPath, "userdata-qemu.img.qcow2")
		userdata2 := filepath.Join(avdPath, "userdata-qemu.img")
		if st, statErr := os.Stat(userdata1); statErr == nil && st.Size() > 1024*1024 {
			KillEmulator(env, serial)
			return SaveGolden(env, name, dest)
		}
		if st, statErr := os.Stat(userdata2); statErr == nil && st.Size() > 1024*1024 {
			KillEmulator(env, serial)
			return SaveGolden(env, name, dest)
		}
		return "", 0, fmt.Errorf("%w\nEmulator log: %s", err, logPath)
	}

	// Disable lockscreen and complete setup
	_ = run(env.ADB, "-s", serial, "shell", "settings", "put", "global", "device_provisioned", "1")
	_ = run(env.ADB, "-s", serial, "shell", "settings", "put", "secure", "user_setup_complete", "1")
	_ = run(env.ADB, "-s", serial, "shell", "locksettings", "set-disabled", "true")
	_ = run(env.ADB, "-s", serial, "shell", "wm", "dismiss-keyguard")
	_ = run(env.ADB, "-s", serial, "shell", "input", "keyevent", "82") // MENU key to wake/unlock

	if extra > 0 {
		time.Sleep(extra)
	}

	KillEmulator(env, serial)
	return SaveGolden(env, name, dest)
}

func RunAVD(env Env, name string, extraArgs ...string) (string, error) {
	_, span := startSpan(
		env,
		"avd.RunAVD",
		attribute.String("name", name),
	)
	defer span.End()
	ensureADB(env)
	port, err := FindFreeEvenPort(5580, 5800)
	if err != nil {
		recordSpanError(span, err)
		return "", err
	}
	_, serial, logPath, err := StartEmulatorOnPort(env, name, port, extraArgs...)
	if err != nil {
		recordSpanError(span, err)
		return "", err
	}

	// wait up to 60s for adb to see this exact serial
	if err := waitForEmulatorSerial(env, serial, 60*time.Second); err != nil {
		recordSpanError(span, err)
		return "", fmt.Errorf("%w\nemulator log: %s", err, logPath)
	}
	span.SetAttributes(attribute.String("serial", serial))
	fmt.Printf("Started %s on %s (log: %s)\n", name, serial, logPath)
	return serial, nil
}

func BakeAPK(env Env, base, name, golden string, apks []string, timeout time.Duration) (string, int64, error) {
	if _, err := CloneFromGolden(env, base, name, golden); err != nil {
		return "", 0, err
	}
	cmd, err := StartEmulator(env, name)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = cmd.Process.Kill() }()

	serial, err := GuessEmulatorSerial(env)
	if err != nil {
		return "", 0, err
	}
	if err := WaitForBoot(env, serial, timeout); err != nil {
		return "", 0, err
	}
	for _, apk := range apks {
		if err := run(env.ADB, "-s", serial, "install", "-r", apk); err != nil {
			return "", 0, fmt.Errorf("install %s: %w", apk, err)
		}
	}
	KillEmulator(env, serial)

	// Return overlay path and size
	cloneDir := filepath.Join(env.AVDHome, name+".avd")
	ud := filepath.Join(cloneDir, "userdata-qemu.img.qcow2")
	if _, err := os.Stat(ud); err != nil {
		ud = filepath.Join(cloneDir, "userdata-qemu.img")
	}
	st, _ := os.Stat(ud)
	return ud, st.Size(), nil
}

func Delete(env Env, name string) error {
	if name == "" {
		return errors.New("empty name")
	}
	_ = os.RemoveAll(filepath.Join(env.AVDHome, name+".avd"))
	_ = os.Remove(filepath.Join(env.AVDHome, name+".ini"))
	return nil
}

func infoOf(env Env, name string) (Info, error) {
	dir := filepath.Join(env.AVDHome, name+".avd")
	ud := filepath.Join(dir, "userdata-qemu.img.qcow2")
	if _, err := os.Stat(ud); err != nil {
		alt := filepath.Join(dir, "userdata-qemu.img")
		if _, err2 := os.Stat(alt); err2 == nil {
			ud = alt
		} else {
			ud = filepath.Join(dir, "userdata.img")
		}
	}
	var sz int64
	if st, err := os.Stat(ud); err == nil {
		sz = st.Size()
	}
	return Info{Name: name, Path: dir, Userdata: ud, SizeBytes: sz}, nil
}

// ensureADB starts adb server (idempotent).
func ensureADB(env Env) { _ = exec.Command(env.ADB, "start-server").Run() }

// StartEmulatorOnPort starts emulator with a fixed port and returns (*exec.Cmd, serial, logPath).
func StartEmulatorOnPort(env Env, name string, port int, extraArgs ...string) (*exec.Cmd, string, string, error) {
	_, span := startSpan(
		env,
		"avd.StartEmulatorOnPort",
		attribute.String("name", name),
		attribute.Int("port", port),
	)
	defer span.End()
	logEvent(env, "emulator start requested", "name", name, "port", port)
	// emulator uses a pair: <port> and <port+1>; must be even
	if port%2 != 0 {
		err := fmt.Errorf("port %d is odd; emulator requires even port numbers (uses port and port+1)", port)
		recordSpanError(span, err)
		return nil, "", "", err
	}
	if port < 5554 || port > 5800 {
		err := fmt.Errorf("port %d out of valid range (5554-5800)", port)
		recordSpanError(span, err)
		return nil, "", "", err
	}

	// Check if port is already in use (with retry for TIME_WAIT sockets)
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if isPortFree(port) && isPortFree(port+1) {
			break
		}
		if attempt < maxRetries-1 {
			time.Sleep(2 * time.Second)
		} else {
			err := fmt.Errorf(
				"port %d or %d still in use after %d retries (may be in TIME_WAIT state)",
				port,
				port+1,
				maxRetries,
			)
			recordSpanError(span, err)
			return nil, "", "", err
		}
	}

	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("emulator-%s-%d.log", name, port))
	logFile, err := os.Create(logPath)
	if err != nil {
		recordSpanError(span, err)
		return nil, "", "", fmt.Errorf("open log: %w", err)
	}
	logWriter := newLineLogWriter(
		env,
		"name",
		name,
		"port",
		port,
		"log_path",
		logPath,
	)

	args := []string{
		"-avd", name,
		"-port", fmt.Sprint(port),
		"-no-window",
		"-no-boot-anim",
		"-no-snapshot",
		"-no-snapshot-load",
		"-no-snapshot-save",
		"-skip-adb-auth",
		"-no-metrics",
		"-no-location-ui",
		"-no-audio",
		"-read-only",
		"-gpu", "swiftshader_indirect",
		"-logcat", "*:S",
	}

	args = append(args, extraArgs...)
	cmd := exec.Command(env.Emulator, args...)
	cmd.Stdout = io.MultiWriter(logFile, logWriter)
	cmd.Stderr = io.MultiWriter(logFile, logWriter)
	cmd.Env = append(os.Environ(), "QEMU_FILE_LOCKING=off", "ADB_VENDOR_KEYS=/dev/null")

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		recordSpanError(span, err)
		logEvent(
			env,
			"emulator start failed",
			"name",
			name,
			"port",
			port,
			"error",
			err,
			"log_path",
			logPath,
		)
		return nil, "", "", fmt.Errorf("emulator start: %w", err)
	}
	serial := fmt.Sprintf("emulator-%d", port)
	span.SetAttributes(
		attribute.String("serial", serial),
		attribute.Int("pid", cmd.Process.Pid),
		attribute.String("log_path", logPath),
	)
	logEvent(
		env,
		"emulator started",
		"name",
		name,
		"port",
		port,
		"serial",
		serial,
		"pid",
		cmd.Process.Pid,
		"log_path",
		logPath,
	)
	return cmd, serial, logPath, nil
}

// waitForEmulatorSerial polls adb devices for a specific serial.
func waitForEmulatorSerial(env Env, serial string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var buf bytes.Buffer
		c := exec.Command(env.ADB, "devices")
		c.Stdout = &buf
		_ = c.Run()
		for _, line := range strings.Split(buf.String(), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && f[0] == serial {
				return nil // seen (status can be 'device' or 'offline'; WaitForBoot will handle readiness)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("device %s not seen within %s", serial, timeout)
}

// FindFreeEvenPort returns the first free even port in [start, end) (emulator uses port and port+1).
func FindFreeEvenPort(start, end int) (int, error) {
	if start%2 != 0 {
		start++
	}
	for p := start; p < end; p += 2 {
		l1, err1 := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err1 != nil {
			continue
		}
		l2, err2 := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+1))
		if err2 != nil {
			_ = l1.Close()
			continue
		}
		_ = l1.Close()
		_ = l2.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free even port found in %d..%d", start, end)
}

// GetAVDNameFromSerial asks the emulator console for the AVD name.
func GetAVDNameFromSerial(env Env, serial string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command(env.ADB, "-s", serial, "emu", "avd", "name")
	cmd.Stdout = &buf
	_ = cmd.Run()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 {
		return "", nil
	}
	if strings.TrimSpace(lines[len(lines)-1]) == "OK" && len(lines) > 1 {
		lines = lines[:len(lines)-1]
	}
	name := strings.TrimSpace(lines[0])
	return name, nil
}

type ProcInfo struct {
	Serial string `json:"serial"`
	Name   string `json:"name"`
	Port   int    `json:"port"`
	PID    int    `json:"pid"`
	Booted bool   `json:"booted"`
}

func ListRunning(env Env) ([]ProcInfo, error) {
	_, span := startSpan(env, "avd.ListRunning")
	defer span.End()
	ensureADB(env)

	var procs []ProcInfo
	seen := make(map[int]bool)

	// Strategy 1: Get emulators from adb devices (may not show all if just started)
	var out bytes.Buffer
	c := exec.Command(env.ADB, "devices")
	c.Stdout = &out
	_ = c.Run()

	for _, line := range strings.Split(out.String(), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.HasPrefix(f[0], "emulator-") {
			serial := f[0]
			port := 0
			if n, err := strconv.Atoi(strings.TrimPrefix(serial, "emulator-")); err == nil {
				port = n
			}
			if port == 0 {
				continue
			}
			seen[port] = true

			// Try to get name from adb, fallback to process cmdline
			name, _ := GetAVDNameFromSerial(env, serial)
			pid := findEmulatorPID(port)
			if name == "" && pid > 0 {
				name = findEmulatorNameFromPID(pid)
			}

			boot := false
			// quick boot check using explicit serial
			var b bytes.Buffer
			cmd := exec.Command(env.ADB, "-s", serial, "shell", "getprop", "sys.boot_completed")
			cmd.Stdout = &b
			_ = cmd.Run()
			if strings.TrimSpace(b.String()) == "1" {
				boot = true
			}
			procs = append(procs, ProcInfo{Serial: serial, Name: name, Port: port, PID: pid, Booted: boot})
		}
	}

	// Strategy 2: Scan for running qemu processes that adb missed
	// This catches emulators that just started and haven't registered with adb yet
	// Scan the full range that emulators typically use
	for port := 5554; port <= 5800; port += 2 {
		if seen[port] {
			continue
		}
		pid := findEmulatorPID(port)
		if pid > 0 {
			// Found a running emulator on this port
			serial := fmt.Sprintf("emulator-%d", port)
			// Try to get name from adb, fallback to process cmdline
			name, _ := GetAVDNameFromSerial(env, serial)
			if name == "" {
				name = findEmulatorNameFromPID(pid)
			}

			// Try to check boot status
			boot := false
			var b bytes.Buffer
			cmd := exec.Command(env.ADB, "-s", serial, "shell", "getprop", "sys.boot_completed")
			cmd.Stdout = &b
			cmd.Stderr = &bytes.Buffer{} // discard errors
			if cmd.Run() == nil && strings.TrimSpace(b.String()) == "1" {
				boot = true
			}

			procs = append(procs, ProcInfo{Serial: serial, Name: name, Port: port, PID: pid, Booted: boot})
		}
	}

	return procs, nil
}

// findEmulatorPID best-effort: parse `ps` for qemu-system or emulator on the given port.
func findEmulatorPID(port int) int {
	// Linux-only, best effort: look for "-port <port>" in process cmdline.
	bs, err := os.ReadFile("/proc/self/mounts")
	_ = bs
	_ = err // keep import happy on non-Linux builds
	entries, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, p := range entries {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		// Must contain both "-port <port>" AND "qemu-system" or "emulator" (not docker-proxy!)
		if bytes.Contains(b, []byte(fmt.Sprintf("-port%c%d", 0, port))) &&
			(bytes.Contains(b, []byte("qemu-system")) || bytes.Contains(b, []byte("emulator"))) {
			// extract PID from path /proc/<pid>/cmdline
			base := filepath.Base(filepath.Dir(p))
			if n, err := strconv.Atoi(base); err == nil {
				// Verify this PID actually exists and is running
				if _, statErr := os.Stat(filepath.Join("/proc", base, "stat")); statErr == nil {
					return n
				}
			}
		}
	}
	return 0
}

// findEmulatorNameFromPID extracts AVD name from process cmdline
func findEmulatorNameFromPID(pid int) string {
	if pid == 0 {
		return ""
	}
	cmdlinePath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	b, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return ""
	}
	// cmdline is null-separated: [emulator, -avd, name, -port, ...]
	parts := bytes.Split(b, []byte{0})
	for i, part := range parts {
		if string(part) == "-avd" && i+1 < len(parts) {
			return string(parts[i+1])
		}
	}
	return ""
}

// isPortFree checks if a TCP port is available
func isPortFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// createSDCard creates an sdcard.img file based on config.ini sdcard.size setting
func createSDCard(env Env, avdDir, configPath string) error {
	// Read config.ini to get sdcard.size
	cfgBytes, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	// Parse sdcard.size (e.g., "512 MB", "512M", "1G")
	var sdcardSize string
	for _, line := range strings.Split(string(cfgBytes), "\n") {
		if strings.HasPrefix(line, "sdcard.size=") {
			sdcardSize = strings.TrimSpace(strings.TrimPrefix(line, "sdcard.size="))
			break
		}
	}

	// Default to 512MB if not specified
	if sdcardSize == "" {
		sdcardSize = "512M"
	}

	// Normalize size format: remove spaces, ensure minimum 9M
	// "512 MB" -> "512M", "1 GB" -> "1G"
	sdcardSize = strings.ReplaceAll(sdcardSize, " ", "")
	sdcardSize = strings.ToUpper(sdcardSize)

	// Ensure minimum 512M (mksdcard requires at least 9M)
	if !strings.Contains(sdcardSize, "M") && !strings.Contains(sdcardSize, "G") {
		sdcardSize = "512M"
	}

	// Create sdcard using mksdcard tool
	sdcardPath := filepath.Join(avdDir, "sdcard.img")
	mksdcard := filepath.Join(env.SDKRoot, "emulator", "mksdcard")

	// Try mksdcard first (preferred)
	if _, err := os.Stat(mksdcard); err == nil {
		if err := run(mksdcard, sdcardSize, sdcardPath); err != nil {
			return fmt.Errorf("mksdcard failed: %w", err)
		}
		return nil
	}

	// Fallback: create empty file with qemu-img
	return run(env.QemuImg, "create", "-f", "raw", sdcardPath, sdcardSize)
}

// CustomizeStart prepares AVD for manual customization and starts GUI emulator without snapshots.
// Returns path to emulator log file.
func CustomizeStart(env Env, name string) (string, error) {
	if name == "" {
		return "", errors.New("empty name")
	}
	avdDir := filepath.Join(env.AVDHome, name+".avd")
	cfg := filepath.Join(avdDir, "config.ini")
	b, err := os.ReadFile(cfg)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	if err := os.WriteFile(cfg, sanitizeConfigINI(b), 0o644); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	_ = os.RemoveAll(filepath.Join(avdDir, "snapshots"))

	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("emulator-%s-customize.log", name))
	lf, err := os.Create(logPath)
	if err != nil {
		return "", fmt.Errorf("open log: %w", err)
	}
	args := []string{"-avd", name, "-no-snapshot-load", "-no-snapshot-save"}
	cmd := exec.Command(env.Emulator, args...)
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.Env = append(os.Environ(), "QEMU_FILE_LOCKING=off")
	if err := cmd.Start(); err != nil {
		_ = lf.Close()
		return "", fmt.Errorf("emulator start: %w", err)
	}
	return logPath, nil
}

// CustomizeFinish stops the emulator (if running) and exports userdata to a golden qcow2.
func CustomizeFinish(env Env, name, dest string) (string, int64, error) {
	if name == "" {
		return "", 0, errors.New("empty name")
	}
	if procs, err := ListRunning(env); err == nil {
		for _, p := range procs {
			if p.Name == name {
				KillEmulator(env, p.Serial)
				time.Sleep(1 * time.Second)
				break
			}
		}
	}
	if dest == "" {
		dir := env.GoldenDir
		_ = os.MkdirAll(dir, 0o755)
		dest = filepath.Join(dir, fmt.Sprintf("%s-custom.qcow2", name))
	}
	return SaveGolden(env, name, dest)
}

// Stop by serial (clean). Falls back to SIGTERM if adb fails.
func StopBySerial(env Env, serial string) error {
	if !strings.HasPrefix(serial, "emulator-") {
		return fmt.Errorf("invalid serial format: %s (expected emulator-XXXX)", serial)
	}

	// Extract port from serial
	port := 0
	if n, err := strconv.Atoi(strings.TrimPrefix(serial, "emulator-")); err == nil {
		port = n
	}
	_, span := startSpan(
		env,
		"avd.StopBySerial",
		attribute.String("serial", serial),
		attribute.Int("port", port),
	)
	defer span.End()
	logEvent(env, "emulator stop requested", "serial", serial, "port", port)

	// Try graceful shutdown via adb first
	cmd := exec.Command(env.ADB, "-s", serial, "emu", "kill")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	adbErr := cmd.Run()

	// Wait a moment to see if it worked
	time.Sleep(1 * time.Second)

	// Check if process is still running
	pid := findEmulatorPID(port)
	if pid == 0 {
		// Successfully stopped
		span.SetAttributes(attribute.Bool("stopped", true))
		logEvent(env, "emulator stopped", "serial", serial, "port", port)
		return nil
	}

	// ADB kill failed or didn't work, fallback to SIGTERM
	if proc, err := os.FindProcess(pid); err == nil {
		if killErr := proc.Signal(os.Interrupt); killErr == nil {
			// Wait a bit for graceful shutdown
			time.Sleep(2 * time.Second)
			// Check if still running
			if findEmulatorPID(port) > 0 {
				// Force kill
				_ = proc.Kill()
			}
			span.SetAttributes(attribute.Bool("stopped", true))
			logEvent(env, "emulator stopped", "serial", serial, "port", port, "pid", pid)
			return nil
		}
	}

	// If we got here, adb failed and we couldn't kill the process
	if adbErr != nil {
		recordSpanError(span, adbErr)
		logEvent(env, "emulator stop failed", "serial", serial, "port", port, "pid", pid, "error", adbErr)
		return fmt.Errorf("failed to stop %s via adb: %w\nADB error: %s\nAlso failed to kill PID %d",
			serial, adbErr, errBuf.String(), pid)
	}

	return nil
}
