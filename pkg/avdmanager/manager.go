// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

// Package avdmanager provides a Go library for managing Android Virtual Devices (AVDs)
// with golden image and clone workflows.
package avdmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/forkbombeu/avdctl/internal/avd"
	"github.com/forkbombeu/avdctl/internal/remoteavdctl"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Manager provides high-level AVD management operations.
type Manager struct {
	env avd.Env
}

var managerTracer = otel.Tracer("avdctl/manager")

// New creates a new AVD Manager with auto-detected environment.
func New() *Manager {
	return &Manager{
		env: avd.Detect(),
	}
}

// NewWithCorrelationID creates a new AVD Manager with a correlation ID for structured logs.
func NewWithCorrelationID(correlationID string) *Manager {
	return NewWithContextAndCorrelationID(context.Background(), correlationID)
}

// NewWithContext creates a new AVD Manager with a custom context for tracing.
func NewWithContext(ctx context.Context) *Manager {
	return NewWithContextAndCorrelationID(ctx, "")
}

// NewWithContextAndCorrelationID creates a new AVD Manager with a custom context and correlation ID.
func NewWithContextAndCorrelationID(ctx context.Context, correlationID string) *Manager {
	env := avd.Detect()
	if ctx == nil {
		ctx = context.Background()
	}
	env.Context = ctx
	env.CorrelationID = correlationID
	return &Manager{
		env: env,
	}
}

// NewWithEnv creates a new AVD Manager with custom environment configuration.
func NewWithEnv(env Environment) *Manager {
	ctx := env.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return &Manager{
		env: avd.Env{
			SDKRoot:       env.SDKRoot,
			AVDHome:       env.AVDHome,
			GoldenDir:     env.GoldenDir,
			ClonesDir:     env.ClonesDir,
			ConfigTpl:     env.ConfigTemplate,
			Emulator:      env.EmulatorBin,
			ADB:           env.ADBBin,
			AvdMgr:        env.AvdManagerBin,
			SdkManager:    env.SdkManagerBin,
			QemuImg:       env.QemuImgBin,
			SSHTarget:     env.SSHTarget,
			SSHArgs:       env.SSHArgs,
			CorrelationID: env.CorrelationID,
			Context:       ctx,
		},
	}
}

// Context returns the context bound to this manager.
func (m *Manager) Context() context.Context {
	return m.env.Context
}

// CorrelationID returns the correlation ID configured on this manager.
func (m *Manager) CorrelationID() string {
	return m.env.CorrelationID
}

func (m *Manager) spanContext() context.Context {
	if m.env.Context != nil {
		return m.env.Context
	}
	return context.Background()
}

func (m *Manager) startSpan(name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if m.env.CorrelationID != "" {
		attrs = append(attrs, attribute.String("correlation_id", m.env.CorrelationID))
	}
	return managerTracer.Start(m.spanContext(), name, trace.WithAttributes(attrs...))
}

func (m *Manager) withContext(ctx context.Context) avd.Env {
	env := m.env
	if ctx != nil {
		env.Context = ctx
	}
	return env
}

func (m *Manager) usesRemote() bool {
	return strings.TrimSpace(m.env.SSHTarget) != ""
}

func (m *Manager) ensureNotRunning(name string) error {
	if name == "" {
		return errors.New("empty AVD name")
	}
	procs, err := m.ListRunning()
	if err != nil {
		return err
	}
	for _, proc := range procs {
		if proc.Name == name {
			return fmt.Errorf("AVD %s already running on %s", name, proc.Serial)
		}
	}
	return nil
}

func recordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
}

// Environment holds configuration for AVD tools and paths.
type Environment struct {
	SDKRoot        string          // ANDROID_SDK_ROOT
	AVDHome        string          // ANDROID_AVD_HOME (default ~/.android/avd)
	GoldenDir      string          // Directory for golden QCOW2 images
	ClonesDir      string          // Directory for clones (optional)
	ConfigTemplate string          // Path to config.ini template (optional)
	EmulatorBin    string          // Path to emulator binary (default: "emulator")
	ADBBin         string          // Path to adb binary (default: "adb")
	AvdManagerBin  string          // Path to avdmanager binary (default: "avdmanager")
	SdkManagerBin  string          // Path to sdkmanager binary (default: "sdkmanager")
	QemuImgBin     string          // Path to qemu-img binary (default: "qemu-img")
	SSHTarget      string          // Optional SSH target (user@host) for remote command execution
	SSHArgs        []string        // Optional extra ssh args (e.g. []string{"-i", "~/.ssh/key"})
	CorrelationID  string          // Correlation ID for log enrichment
	Context        context.Context // Context for tracing
}

