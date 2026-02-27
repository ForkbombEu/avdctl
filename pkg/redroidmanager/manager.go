// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

// Package redroidmanager provides a Go library for managing Redroid containers.
package redroidmanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Manager provides Redroid lifecycle operations.
type Manager struct {
	env Environment
}

// Environment defines binaries and optional SSH transport.
type Environment struct {
	DockerBin string
	ADBBin    string
	TarBin    string

	SSHTarget string
	SSHBin    string
	SSHArgs   []string

	Context context.Context
}

// StartOptions configures container start behavior.
type StartOptions struct {
	Name    string
	Image   string
	DataDir string
	DataTar string

	HostPort int

	ShmSize string
	Memory  string
	CPUs    string

	BinderFS string

	Width  int
	Height int
	DPI    int
}

// WaitOptions configures readiness checks.
type WaitOptions struct {
	Serial       string
	Timeout      time.Duration
	PollInterval time.Duration
}

// New creates a manager with defaults and env-based SSH settings.
func New() *Manager {
	return NewWithEnv(Environment{
		DockerBin: "docker",
		ADBBin:    "adb",
		TarBin:    "tar",
		SSHTarget: os.Getenv("AVDCTL_SSH_TARGET"),
		SSHBin:    getenv("AVDCTL_SSH_BIN", "ssh"),
		SSHArgs:   strings.Fields(os.Getenv("AVDCTL_SSH_ARGS")),
		Context:   context.Background(),
	})
}

// NewWithEnv creates a manager with explicit configuration.
func NewWithEnv(env Environment) *Manager {
	if strings.TrimSpace(env.DockerBin) == "" {
		env.DockerBin = "docker"
	}
	if strings.TrimSpace(env.ADBBin) == "" {
		env.ADBBin = "adb"
	}
	if strings.TrimSpace(env.TarBin) == "" {
		env.TarBin = "tar"
	}
	if strings.TrimSpace(env.SSHBin) == "" {
		env.SSHBin = "ssh"
	}
	if env.Context == nil {
		env.Context = context.Background()
	}
	return &Manager{env: env}
}

