// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

// Package avdmanager provides a Go library for managing Android Virtual Devices (AVDs)
// with golden image and clone workflows.
package avdmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/forkbombeu/avdctl/internal/avd"
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

// KillAllEmulatorsOptions contains options for force-stopping all emulators.
type KillAllEmulatorsOptions struct {
	MaxPasses int           // Maximum kill passes (default: 5)
	Delay     time.Duration // Delay between passes (default: 500ms)
}

// KillAllEmulatorsReport reports the results of the kill-all operation.
type KillAllEmulatorsReport struct {
	Passes        int   // Number of passes executed
	KilledPIDs    []int // Emulator PIDs killed
	KilledParents []int // Parent PIDs killed for zombies
	Remaining     int   // Remaining emulator processes after all passes
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

// InitBase creates a new base AVD. Auto-installs system image if missing.
func (m *Manager) InitBase(opts InitBaseOptions) (AVDInfo, error) {
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
		if err := m.ensureNotRunning(opts.Name); err != nil {
			recordSpanError(span, err)
			return "", "", err
		}
		serial, err = avd.RunAVD(m.withContext(ctx), opts.Name)
		recordSpanError(span, err)
		if err == nil {
			span.SetAttributes(attribute.String("serial", serial))
		}
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

	_, serial, logPath, err = avd.StartEmulatorOnPort(m.withContext(ctx), opts.Name, port)
	recordSpanError(span, err)
	if err == nil {
		span.SetAttributes(attribute.String("serial", serial))
	}
	return serial, logPath, err
}

// List returns all AVDs under ANDROID_AVD_HOME.
func (m *Manager) List() ([]AVDInfo, error) {
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
	err := avd.StopBySerial(m.withContext(ctx), serial)
	recordSpanError(span, err)
	return err
}

// StopByName stops a running emulator by AVD name.
func (m *Manager) StopByName(name string) error {
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

// Delete removes an AVD (both .avd directory and .ini file).
func (m *Manager) Delete(name string) error {
	return avd.Delete(m.env, name)
}

// SaveGolden exports an AVD's userdata to a compressed QCOW2 golden image.
func (m *Manager) SaveGolden(opts SaveGoldenOptions) (path string, sizeBytes int64, err error) {
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
	return avd.PrewarmGolden(m.env, opts.Name, opts.Destination, opts.ExtraSettle, opts.BootTimeout)
}

// BakeAPK creates a clone, boots it, installs APKs, then exports as a new golden image.
func (m *Manager) BakeAPK(opts BakeAPKOptions) (clonePath string, cloneSize int64, err error) {
	if opts.BootTimeout == 0 {
		opts.BootTimeout = 3 * time.Minute
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
	return avd.FindFreeEvenPort(start, end)
}