// BootProgressFunc reports boot progress updates.
type BootProgressFunc func(status string, elapsed time.Duration)

// AVDInfo contains information about an AVD.
type AVDInfo struct {
	Name      string // AVD name
	Path      string // Path to .avd directory
	Userdata  string // Path to userdata file
	SizeBytes int64  // Size of userdata in bytes
}

// ProcessInfo contains information about a running emulator.
type ProcessInfo struct {
	Serial string // Emulator serial (e.g., emulator-5580)
	Name   string // AVD name
	Port   int    // Console port
	PID    int    // Process ID
	Booted bool   // Whether Android has fully booted
}

// InitBaseOptions contains options for creating a base AVD.
type InitBaseOptions struct {
	Name        string // AVD name (required)
	SystemImage string // System image ID (e.g., "system-images;android-35;google_apis_playstore;x86_64")
	Device      string // Device profile (e.g., "pixel_6")
}

// CloneOptions contains options for creating a clone from a golden image.
type CloneOptions struct {
	BaseName   string // Base AVD name (required)
	CloneName  string // New clone name (required)
	GoldenPath string // Path to golden QCOW2 image (required)
}

// RunOptions contains options for running an emulator.
type RunOptions struct {
	Name string // AVD name (required)
	Port int    // Console port (0 = auto-assign)
}

// SaveGoldenOptions contains options for saving a golden image.
type SaveGoldenOptions struct {
	Name        string // AVD name (required)
	Destination string // Destination path for QCOW2 (optional, auto-generated if empty)
}

// PrewarmOptions contains options for prewarming a golden image.
type PrewarmOptions struct {
	Name        string        // AVD name (required)
	Destination string        // Destination path for QCOW2 (optional)
	ExtraSettle time.Duration // Extra time to settle after boot (default: 30s)
	BootTimeout time.Duration // Boot timeout (default: 3m)
}

// BakeAPKOptions contains options for baking APKs into a golden image.
type BakeAPKOptions struct {
	BaseName    string        // Base AVD name (required)
	CloneName   string        // New clone name (required)
	GoldenPath  string        // Base golden QCOW2 path (required)
	APKPaths    []string      // Paths to APKs to install (required)
	Destination string        // Destination path for new golden QCOW2 (optional)
	BootTimeout time.Duration // Boot timeout (default: 3m)
}

// KillAllEmulatorsOptions contains options for gracefully stopping all emulators.
type KillAllEmulatorsOptions struct {
	MaxPasses int           // Maximum termination passes (default: 5)
	Delay     time.Duration // Delay between passes (default: 500ms)
}

// KillAllEmulatorsReport reports the results of the stop-all operation.
type KillAllEmulatorsReport struct {
	Passes        int   // Number of passes executed
	KilledPIDs    []int // Emulator PIDs that were sent SIGTERM (gracefully terminated)
	KilledParents []int // Parent PIDs that were sent SIGKILL (for zombie cleanup)
	Remaining     int   // Remaining emulator processes after all passes
}

// InitBase creates a new base AVD. Auto-installs system image if missing.
func (m *Manager) InitBase(opts InitBaseOptions) (AVDInfo, error) {
	if m.usesRemote() {
		args := []string{"init-base", "--name", opts.Name, "--image", opts.SystemImage, "--device", opts.Device}
		if _, err := m.runRemote(args...); err != nil {
			return AVDInfo{}, err
		}
		return m.findAVDInfo(opts.Name)
	}
	info, err := avd.InitBase(m.env, opts.Name, opts.SystemImage, opts.Device)
	if err != nil {
		return AVDInfo{}, err
	}
	return AVDInfo{
		Name:      info.Name,
		Path:      info.Path,
		Userdata:  info.Userdata,
		SizeBytes: info.SizeBytes,
	}, nil
}