// Start restores data from tar and starts a Redroid container.
func (m *Manager) Start(opts StartOptions) (string, error) {
	if strings.TrimSpace(opts.Name) == "" {
		return "", errors.New("empty container name")
	}
	if strings.TrimSpace(opts.Image) == "" {
		return "", errors.New("empty image")
	}
	if strings.TrimSpace(opts.DataDir) == "" {
		return "", errors.New("empty data directory")
	}
	if strings.TrimSpace(opts.DataTar) == "" {
		return "", errors.New("empty data tar path")
	}
	if opts.HostPort == 0 {
		opts.HostPort = 5555
	}
	if strings.TrimSpace(opts.ShmSize) == "" {
		opts.ShmSize = "3g"
	}
	if strings.TrimSpace(opts.Memory) == "" {
		opts.Memory = "5g"
	}
	if strings.TrimSpace(opts.CPUs) == "" {
		opts.CPUs = "4"
	}
	if strings.TrimSpace(opts.BinderFS) == "" {
		opts.BinderFS = "/dev/binderfs"
	}
	if opts.Width == 0 {
		opts.Width = 1080
	}
	if opts.Height == 0 {
		opts.Height = 2400
	}
	if opts.DPI == 0 {
		opts.DPI = 360
	}

	// Equivalent to: rm -rf <dataDir>; tar -C <parent> -xf <dataTar>
	dataParent := filepath.Dir(opts.DataDir)
	if err := m.run("mkdir", "-p", dataParent); err != nil {
		return "", err
	}
	if err := m.run("rm", "-rf", opts.DataDir); err != nil {
		return "", err
	}
	if err := m.run(m.env.TarBin, "--numeric-owner", "--xattrs", "--acls", "-C", dataParent, "-xf", opts.DataTar); err != nil {
		return "", err
	}

	// Best-effort cleanup if already present.
	_, _ = m.runOutput(m.env.DockerBin, "rm", "-f", opts.Name)

	args := []string{
		"run", "-d",
		"--name", opts.Name,
		"--privileged",
		"--shm-size=" + opts.ShmSize,
		"--memory=" + opts.Memory,
		"--cpus=" + opts.CPUs,
		"-v", opts.BinderFS + ":/dev/binderfs",
		"-v", opts.DataDir + ":/data",
		"-p", fmt.Sprintf("%d:5555", opts.HostPort),
		opts.Image,
		"androidboot.use_memfd=1",
		"androidboot.hardware=redroid",
		fmt.Sprintf("androidboot.redroid_width=%d", opts.Width),
		fmt.Sprintf("androidboot.redroid_height=%d", opts.Height),
		fmt.Sprintf("androidboot.redroid_dpi=%d", opts.DPI),
	}

	out, err := m.runOutput(m.env.DockerBin, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// WaitForBoot waits until framework services are available.
func (m *Manager) WaitForBoot(opts WaitOptions) error {
	if strings.TrimSpace(opts.Serial) == "" {
		opts.Serial = "127.0.0.1:5555"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 180 * time.Second
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}

	_ = m.run(m.env.ADBBin, "start-server")
	_, _ = m.runOutput(m.env.ADBBin, "connect", opts.Serial)
	if err := m.run(m.env.ADBBin, "-s", opts.Serial, "wait-for-device"); err != nil {
		return err
	}

	deadline := time.Now().Add(opts.Timeout)
	for time.Now().Before(deadline) {
		ss, _ := m.adbShell(opts.Serial, "getprop", "init.svc.system_server")
		bc, _ := m.adbShell(opts.Serial, "getprop", "sys.boot_completed")
		pkg, _ := m.adbShell(opts.Serial, "service", "check", "package")
		act, _ := m.adbShell(opts.Serial, "service", "check", "activity")

		if strings.TrimSpace(bc) == "1" &&
			strings.Contains(strings.ToLower(pkg), "found") &&
			strings.Contains(strings.ToLower(act), "found") &&
			strings.TrimSpace(ss) != "" {
			return nil
		}
		time.Sleep(opts.PollInterval)
	}

	diag, _ := m.adbShell(opts.Serial, "getprop")
	return fmt.Errorf("timed out waiting for redroid boot on %s\nDiagnostics:\n%s", opts.Serial, diag)
}

// Stop stops a running Redroid container by name.
func (m *Manager) Stop(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("empty container name")
	}
	return m.run(m.env.DockerBin, "stop", name)
}

// Delete force-removes a Redroid container by name.
func (m *Manager) Delete(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("empty container name")
	}
	return m.run(m.env.DockerBin, "rm", "-f", name)
}

func (m *Manager) adbShell(serial string, args ...string) (string, error) {
	base := []string{"-s", serial, "shell"}
	base = append(base, args...)
	out, err := m.runOutput(m.env.ADBBin, base...)
	return strings.TrimSpace(strings.ReplaceAll(out, "\r", "")), err
}

func (m *Manager) run(bin string, args ...string) error {
	_, err := m.runOutput(bin, args...)
	return err
}

func (m *Manager) runOutput(bin string, args ...string) (string, error) {
	ctx := m.env.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := m.commandContext(ctx, bin, args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %v failed: %w\n%s", bin, args, err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

func (m *Manager) commandContext(ctx context.Context, bin string, args ...string) *exec.Cmd {
	if strings.TrimSpace(m.env.SSHTarget) == "" {
		return exec.CommandContext(ctx, bin, args...)
	}
	cmdArgs := append([]string{bin}, args...)
	remote := "sh -lc " + shellQuote(shellJoin(cmdArgs))
	sshArgs := append([]string{}, m.env.SSHArgs...)
	sshArgs = append(sshArgs, m.env.SSHTarget, remote)
	return exec.CommandContext(ctx, m.env.SSHBin, sshArgs...)
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
