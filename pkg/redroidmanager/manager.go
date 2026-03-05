// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

// Package redroidmanager provides a Go library for managing Redroid containers.
package redroidmanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/go-units"
	"github.com/forkbombeu/avdctl/internal/remoteavdctl"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// Manager provides Redroid lifecycle operations.
type Manager struct {
	env    Environment
	docker dockerClient
}

// Environment defines binaries and optional SSH transport.
type Environment struct {
	DockerHost string
	ADBBin     string
	TarBin     string
	SudoBin    string
	Sudo       bool
	SudoPass   string

	SSHTarget string
	SSHArgs   []string

	Context context.Context
}

type dockerRunOptions struct {
	Name     string
	Image    string
	DataDir  string
	HostPort int
	ShmSize  string
	Memory   string
	CPUs     string
	BinderFS string
	Width    int
	Height   int
	DPI      int
}

type dockerClient interface {
	RemoveContainer(ctx context.Context, name string, force bool) error
	RunContainer(ctx context.Context, opts dockerRunOptions) (string, error)
	StopContainer(ctx context.Context, name string) error
	PullImage(ctx context.Context, image string) error
}

type dockerSDKClient struct {
	cli *client.Client
}

// StartOptions configures container start behavior.
type StartOptions struct {
	Name    string
	Image   string
	DataDir string
	DataTar string
	UseSudo bool

	HostPort int

	ShmSize string
	Memory  string
	CPUs    string

	BinderFS string

	Width  int
	Height int
	DPI    int
}

// InitOptions configures redroid asset initialization.
type InitOptions struct {
	Image      string
	DataTarURL string
	DataTar    string
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
		DockerHost: strings.TrimSpace(os.Getenv("DOCKER_HOST")),
		ADBBin:     "adb",
		TarBin:     "tar",
		SudoBin:    strings.TrimSpace(os.Getenv("AVDCTL_SUDO_BIN")),
		Sudo:       getenvBool("AVDCTL_SUDO"),
		SudoPass:   os.Getenv("AVDCTL_SUDO_PASSWORD"),
		SSHTarget:  os.Getenv("AVDCTL_SSH_TARGET"),
		SSHArgs:    strings.Fields(os.Getenv("AVDCTL_SSH_ARGS")),
		Context:    context.Background(),
	})
}

// NewWithEnv creates a manager with explicit configuration.
func NewWithEnv(env Environment) *Manager {
	if strings.TrimSpace(env.ADBBin) == "" {
		env.ADBBin = "adb"
	}
	if strings.TrimSpace(env.TarBin) == "" {
		env.TarBin = "tar"
	}
	if strings.TrimSpace(env.SudoBin) == "" {
		env.SudoBin = "sudo"
	}
	if env.Context == nil {
		env.Context = context.Background()
	}
	return &Manager{
		env:    env,
		docker: newDockerSDKClient(env),
	}
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
	useSudo := opts.UseSudo || m.env.Sudo
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		args := []string{
			"redroid", "start",
			"--name", opts.Name,
			"--image", opts.Image,
			"--data-dir", opts.DataDir,
			"--data-tar", opts.DataTar,
			"--port", strconv.Itoa(opts.HostPort),
			"--shm-size", opts.ShmSize,
			"--memory", opts.Memory,
			"--cpus", opts.CPUs,
			"--binderfs", opts.BinderFS,
			"--width", strconv.Itoa(opts.Width),
			"--height", strconv.Itoa(opts.Height),
			"--dpi", strconv.Itoa(opts.DPI),
		}
		if useSudo {
			args = append(args, "--sudo")
		}
		out, err := m.runRemote(args...)
		if err != nil {
			return "", err
		}
		openIdx := strings.LastIndex(out, "(")
		closeIdx := strings.LastIndex(out, ")")
		if openIdx >= 0 && closeIdx > openIdx {
			return strings.TrimSpace(out[openIdx+1 : closeIdx]), nil
		}
		return "", nil
	}

	// Equivalent to: rm -rf <dataDir>; tar -C <parent> -xf <dataTar>
	dataParent := filepath.Dir(opts.DataDir)
	run := m.run
	if useSudo {
		run = m.runSudo
	}
	if err := run("mkdir", "-p", dataParent); err != nil {
		return "", err
	}
	if err := run("rm", "-rf", opts.DataDir); err != nil {
		return "", err
	}
	if err := run(m.env.TarBin, "--numeric-owner", "--xattrs", "--acls", "-C", dataParent, "-xf", opts.DataTar); err != nil {
		return "", err
	}

	// Best-effort cleanup if already present.
	_ = m.docker.RemoveContainer(m.context(), opts.Name, true)

	return m.docker.RunContainer(m.context(), dockerRunOptions{
		Name:     opts.Name,
		Image:    opts.Image,
		DataDir:  opts.DataDir,
		HostPort: opts.HostPort,
		ShmSize:  opts.ShmSize,
		Memory:   opts.Memory,
		CPUs:     opts.CPUs,
		BinderFS: opts.BinderFS,
		Width:    opts.Width,
		Height:   opts.Height,
		DPI:      opts.DPI,
	})
}

