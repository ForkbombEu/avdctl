package redroidmanager

import (
	"context"
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

type fakeDockerClient struct {
	removeCalls []string
	stopCalls   []string
	runCalls    []dockerRunOptions

	runID string
	err   error
}

func (f *fakeDockerClient) RemoveContainer(_ context.Context, name string, _ bool) error {
	f.removeCalls = append(f.removeCalls, name)
	return f.err
}

func (f *fakeDockerClient) RunContainer(_ context.Context, opts dockerRunOptions) (string, error) {
	f.runCalls = append(f.runCalls, opts)
	if f.runID == "" {
		f.runID = "container-123"
	}
	return f.runID, f.err
}

func (f *fakeDockerClient) StopContainer(_ context.Context, name string) error {
	f.stopCalls = append(f.stopCalls, name)
	return f.err
}

func TestStartUntarsAndRunsContainer(t *testing.T) {
	tmp := t.TempDir()
	tarLog := filepath.Join(tmp, "tar.log")

	tar := writeExecScript(t, tmp, "tar", `
echo "$@" >> "`+tarLog+`"
exit 0
`)

	dataDir := filepath.Join(tmp, "redroid-data")
	dataTar := filepath.Join(tmp, "redroid-data.tar")
	if err := os.WriteFile(dataTar, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write data tar: %v", err)
	}

	fakeDocker := &fakeDockerClient{runID: "container-123"}
	m := NewWithEnv(Environment{
		TarBin: tar,
		ADBBin: filepath.Join(tmp, "missing-adb"),
	})
	m.docker = fakeDocker

	containerID, err := m.Start(StartOptions{
		Name:     "redroid15",
		Image:    "magsafe/redroid15gappsmagisk:latest",
		DataDir:  dataDir,
		DataTar:  dataTar,
		HostPort: 5557,
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if containerID != "container-123" {
		t.Fatalf("containerID = %q", containerID)
	}

	tarCmd, err := os.ReadFile(tarLog)
	if err != nil {
		t.Fatalf("read tar log: %v", err)
	}
	if !strings.Contains(string(tarCmd), "-C "+filepath.Dir(dataDir)+" -xf "+dataTar) {
		t.Fatalf("unexpected tar args: %s", string(tarCmd))
	}

	if len(fakeDocker.removeCalls) != 1 || fakeDocker.removeCalls[0] != "redroid15" {
		t.Fatalf("unexpected remove calls: %#v", fakeDocker.removeCalls)
	}
	if len(fakeDocker.runCalls) != 1 {
		t.Fatalf("expected one run call, got %#v", fakeDocker.runCalls)
	}
	run := fakeDocker.runCalls[0]
	if run.Name != "redroid15" || run.HostPort != 5557 {
		t.Fatalf("unexpected run opts: %#v", run)
	}
}

func TestWaitForBoot(t *testing.T) {
	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	adb := writeExecScript(t, tmp, "adb", `
case "$1" in
  start-server)
    exit 0
    ;;
  connect)
    exit 0
    ;;
  -s)
    if [ "$3" = "wait-for-device" ]; then
      exit 0
    fi
    if [ "$3" = "shell" ] && [ "$4" = "getprop" ] && [ "$5" = "init.svc.system_server" ]; then
      echo "running"
      exit 0
    fi
    if [ "$3" = "shell" ] && [ "$4" = "getprop" ] && [ "$5" = "sys.boot_completed" ]; then
      c=0
      if [ -f "`+state+`" ]; then
        c=$(cat "`+state+`")
      fi
      c=$((c+1))
      echo "$c" > "`+state+`"
      if [ "$c" -ge 2 ]; then
        echo "1"
      else
        echo "0"
      fi
      exit 0
    fi
    if [ "$3" = "shell" ] && [ "$4" = "service" ] && [ "$5" = "check" ] && [ "$6" = "package" ]; then
      echo "Service package: found"
      exit 0
    fi
    if [ "$3" = "shell" ] && [ "$4" = "service" ] && [ "$5" = "check" ] && [ "$6" = "activity" ]; then
      echo "Service activity: found"
      exit 0
    fi
    ;;
esac
exit 0
`)

	m := NewWithEnv(Environment{
		ADBBin: adb,
		TarBin: filepath.Join(tmp, "missing-tar"),
	})
	m.docker = &fakeDockerClient{}

	err := m.WaitForBoot(WaitOptions{
		Serial:       "127.0.0.1:5555",
		Timeout:      3 * time.Second,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WaitForBoot() error: %v", err)
	}
}

func TestStopAndDelete(t *testing.T) {
	fakeDocker := &fakeDockerClient{}
	m := NewWithEnv(Environment{})
	m.docker = fakeDocker

	if err := m.Stop("redroid15"); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
	if err := m.Delete("redroid15"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	if len(fakeDocker.stopCalls) != 1 || fakeDocker.stopCalls[0] != "redroid15" {
		t.Fatalf("unexpected stop calls: %#v", fakeDocker.stopCalls)
	}
	if len(fakeDocker.removeCalls) != 1 || fakeDocker.removeCalls[0] != "redroid15" {
		t.Fatalf("unexpected remove calls: %#v", fakeDocker.removeCalls)
	}
}

func TestDockerHostFromEnv(t *testing.T) {
	if got := dockerHostFromEnv(Environment{SSHTarget: "android@host"}); got != "ssh://android@host" {
		t.Fatalf("dockerHostFromEnv ssh = %q", got)
	}
	if got := dockerHostFromEnv(Environment{SSHTarget: "android@host", DockerHost: "unix:///var/run/docker.sock"}); got != "unix:///var/run/docker.sock" {
		t.Fatalf("dockerHostFromEnv explicit host = %q", got)
	}
	if got := dockerHostFromEnv(Environment{}); got != "" {
		t.Fatalf("dockerHostFromEnv empty = %q", got)
	}
}
