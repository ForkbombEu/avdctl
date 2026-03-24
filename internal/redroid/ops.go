package redroid

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	"go.opentelemetry.io/otel/attribute"
)

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

type WaitOptions struct {
	Serial       string
	Timeout      time.Duration
	PollInterval time.Duration
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
}

type dockerSDKClient struct {
	cli *client.Client
}

type manager struct {
	env    Env
	docker dockerClient
}

func Start(env Env, opts StartOptions) (string, error) {
	return newManagerWithEnv(env).Start(opts)
}

func WaitForBoot(env Env, opts WaitOptions) error {
	return newManagerWithEnv(env).WaitForBoot(opts)
}

func Stop(env Env, name string) error {
	return newManagerWithEnv(env).Stop(name)
}

func Delete(env Env, name string) error {
	return newManagerWithEnv(env).Delete(name)
}

func newManagerWithEnv(env Env) *manager {
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
	return &manager{
		env:    env,
		docker: newDockerSDKClient(env),
	}
}

func (m *manager) Start(opts StartOptions) (string, error) {
	ctx, span := startSpan(
		m.env,
		"redroid.Start",
		attribute.String("name", opts.Name),
		attribute.String("image", opts.Image),
		attribute.Int("host_port", opts.HostPort),
	)
	defer span.End()
	m.env.Context = ctx
	if strings.TrimSpace(opts.Name) == "" {
		err := errors.New("empty container name")
		recordSpanError(span, err)
		return "", err
	}
	if strings.TrimSpace(opts.Image) == "" {
		err := errors.New("empty image")
		recordSpanError(span, err)
		return "", err
	}
	if strings.TrimSpace(opts.DataDir) == "" {
		err := errors.New("empty data directory")
		recordSpanError(span, err)
		return "", err
	}
	if strings.TrimSpace(opts.DataTar) == "" {
		err := errors.New("empty data tar path")
		recordSpanError(span, err)
		return "", err
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
	span.SetAttributes(attribute.Int("host_port", opts.HostPort))
	logEvent(m.env, "redroid start requested", "name", opts.Name, "image", opts.Image, "host_port", opts.HostPort)

	useSudo := opts.UseSudo || m.env.Sudo
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		args := []string{
			"run", "redroid",
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
			recordSpanError(span, err)
			return "", err
		}
		openIdx := strings.LastIndex(out, "(")
		closeIdx := strings.LastIndex(out, ")")
		if openIdx >= 0 && closeIdx > openIdx {
			containerID := strings.TrimSpace(out[openIdx+1 : closeIdx])
			span.SetAttributes(attribute.String("container_id", containerID))
			logEvent(m.env, "redroid started", "name", opts.Name, "container_id", containerID, "host_port", opts.HostPort)
			return containerID, nil
		}
		logEvent(m.env, "redroid started", "name", opts.Name, "host_port", opts.HostPort)
		return "", nil
	}

	dataParent := filepath.Dir(opts.DataDir)
	run := m.run
	if useSudo {
		run = m.runSudo
	}
	if err := run("mkdir", "-p", dataParent); err != nil {
		recordSpanError(span, err)
		return "", err
	}
	if err := run("rm", "-rf", opts.DataDir); err != nil {
		recordSpanError(span, err)
		return "", err
	}
	if err := run(m.env.TarBin, "--numeric-owner", "--xattrs", "--acls", "-C", dataParent, "-xf", opts.DataTar); err != nil {
		recordSpanError(span, err)
		return "", err
	}

	_ = m.docker.RemoveContainer(m.context(), opts.Name, true)
	containerID, err := m.docker.RunContainer(m.context(), dockerRunOptions{
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
	recordSpanError(span, err)
	if err != nil {
		return "", err
	}
	span.SetAttributes(attribute.String("container_id", containerID))
	logEvent(m.env, "redroid started", "name", opts.Name, "container_id", containerID, "host_port", opts.HostPort)
	return containerID, nil
}

func (m *manager) WaitForBoot(opts WaitOptions) error {
	ctx, span := startSpan(
		m.env,
		"redroid.WaitForBoot",
		attribute.String("serial", opts.Serial),
	)
	defer span.End()
	m.env.Context = ctx
	if strings.TrimSpace(opts.Serial) == "" {
		opts.Serial = "127.0.0.1:5555"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 180 * time.Second
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	span.SetAttributes(
		attribute.String("serial", opts.Serial),
		attribute.String("timeout", opts.Timeout.String()),
		attribute.String("poll_interval", opts.PollInterval.String()),
	)
	logEvent(m.env, "redroid boot wait started", "serial", opts.Serial, "timeout", opts.Timeout.String(), "poll_interval", opts.PollInterval.String())
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		_, err := m.runRemote(
			"run", "redroid",
			"--wait",
			"--serial", opts.Serial,
			"--timeout", opts.Timeout.String(),
			"--poll", opts.PollInterval.String(),
		)
		recordSpanError(span, err)
		if err == nil {
			logEvent(m.env, "redroid boot completed", "serial", opts.Serial)
		}
		return err
	}

	_ = m.run(m.env.ADBBin, "start-server")
	_, _ = m.runOutput(m.env.ADBBin, "connect", opts.Serial)
	if err := m.run(m.env.ADBBin, "-s", opts.Serial, "wait-for-device"); err != nil {
		recordSpanError(span, err)
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
			span.SetAttributes(attribute.Bool("boot_completed", true))
			logEvent(m.env, "redroid boot completed", "serial", opts.Serial)
			return nil
		}
		time.Sleep(opts.PollInterval)
	}

	diag, _ := m.adbShell(opts.Serial, "getprop")
	err := fmt.Errorf("timed out waiting for redroid boot on %s\nDiagnostics:\n%s", opts.Serial, diag)
	recordSpanError(span, err)
	logEvent(m.env, "redroid boot timeout", "serial", opts.Serial)
	return err
}

func (m *manager) Stop(name string) error {
	ctx, span := startSpan(
		m.env,
		"redroid.Stop",
		attribute.String("name", name),
	)
	defer span.End()
	m.env.Context = ctx
	if strings.TrimSpace(name) == "" {
		err := errors.New("empty container name")
		recordSpanError(span, err)
		return err
	}
	logEvent(m.env, "redroid stop requested", "name", name)
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		_, err := m.runRemote("stop", "redroid", "--name", name)
		recordSpanError(span, err)
		if err == nil {
			logEvent(m.env, "redroid stopped", "name", name)
		}
		return err
	}
	err := m.docker.StopContainer(m.context(), name)
	recordSpanError(span, err)
	if err == nil {
		logEvent(m.env, "redroid stopped", "name", name)
	}
	return err
}

