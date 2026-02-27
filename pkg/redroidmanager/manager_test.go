package redroidmanager

import (
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

func TestStartUntarsAndRunsContainer(t *testing.T) {
	tmp := t.TempDir()
	dockerLog := filepath.Join(tmp, "docker.log")
	tarLog := filepath.Join(tmp, "tar.log")

	docker := writeExecScript(t, tmp, "docker", `
echo "$@" >> "`+dockerLog+`"
if [ "$1" = "run" ]; then
  echo "container-123"
fi
exit 0
`)
	tar := writeExecScript(t, tmp, "tar", `
echo "$@" >> "`+tarLog+`"
exit 0
`)

	dataDir := filepath.Join(tmp, "redroid-data")
	dataTar := filepath.Join(tmp, "redroid-data.tar")
	if err := os.WriteFile(dataTar, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write data tar: %v", err)
	}

	m := NewWithEnv(Environment{
		DockerBin: docker,
		TarBin:    tar,
		ADBBin:    filepath.Join(tmp, "missing-adb"),
	})

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

	dockerCmds, err := os.ReadFile(dockerLog)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	log := string(dockerCmds)
	if !strings.Contains(log, "rm -f redroid15") {
		t.Fatalf("missing rm -f command: %s", log)
	}
	if !strings.Contains(log, "run -d --name redroid15") {
		t.Fatalf("missing docker run command: %s", log)
	}
	if !strings.Contains(log, "-p 5557:5555") {
		t.Fatalf("missing port mapping: %s", log)
	}
	if !strings.Contains(log, "magsafe/redroid15gappsmagisk:latest") {
		t.Fatalf("missing image: %s", log)
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
		ADBBin:    adb,
		DockerBin: filepath.Join(tmp, "missing-docker"),
		TarBin:    filepath.Join(tmp, "missing-tar"),
	})

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
	tmp := t.TempDir()
	dockerLog := filepath.Join(tmp, "docker.log")
	docker := writeExecScript(t, tmp, "docker", `
echo "$@" >> "`+dockerLog+`"
exit 0
`)

	m := NewWithEnv(Environment{
		DockerBin: docker,
		ADBBin:    filepath.Join(tmp, "missing-adb"),
		TarBin:    filepath.Join(tmp, "missing-tar"),
	})
	if err := m.Stop("redroid15"); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
	if err := m.Delete("redroid15"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	cmds, err := os.ReadFile(dockerLog)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	log := string(cmds)
	if !strings.Contains(log, "stop redroid15") {
		t.Fatalf("missing stop command: %s", log)
	}
	if !strings.Contains(log, "rm -f redroid15") {
		t.Fatalf("missing rm command: %s", log)
	}
}

func TestSSHTransport(t *testing.T) {
	tmp := t.TempDir()
	sshLog := filepath.Join(tmp, "ssh.log")
	ssh := writeExecScript(t, tmp, "ssh", `
echo "$@" > "`+sshLog+`"
exit 0
`)

	m := NewWithEnv(Environment{
		DockerBin: "docker",
		ADBBin:    "adb",
		TarBin:    "tar",
		SSHTarget: "android@host",
		SSHBin:    ssh,
	})
	if err := m.Delete("redroid15"); err != nil {
		t.Fatalf("Delete() via ssh error: %v", err)
	}

	args, err := os.ReadFile(sshLog)
	if err != nil {
		t.Fatalf("read ssh log: %v", err)
	}
	got := string(args)
	if !strings.Contains(got, "android@host") {
		t.Fatalf("missing target in ssh args: %s", got)
	}
	if !strings.Contains(got, "sh -lc") || !strings.Contains(got, "docker") || !strings.Contains(got, "redroid15") {
		t.Fatalf("unexpected ssh command framing: %s", got)
	}
}