// Clone creates a lightweight clone backed by a golden QCOW2 image.
func (m *Manager) Clone(opts CloneOptions) (AVDInfo, error) {
	ctx, span := m.startSpan(
		"avdmanager.Clone",
		attribute.String("avd_name", opts.CloneName),
	)
	defer span.End()
	if m.usesRemote() {
		_, err := m.runRemote(
			"clone",
			"--base", opts.BaseName,
			"--name", opts.CloneName,
			"--golden", opts.GoldenPath,
		)
		recordSpanError(span, err)
		if err != nil {
			return AVDInfo{}, err
		}
		return m.findAVDInfo(opts.CloneName)
	}
	info, err := avd.CloneFromGolden(m.withContext(ctx), opts.BaseName, opts.CloneName, opts.GoldenPath)
	recordSpanError(span, err)
	if err != nil {
		return AVDInfo{}, err
	}
	return AVDInfo{
		Name:      info.Name,
		Path:      info.Path,
		Userdata:  info.Userdata,
		SizeBytes: info.SizeBytes,
	}, nil
}

// Run starts an emulator instance headless and returns the serial.
func (m *Manager) Run(opts RunOptions) (string, error) {
	ctx, span := m.startSpan(
		"avdmanager.Run",
		attribute.String("avd_name", opts.Name),
	)
	defer span.End()
	if m.usesRemote() {
		if err := m.ensureNotRunning(opts.Name); err != nil {
			recordSpanError(span, err)
			return "", err
		}
		out, err := m.runRemote("run", "--name", opts.Name)
		recordSpanError(span, err)
		if err != nil {
			return "", err
		}
		serial, _, parseErr := parseStartedLine(out)
		recordSpanError(span, parseErr)
		if parseErr != nil {
			return "", parseErr
		}
		span.SetAttributes(attribute.String("serial", serial))
		return serial, nil
	}
	if err := m.ensureNotRunning(opts.Name); err != nil {
		recordSpanError(span, err)
		return "", err
	}
	serial, err := avd.RunAVD(m.withContext(ctx), opts.Name)
	recordSpanError(span, err)
	if err == nil {
		span.SetAttributes(attribute.String("serial", serial))
	}
	return serial, err
}

// RunOnPort starts an emulator instance on a specific port.
func (m *Manager) RunOnPort(opts RunOptions) (serial string, logPath string, err error) {
	ctx, span := m.startSpan(
		"avdmanager.RunOnPort",
		attribute.String("avd_name", opts.Name),
		attribute.Int("port", opts.Port),
	)
	defer span.End()
	if opts.Port == 0 {
		serial, err = m.Run(opts)
		return serial, "", err
	}
	if err := m.ensureNotRunning(opts.Name); err != nil {
		recordSpanError(span, err)
		return "", "", err
	}

	port := opts.Port
	procs, err := m.ListRunning()
	if err != nil {
		recordSpanError(span, err)
		return "", "", err
	}
	for _, proc := range procs {
		if proc.Port == port {
			freePort, err := m.FindFreePort(5554, 5800)
			if err != nil {
				recordSpanError(span, err)
				return "", "", err
			}
			port = freePort
			break
		}
	}
	if m.usesRemote() {
		out, runErr := m.runRemote("run", "--name", opts.Name, "--port", strconv.Itoa(port))
		recordSpanError(span, runErr)
		if runErr != nil {
			return "", "", runErr
		}
		serial, logPath, parseErr := parseStartedLine(out)
		recordSpanError(span, parseErr)
		if parseErr != nil {
			return "", "", parseErr
		}
		span.SetAttributes(attribute.String("serial", serial))
		return serial, logPath, nil
	}

	_, serial, logPath, err = avd.StartEmulatorOnPort(m.withContext(ctx), opts.Name, port)
	recordSpanError(span, err)
	if err == nil {
		span.SetAttributes(attribute.String("serial", serial))
	}
	return serial, logPath, err
}