// Init pulls the configured redroid image and downloads the data-tar.
func (m *Manager) Init(opts InitOptions) error {
	if strings.TrimSpace(opts.Image) == "" {
		return errors.New("empty image")
	}
	if strings.TrimSpace(opts.DataTarURL) == "" {
		return errors.New("empty data tar url")
	}
	if strings.TrimSpace(opts.DataTar) == "" {
		return errors.New("empty data tar path")
	}

	if err := m.PullImage(opts.Image); err != nil {
		return fmt.Errorf("pull redroid image: %w", err)
	}
	if err := m.downloadDataTar(opts.DataTarURL, opts.DataTar); err != nil {
		return fmt.Errorf("download data tar: %w", err)
	}
	return nil
}

// PullImage ensures a redroid docker image is present locally.
func (m *Manager) PullImage(image string) error {
	if strings.TrimSpace(image) == "" {
		return errors.New("empty image")
	}
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		return errors.New("redroid image pull over SSH is not implemented")
	}
	return m.docker.PullImage(m.context(), image)
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
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		_, err := m.runRemote(
			"redroid", "wait",
			"--serial", opts.Serial,
			"--timeout", opts.Timeout.String(),
			"--poll", opts.PollInterval.String(),
		)
		return err
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
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		_, err := m.runRemote("redroid", "stop", "--name", name)
		return err
	}
	return m.docker.StopContainer(m.context(), name)
}

// Delete force-removes a Redroid container by name.
func (m *Manager) Delete(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("empty container name")
	}
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		_, err := m.runRemote("redroid", "delete", "--name", name)
		return err
	}
	return m.docker.RemoveContainer(m.context(), name, true)
}

