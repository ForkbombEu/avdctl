// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

// Package avdmanager provides a Go library for managing Android Virtual Devices (AVDs)
// with golden image and clone workflows.
package avdmanager

import (
	"context"
	"time"

	"github.com/forkbombeu/avdctl/internal/avd"
)

// Manager provides high-level AVD management operations.
type Manager struct {
	env avd.Env
}

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
	info, err := avd.CloneFromGolden(m.env, opts.BaseName, opts.CloneName, opts.GoldenPath)
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
	return avd.RunAVD(m.env, opts.Name)
}

// RunOnPort starts an emulator instance on a specific port.
func (m *Manager) RunOnPort(opts RunOptions) (serial string, logPath string, err error) {
	if opts.Port == 0 {
		serial, err := avd.RunAVD(m.env, opts.Name)
		return serial, "", err
	}
	_, serial, logPath, err = avd.StartEmulatorOnPort(m.env, opts.Name, opts.Port)
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
	return avd.StopBySerial(m.env, serial)
}

// StopByName stops a running emulator by AVD name.
func (m *Manager) StopByName(name string) error {
	procs, err := avd.ListRunning(m.env)
	if err != nil {
		return err
	}
	for _, p := range procs {
		if p.Name == name {
			return avd.StopBySerial(m.env, p.Serial)
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
	return avd.WaitForBoot(m.env, serial, timeout)
}

// FindFreePort finds a free even port pair for emulator (uses port and port+1).
func (m *Manager) FindFreePort(start, end int) (int, error) {
	return avd.FindFreeEvenPort(start, end)
}