// List returns all AVDs under ANDROID_AVD_HOME.
func (m *Manager) List() ([]AVDInfo, error) {
	if m.usesRemote() {
		var infos []AVDInfo
		if err := m.runRemoteJSON(&infos, "list", "--json"); err != nil {
			return nil, err
		}
		return infos, nil
	}
	infos, err := avd.List(m.env)
	if err != nil {
		return nil, err
	}
	result := make([]AVDInfo, len(infos))
	for i, info := range infos {
		result[i] = AVDInfo{
			Name:      info.Name,
			Path:      info.Path,
			Userdata:  info.Userdata,
			SizeBytes: info.SizeBytes,
		}
	}
	return result, nil
}

// ListRunning returns all currently running emulator instances.
func (m *Manager) ListRunning() ([]ProcessInfo, error) {
	if m.usesRemote() {
		var procs []ProcessInfo
		if err := m.runRemoteJSON(&procs, "ps", "--json"); err != nil {
			return nil, err
		}
		return procs, nil
	}
	procs, err := avd.ListRunning(m.env)
	if err != nil {
		return nil, err
	}
	result := make([]ProcessInfo, len(procs))
	for i, p := range procs {
		result[i] = ProcessInfo{
			Serial: p.Serial,
			Name:   p.Name,
			Port:   p.Port,
			PID:    p.PID,
			Booted: p.Booted,
		}
	}
	return result, nil
}

// Stop stops a running emulator by serial (e.g., "emulator-5580").
func (m *Manager) Stop(serial string) error {
	ctx, span := m.startSpan(
		"avdmanager.Stop",
		attribute.String("serial", serial),
	)
	defer span.End()
	if m.usesRemote() {
		_, err := m.runRemote("stop", "--serial", serial)
		recordSpanError(span, err)
		return err
	}
	err := avd.StopBySerial(m.withContext(ctx), serial)
	recordSpanError(span, err)
	return err
}

// StopBluetooth disables Bluetooth and scanning on a running emulator by serial.
func (m *Manager) StopBluetooth(serial string) error {
	ctx, span := m.startSpan(
		"avdmanager.StopBluetooth",
		attribute.String("serial", serial),
	)
	defer span.End()
	if m.usesRemote() {
		_, err := m.runRemote("stop-bluetooth", "--serial", serial)
		recordSpanError(span, err)
		return err
	}
	err := avd.StopBluetooth(m.withContext(ctx), serial)
	recordSpanError(span, err)
	return err
}

// StopByName stops a running emulator by AVD name.
func (m *Manager) StopByName(name string) error {
	if m.usesRemote() {
		_, err := m.runRemote("stop", "--name", name)
		return err
	}
	procs, err := avd.ListRunning(m.env)
	if err != nil {
		return err
	}
	for _, p := range procs {
		if p.Name == name {
			return m.Stop(p.Serial)
		}
	}
	return nil // Not running
}

// KillAllEmulators gracefully stops all emulator processes using SIGTERM, retrying until none remain.
func (m *Manager) KillAllEmulators(opts KillAllEmulatorsOptions) (KillAllEmulatorsReport, error) {
	ctx, span := m.startSpan(
		"avdmanager.KillAllEmulators",
		attribute.Int("max_passes", opts.MaxPasses),
		attribute.String("delay", opts.Delay.String()),
	)
	defer span.End()
	if m.usesRemote() {
		if opts.MaxPasses <= 0 {
			opts.MaxPasses = 5
		}
		if opts.Delay <= 0 {
			opts.Delay = 500 * time.Millisecond
		}
		report := KillAllEmulatorsReport{}
		for pass := 0; pass < opts.MaxPasses; pass++ {
			procs, listErr := m.ListRunning()
			if listErr != nil {
				recordSpanError(span, listErr)
				return KillAllEmulatorsReport{}, listErr
			}
			if len(procs) == 0 {
				report.Passes = pass
				report.Remaining = 0
				return report, nil
			}
			report.Passes = pass + 1
			for _, proc := range procs {
				_ = m.Stop(proc.Serial)
				if proc.PID > 0 {
					report.KilledPIDs = append(report.KilledPIDs, proc.PID)
				}
			}
			if pass < opts.MaxPasses-1 {
				time.Sleep(opts.Delay)
			}
		}
		procs, listErr := m.ListRunning()
		if listErr != nil {
			recordSpanError(span, listErr)
			return KillAllEmulatorsReport{}, listErr
		}
		report.Remaining = len(procs)
		return report, nil
	}

	report, err := avd.KillAllEmulators(m.withContext(ctx), opts.MaxPasses, opts.Delay)
	recordSpanError(span, err)
	if err != nil {
		return KillAllEmulatorsReport{}, err
	}
	return KillAllEmulatorsReport{
		Passes:        report.Passes,
		KilledPIDs:    report.KilledPIDs,
		KilledParents: report.KilledParents,
		Remaining:     report.Remaining,
	}, nil
}