func (m *Manager) context() context.Context {
	if m.env.Context == nil {
		return context.Background()
	}
	return m.env.Context
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

func (m *Manager) runSudo(bin string, args ...string) error {
	_, err := m.runOutputSudo(bin, args...)
	return err
}

func (m *Manager) runOutput(bin string, args ...string) (string, error) {
	return m.runOutputWithInput(nil, bin, args...)
}

func (m *Manager) runOutputSudo(bin string, args ...string) (string, error) {
	sudoArgs := make([]string, 0, len(args)+5)
	var stdin io.Reader
	if m.env.SudoPass != "" {
		sudoArgs = append(sudoArgs, "-S", "-p", "")
		stdin = strings.NewReader(m.env.SudoPass + "\n")
	} else {
		sudoArgs = append(sudoArgs, "-n")
	}
	sudoArgs = append(sudoArgs, bin)
	sudoArgs = append(sudoArgs, args...)
	return m.runOutputWithInput(stdin, m.env.SudoBin, sudoArgs...)
}

func (m *Manager) runOutputWithInput(stdin io.Reader, bin string, args ...string) (string, error) {
	ctx := m.env.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := m.commandContext(ctx, bin, args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %v failed: %w\n%s", bin, args, err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

func (m *Manager) commandContext(ctx context.Context, bin string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, bin, args...)
}

func (m *Manager) runRemote(args ...string) (string, error) {
	ctx := m.context()
	out, errOut, err := remoteavdctl.RunOutput(ctx, m.env.SSHTarget, m.env.SSHArgs, args)
	if err != nil {
		return "", fmt.Errorf("remote avdctl %v failed: %w\n%s", args, err, strings.TrimSpace(errOut))
	}
	return strings.TrimSpace(out), nil
}

func (m *Manager) downloadDataTar(url, dst string) error {
	ctx := m.context()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func newDockerSDKClient(env Environment) dockerClient {
	opts := []client.Opt{
		client.FromEnv,
	}
	if host := dockerHostFromEnv(env); host != "" {
		opts = append(opts, client.WithHost(host))
	}

	cli, err := client.New(opts...)
	if err != nil {
		return &dockerClientInitError{err: err}
	}
	return &dockerSDKClient{cli: cli}
}

func dockerHostFromEnv(env Environment) string {
	if strings.TrimSpace(env.DockerHost) != "" {
		return strings.TrimSpace(env.DockerHost)
	}
	if strings.TrimSpace(env.SSHTarget) != "" {
		return "ssh://" + strings.TrimSpace(env.SSHTarget)
	}
	return ""
}

func getenvBool(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	switch strings.ToLower(v) {
	case "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseMemoryBytes(value string) (int64, error) {
	bytesValue, err := units.RAMInBytes(value)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", value, err)
	}
	return bytesValue, nil
}

func parseCPUNano(value string) (int64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cpus value %q: %w", value, err)
	}
	if f <= 0 {
		return 0, fmt.Errorf("invalid cpus value %q: must be > 0", value)
	}
	return int64(f * 1_000_000_000), nil
}

func (d *dockerSDKClient) RemoveContainer(ctx context.Context, name string, force bool) error {
	if d.cli == nil {
		return errors.New("docker client not initialized")
	}
	_, err := d.cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: force})
	if err != nil && errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

func (d *dockerSDKClient) StopContainer(ctx context.Context, name string) error {
	if d.cli == nil {
		return errors.New("docker client not initialized")
	}
	timeout := 15
	_, err := d.cli.ContainerStop(ctx, name, client.ContainerStopOptions{Timeout: &timeout})
	return err
}

func (d *dockerSDKClient) PullImage(ctx context.Context, image string) error {
	if d.cli == nil {
		return errors.New("docker client not initialized")
	}
	rc, err := d.cli.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}

func (d *dockerSDKClient) RunContainer(ctx context.Context, opts dockerRunOptions) (string, error) {
	if d.cli == nil {
		return "", errors.New("docker client not initialized")
	}
	shmSize, err := parseMemoryBytes(opts.ShmSize)
	if err != nil {
		return "", err
	}
	memory, err := parseMemoryBytes(opts.Memory)
	if err != nil {
		return "", err
	}
	nanoCPUs, err := parseCPUNano(opts.CPUs)
	if err != nil {
		return "", err
	}

	portKey, err := network.ParsePort("5555/tcp")
	if err != nil {
		return "", fmt.Errorf("invalid container port: %w", err)
	}
	hostConfig := &container.HostConfig{
		Privileged: true,
		ShmSize:    shmSize,
		Resources: container.Resources{
			Memory:   memory,
			NanoCPUs: nanoCPUs,
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: opts.BinderFS,
				Target: "/dev/binderfs",
			},
			{
				Type:   mount.TypeBind,
				Source: opts.DataDir,
				Target: "/data",
			},
		},
		PortBindings: network.PortMap{
			portKey: []network.PortBinding{{HostPort: strconv.Itoa(opts.HostPort)}},
		},
	}

	cfg := &container.Config{
		Image: opts.Image,
		Cmd: []string{
			"androidboot.use_memfd=1",
			"androidboot.hardware=redroid",
			fmt.Sprintf("androidboot.redroid_width=%d", opts.Width),
			fmt.Sprintf("androidboot.redroid_height=%d", opts.Height),
			fmt.Sprintf("androidboot.redroid_dpi=%d", opts.DPI),
		},
		ExposedPorts: network.PortSet{
			portKey: struct{}{},
		},
	}

	resp, err := d.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     cfg,
		HostConfig: hostConfig,
		Name:       opts.Name,
	})
	if err != nil {
		return "", err
	}
	if _, err := d.cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		return "", err
	}
	return resp.ID, nil
}

type dockerClientInitError struct {
	err error
}

func (d *dockerClientInitError) RemoveContainer(_ context.Context, _ string, _ bool) error {
	return d.err
}

func (d *dockerClientInitError) RunContainer(_ context.Context, _ dockerRunOptions) (string, error) {
	return "", d.err
}

func (d *dockerClientInitError) StopContainer(_ context.Context, _ string) error {
	return d.err
}

func (d *dockerClientInitError) PullImage(_ context.Context, _ string) error {
	return d.err
}