func (m *manager) Delete(name string) error {
	ctx, span := startSpan(
		m.env,
		"redroid.Delete",
		attribute.String("name", name),
	)
	defer span.End()
	m.env.Context = ctx
	if strings.TrimSpace(name) == "" {
		err := errors.New("empty container name")
		recordSpanError(span, err)
		return err
	}
	logEvent(m.env, "redroid delete requested", "name", name)
	if strings.TrimSpace(m.env.SSHTarget) != "" {
		_, err := m.runRemote("delete", "redroid", "--name", name)
		recordSpanError(span, err)
		if err == nil {
			logEvent(m.env, "redroid deleted", "name", name)
		}
		return err
	}
	err := m.docker.RemoveContainer(m.context(), name, true)
	recordSpanError(span, err)
	if err == nil {
		logEvent(m.env, "redroid deleted", "name", name)
	}
	return err
}

func (m *manager) context() context.Context {
	if m.env.Context == nil {
		return context.Background()
	}
	return m.env.Context
}

func (m *manager) adbShell(serial string, args ...string) (string, error) {
	base := []string{"-s", serial, "shell"}
	base = append(base, args...)
	out, err := m.runOutput(m.env.ADBBin, base...)
	return strings.TrimSpace(strings.ReplaceAll(out, "\r", "")), err
}

func (m *manager) run(bin string, args ...string) error {
	_, err := m.runOutput(bin, args...)
	return err
}

func (m *manager) runSudo(bin string, args ...string) error {
	_, err := m.runOutputSudo(bin, args...)
	return err
}

func (m *manager) runOutput(bin string, args ...string) (string, error) {
	return m.runOutputWithInput(nil, bin, args...)
}

func (m *manager) runOutputSudo(bin string, args ...string) (string, error) {
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

func (m *manager) runOutputWithInput(stdin io.Reader, bin string, args ...string) (string, error) {
	ctx := m.context()
	cmd := exec.CommandContext(ctx, bin, args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &out
	cmd.Stderr = io.MultiWriter(&errOut, newCommandLogWriter(m.env, bin, args))
	logEvent(m.env, "command started", "command", bin, "args", strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		logEvent(
			m.env,
			"command failed",
			"command",
			bin,
			"args",
			strings.Join(args, " "),
			"error",
			err,
			"stderr",
			strings.TrimSpace(errOut.String()),
		)
		return "", fmt.Errorf("%s %v failed: %w\n%s", bin, args, err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

func (m *manager) runRemote(args ...string) (string, error) {
	out, errOut, err := remoteavdctl.RunOutput(m.context(), m.env.SSHTarget, m.env.SSHArgs, args)
	if err != nil {
		return "", fmt.Errorf("remote avdctl %v failed: %w\n%s", args, err, strings.TrimSpace(errOut))
	}
	return strings.TrimSpace(out), nil
}

func newDockerSDKClient(env Env) dockerClient {
	opts := []client.Opt{client.FromEnv}
	if host := dockerHostFromEnv(env); host != "" {
		opts = append(opts, client.WithHost(host))
	}

	cli, err := client.New(opts...)
	if err != nil {
		return &dockerClientInitError{err: err}
	}
	return &dockerSDKClient{cli: cli}
}

func dockerHostFromEnv(env Env) string {
	if strings.TrimSpace(env.DockerHost) != "" {
		return strings.TrimSpace(env.DockerHost)
	}
	if strings.TrimSpace(env.SSHTarget) != "" {
		return "ssh://" + strings.TrimSpace(env.SSHTarget)
	}
	return ""
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