// Delete removes an AVD (both .avd directory and .ini file).
func (m *Manager) Delete(name string) error {
	if m.usesRemote() {
		_, err := m.runRemote("delete", name)
		return err
	}
	return avd.Delete(m.env, name)
}

// SaveGolden exports an AVD's userdata to a compressed QCOW2 golden image.
func (m *Manager) SaveGolden(opts SaveGoldenOptions) (path string, sizeBytes int64, err error) {
	if m.usesRemote() {
		args := []string{"save-golden", "--name", opts.Name}
		if strings.TrimSpace(opts.Destination) != "" {
			args = append(args, "--dest", opts.Destination)
		}
		out, runErr := m.runRemote(args...)
		if runErr != nil {
			return "", 0, runErr
		}
		return parsePathAndSize(out, "Golden saved")
	}
	return avd.SaveGolden(m.env, opts.Name, opts.Destination)
}

// Prewarm boots an AVD once, waits for full boot, settles caches, then saves as golden image.
// This is useful for creating a "warmed up" golden image without manual configuration.
func (m *Manager) Prewarm(opts PrewarmOptions) (path string, sizeBytes int64, err error) {
	if opts.ExtraSettle == 0 {
		opts.ExtraSettle = 30 * time.Second
	}
	if opts.BootTimeout == 0 {
		opts.BootTimeout = 3 * time.Minute
	}
	if m.usesRemote() {
		args := []string{
			"prewarm",
			"--name", opts.Name,
			"--extra", opts.ExtraSettle.String(),
			"--timeout", opts.BootTimeout.String(),
		}
		if strings.TrimSpace(opts.Destination) != "" {
			args = append(args, "--dest", opts.Destination)
		}
		out, runErr := m.runRemote(args...)
		if runErr != nil {
			return "", 0, runErr
		}
		return parsePathAndSize(out, "Prewarmed golden saved")
	}
	return avd.PrewarmGolden(m.env, opts.Name, opts.Destination, opts.ExtraSettle, opts.BootTimeout)
}

// BakeAPK creates a clone, boots it, installs APKs, then exports as a new golden image.
func (m *Manager) BakeAPK(opts BakeAPKOptions) (clonePath string, cloneSize int64, err error) {
	if opts.BootTimeout == 0 {
		opts.BootTimeout = 3 * time.Minute
	}
	if m.usesRemote() {
		args := []string{
			"bake-apk",
			"--base", opts.BaseName,
			"--name", opts.CloneName,
			"--golden", opts.GoldenPath,
		}
		for _, apk := range opts.APKPaths {
			args = append(args, "--apk", apk)
		}
		if strings.TrimSpace(opts.Destination) != "" {
			args = append(args, "--dest", opts.Destination)
		}
		out, runErr := m.runRemote(args...)
		if runErr != nil {
			return "", 0, runErr
		}
		return parsePathAndSize(out, "Baked clone at")
	}
	return avd.BakeAPK(m.env, opts.BaseName, opts.CloneName, opts.GoldenPath, opts.APKPaths, opts.BootTimeout)
}

// WaitForBoot waits for an emulator to fully boot Android.
func (m *Manager) WaitForBoot(serial string, timeout time.Duration) error {
	return m.WaitForBootWithProgress(serial, timeout, nil)
}

// WaitForBootWithProgress waits for an emulator to fully boot Android and reports progress.
func (m *Manager) WaitForBootWithProgress(
	serial string,
	timeout time.Duration,
	progress BootProgressFunc,
) error {
	ctx, span := m.startSpan(
		"avdmanager.WaitForBoot",
		attribute.String("serial", serial),
	)
	defer span.End()
	if m.usesRemote() {
		start := time.Now()
		deadline := start.Add(timeout)
		for time.Now().Before(deadline) {
			if progress != nil {
				progress("checking_bootanim", time.Since(start))
			}
			procs, err := m.ListRunning()
			if err == nil {
				for _, proc := range procs {
					if proc.Serial == serial && proc.Booted {
						if progress != nil {
							progress("boot_complete", time.Since(start))
						}
						return nil
					}
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
		return fmt.Errorf("boot timeout after %s (serial %s not ready)", timeout, serial)
	}
	err := avd.WaitForBootWithProgress(
		m.withContext(ctx),
		serial,
		timeout,
		func(status string, elapsed time.Duration) {
			if progress == nil {
				return
			}
			progress(status, elapsed)
		},
	)
	recordSpanError(span, err)
	return err
}

// FindFreePort finds a free even port pair for emulator (uses port and port+1).
func (m *Manager) FindFreePort(start, end int) (int, error) {
	if m.usesRemote() {
		if start%2 != 0 {
			start++
		}
		procs, err := m.ListRunning()
		if err != nil {
			return 0, err
		}
		used := make(map[int]bool, len(procs)*2)
		for _, proc := range procs {
			if proc.Port > 0 {
				used[proc.Port] = true
				used[proc.Port+1] = true
			}
		}
		for p := start; p < end; p += 2 {
			if used[p] || used[p+1] {
				continue
			}
			return p, nil
		}
		return 0, fmt.Errorf("no free even port found in %d..%d", start, end)
	}
	return avd.FindFreeEvenPortWithEnv(m.env, start, end)
}

func (m *Manager) runRemote(args ...string) (string, error) {
	ctx := m.env.Context
	if ctx == nil {
		ctx = context.Background()
	}
	out, errOut, err := remoteavdctl.RunOutput(ctx, m.env.SSHTarget, m.env.SSHArgs, args)
	if err != nil {
		return "", fmt.Errorf("remote avdctl %v failed: %w\n%s", args, err, strings.TrimSpace(errOut))
	}
	return out, nil
}

func (m *Manager) runRemoteJSON(dst any, args ...string) error {
	out, err := m.runRemote(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		return fmt.Errorf("decode remote json output for %v: %w", args, err)
	}
	return nil
}

func (m *Manager) findAVDInfo(name string) (AVDInfo, error) {
	infos, err := m.List()
	if err != nil {
		return AVDInfo{}, err
	}
	for _, info := range infos {
		if info.Name == name {
			return info, nil
		}
	}
	return AVDInfo{}, fmt.Errorf("avd %q not found after command completion", name)
}

var startedLineRe = regexp.MustCompile(`Started\s+\S+\s+on\s+(emulator-\d+)(?:\s+\(log:\s*([^)]+)\))?`)
var sizeSuffixRe = regexp.MustCompile(`\((\d+)\s+bytes\)$`)

func parseStartedLine(out string) (serial string, logPath string, err error) {
	m := startedLineRe.FindStringSubmatch(out)
	if len(m) == 0 {
		return "", "", fmt.Errorf("failed to parse serial from output: %q", strings.TrimSpace(out))
	}
	if len(m) > 1 {
		serial = strings.TrimSpace(m[1])
	}
	if len(m) > 2 {
		logPath = strings.TrimSpace(m[2])
	}
	if serial == "" {
		return "", "", fmt.Errorf("failed to parse serial from output: %q", strings.TrimSpace(out))
	}
	return serial, logPath, nil
}

func parsePathAndSize(out string, prefix string) (path string, sizeBytes int64, err error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		m := sizeSuffixRe.FindStringSubmatch(rest)
		if len(m) != 2 {
			continue
		}
		size, convErr := strconv.ParseInt(strings.TrimSpace(m[1]), 10, 64)
		if convErr != nil {
			return "", 0, convErr
		}
		path := strings.TrimSpace(strings.TrimSpace(rest[:strings.LastIndex(rest, "(")]))
		return path, size, nil
	}
	return "", 0, fmt.Errorf("failed to parse output line with prefix %q: %q", prefix, strings.TrimSpace(out))
}
