package avdmanager

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func withRemoteRunner(
	t *testing.T,
	fn func(target string, sshArgs []string, avdArgs []string) (string, string, error),
) {
	t.Helper()
	orig := remoteRunOutput
	remoteRunOutput = func(_ context.Context, target string, sshArgs []string, avdArgs []string) (string, string, error) {
		return fn(target, sshArgs, avdArgs)
	}
	t.Cleanup(func() { remoteRunOutput = orig })
}

func newRemoteManager(t *testing.T) *Manager {
	t.Helper()
	return NewWithEnv(Environment{
		SSHTarget: "filippo@192.168.178.167",
		SSHArgs:   []string{"-i", "~/.ssh/id_ed25519"},
		Context:   context.Background(),
	})
}

func remoteKey(args []string) string {
	return strings.Join(args, "\x00")
}

func TestParseStartedLineAndPathSize(t *testing.T) {
	serial, logPath, err := parseStartedLine("Started credimi-test on emulator-5580 (log: /tmp/emulator.log)")
	if err != nil {
		t.Fatalf("parseStartedLine() error: %v", err)
	}
	if serial != "emulator-5580" || logPath != "/tmp/emulator.log" {
		t.Fatalf("unexpected parseStartedLine result: serial=%q log=%q", serial, logPath)
	}

	serial, logPath, err = parseStartedLine("Started credimi-test on emulator-5582")
	if err != nil {
		t.Fatalf("parseStartedLine() without log error: %v", err)
	}
	if serial != "emulator-5582" || logPath != "" {
		t.Fatalf("unexpected parseStartedLine(no log) result: serial=%q log=%q", serial, logPath)
	}

	if _, _, err := parseStartedLine("emulator started"); err == nil {
		t.Fatal("expected parseStartedLine failure for malformed output")
	}

	path, size, err := parsePathAndSize("Golden saved: /tmp/golden/demo (123 bytes)", "Golden saved")
	if err != nil {
		t.Fatalf("parsePathAndSize() error: %v", err)
	}
	if path != "/tmp/golden/demo" || size != 123 {
		t.Fatalf("unexpected parsePathAndSize result: path=%q size=%d", path, size)
	}

	if _, _, err := parsePathAndSize("Golden saved: /tmp/golden/demo (abc bytes)", "Golden saved"); err == nil {
		t.Fatal("expected parsePathAndSize conversion error")
	}
	if _, _, err := parsePathAndSize("something else", "Golden saved"); err == nil {
		t.Fatal("expected parsePathAndSize prefix mismatch error")
	}
}

func TestRemoteRunHelpers(t *testing.T) {
	m := newRemoteManager(t)
	withRemoteRunner(t, func(target string, sshArgs []string, avdArgs []string) (string, string, error) {
		if target == "" || len(sshArgs) == 0 {
			t.Fatalf("unexpected target/sshArgs: target=%q sshArgs=%v", target, sshArgs)
		}
		switch remoteKey(avdArgs) {
		case remoteKey([]string{"ok"}):
			return "stdout-ok", "", nil
		case remoteKey([]string{"json"}):
			return `{"k":"v"}`, "", nil
		case remoteKey([]string{"bad-json"}):
			return "{", "", nil
		case remoteKey([]string{"boom"}):
			return "", "permission denied", errors.New("ssh failed")
		default:
			return "", "", nil
		}
	})

	out, err := m.runRemote("ok")
	if err != nil {
		t.Fatalf("runRemote(ok) error: %v", err)
	}
	if out != "stdout-ok" {
		t.Fatalf("runRemote(ok) out = %q", out)
	}

	var obj map[string]string
	if err := m.runRemoteJSON(&obj, "json"); err != nil {
		t.Fatalf("runRemoteJSON(json) error: %v", err)
	}
	if obj["k"] != "v" {
		t.Fatalf("decoded json mismatch: %#v", obj)
	}

	if err := m.runRemoteJSON(&obj, "bad-json"); err == nil {
		t.Fatal("expected runRemoteJSON decode error")
	}
	if _, err := m.runRemote("boom"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected wrapped stderr in runRemote error, got: %v", err)
	}
}

func TestRemoteManagerOperations(t *testing.T) {
	m := newRemoteManager(t)
	calls := make([]string, 0, 32)
	withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
		key := remoteKey(avdArgs)
		calls = append(calls, key)
		switch key {
		case remoteKey([]string{"list", "--json"}):
			return `[{"name":"base","path":"/avd/base.avd","userdata":"/avd/base.avd/userdata.img","size_bytes":10}]`, "", nil
		case remoteKey([]string{"ps", "--json"}):
			return `[]`, "", nil
		case remoteKey([]string{"init-base", "--name", "base", "--image", "img", "--device", "pixel_6"}):
			return "Created base", "", nil
		case remoteKey([]string{"clone", "--base", "base", "--name", "clone", "--golden", "/golden/base"}):
			return "Clone ready", "", nil
		case remoteKey([]string{"stop", "--serial", "emulator-5580"}):
			return "", "", nil
		case remoteKey([]string{"stop-bluetooth", "--serial", "emulator-5580"}):
			return "", "", nil
		case remoteKey([]string{"stop", "--name", "base"}):
			return "", "", nil
		case remoteKey([]string{"delete", "base"}):
			return "", "", nil
		default:
			return "", "", nil
		}
	})

	info, err := m.InitBase(InitBaseOptions{Name: "base", SystemImage: "img", Device: "pixel_6"})
	if err != nil {
		t.Fatalf("InitBase(remote) error: %v", err)
	}
	if info.Name != "base" {
		t.Fatalf("InitBase(remote) info mismatch: %#v", info)
	}

	if _, err := m.Clone(CloneOptions{BaseName: "base", CloneName: "clone", GoldenPath: "/golden/base"}); err == nil {
		t.Fatal("expected Clone(remote) not-found error when clone not in list output")
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("List(remote) error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "base" {
		t.Fatalf("List(remote) mismatch: %#v", list)
	}

	if err := m.Stop("emulator-5580"); err != nil {
		t.Fatalf("Stop(remote) error: %v", err)
	}
	if err := m.StopBluetooth("emulator-5580"); err != nil {
		t.Fatalf("StopBluetooth(remote) error: %v", err)
	}
	if err := m.StopByName("base"); err != nil {
		t.Fatalf("StopByName(remote) error: %v", err)
	}
	if err := m.Delete("base"); err != nil {
		t.Fatalf("Delete(remote) error: %v", err)
	}

	if len(calls) == 0 {
		t.Fatal("expected remote calls to be recorded")
	}
}

func TestRemoteRunAndRunOnPortAndWaitForBoot(t *testing.T) {
	m := newRemoteManager(t)
	psCalls := 0
	var runOnPortCalled bool
	var runOnPortSerial string
	var runOnPortLog string
	withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
		if len(avdArgs) == 2 && avdArgs[0] == "ps" && avdArgs[1] == "--json" {
			psCalls++
			if psCalls >= 3 {
				return `[{"serial":"emulator-5582","name":"credimi-test","port":5582,"pid":42,"booted":true}]`, "", nil
			}
			return `[{"serial":"emulator-5580","name":"other","port":5580,"pid":41,"booted":false}]`, "", nil
		}
		switch remoteKey(avdArgs) {
		case remoteKey([]string{"run", "--name", "credimi-test"}):
			return "Started credimi-test on emulator-5580 (log: /tmp/e1.log)\n", "", nil
		}
		if len(avdArgs) == 5 && avdArgs[0] == "run" && avdArgs[1] == "--name" && avdArgs[2] == "credimi-test" && avdArgs[3] == "--port" {
			runOnPortCalled = true
			runOnPortSerial = "emulator-" + avdArgs[4]
			runOnPortLog = "/tmp/e-" + avdArgs[4] + ".log"
			return "Started credimi-test on " + runOnPortSerial + " (log: " + runOnPortLog + ")\n", "", nil
		}
		return "", "", nil
	})

	serial, err := m.Run(RunOptions{Name: "credimi-test"})
	if err != nil {
		t.Fatalf("Run(remote) error: %v", err)
	}
	if serial != "emulator-5580" {
		t.Fatalf("Run(remote) serial = %q", serial)
	}

	serial, logPath, err := m.RunOnPort(RunOptions{Name: "credimi-test", Port: 5580})
	if err != nil {
		t.Fatalf("RunOnPort(remote) error: %v", err)
	}
	if serial != runOnPortSerial || logPath != runOnPortLog || !runOnPortCalled {
		t.Fatalf("RunOnPort(remote) mismatch: serial=%q log=%q called=%v", serial, logPath, runOnPortCalled)
	}

	var statuses []string
	if err := m.WaitForBootWithProgress("emulator-5582", 2*time.Second, func(status string, _ time.Duration) {
		statuses = append(statuses, status)
	}); err != nil {
		t.Fatalf("WaitForBootWithProgress(remote) error: %v", err)
	}
	if len(statuses) == 0 || statuses[len(statuses)-1] != "boot_complete" {
		t.Fatalf("expected boot_complete status, got %v", statuses)
	}
}

func TestRemoteSavePrewarmBakeAndFindFreePort(t *testing.T) {
	m := newRemoteManager(t)
	withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
		switch remoteKey(avdArgs) {
		case remoteKey([]string{"save-golden", "--name", "demo", "--dest", "/tmp/out"}):
			return "Golden saved: /tmp/out (100 bytes)\n", "", nil
		case remoteKey([]string{"prewarm", "--name", "demo", "--extra", "1s", "--timeout", "2m0s", "--dest", "/tmp/pre"}):
			return "Prewarmed golden saved: /tmp/pre (200 bytes)\n", "", nil
		case remoteKey([]string{"bake-apk", "--base", "base", "--name", "clone", "--golden", "/tmp/g", "--apk", "/tmp/a.apk", "--dest", "/tmp/b"}):
			return "Baked clone at /tmp/b (300 bytes)\n", "", nil
		case remoteKey([]string{"ps", "--json"}):
			return `[{"serial":"emulator-5580","name":"demo","port":5580,"pid":10,"booted":true}]`, "", nil
		default:
			return "", "", nil
		}
	})

	p, sz, err := m.SaveGolden(SaveGoldenOptions{Name: "demo", Destination: "/tmp/out"})
	if err != nil || p != "/tmp/out" || sz != 100 {
		t.Fatalf("SaveGolden(remote) mismatch: path=%q size=%d err=%v", p, sz, err)
	}
	p, sz, err = m.Prewarm(PrewarmOptions{
		Name:        "demo",
		Destination: "/tmp/pre",
		ExtraSettle: time.Second,
		BootTimeout: 2 * time.Minute,
	})
	if err != nil || p != "/tmp/pre" || sz != 200 {
		t.Fatalf("Prewarm(remote) mismatch: path=%q size=%d err=%v", p, sz, err)
	}
	p, sz, err = m.BakeAPK(BakeAPKOptions{
		BaseName:    "base",
		CloneName:   "clone",
		GoldenPath:  "/tmp/g",
		APKPaths:    []string{"/tmp/a.apk"},
		Destination: "/tmp/b",
		BootTimeout: 2 * time.Minute,
	})
	if err != nil || p != "/tmp/b" || sz != 300 {
		t.Fatalf("BakeAPK(remote) mismatch: path=%q size=%d err=%v", p, sz, err)
	}

	port, err := m.FindFreePort(5580, 5590)
	if err != nil {
		t.Fatalf("FindFreePort(remote) error: %v", err)
	}
	if port != 5582 {
		t.Fatalf("FindFreePort(remote) expected 5582, got %d", port)
	}
}

func TestRemoteKillAllAndErrorPaths(t *testing.T) {
	m := newRemoteManager(t)
	psCalls := 0
	withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
		switch remoteKey(avdArgs) {
		case remoteKey([]string{"ps", "--json"}):
			psCalls++
			if psCalls == 1 {
				return `[{"serial":"emulator-5580","name":"demo","port":5580,"pid":101,"booted":false}]`, "", nil
			}
			return `[]`, "", nil
		case remoteKey([]string{"stop", "--serial", "emulator-5580"}):
			return "", "", nil
		case remoteKey([]string{"run", "--name", "bad"}):
			return "not parseable", "", nil
		default:
			return "", "", nil
		}
	})

	report, err := m.KillAllEmulators(KillAllEmulatorsOptions{MaxPasses: 3, Delay: time.Millisecond})
	if err != nil {
		t.Fatalf("KillAllEmulators(remote) error: %v", err)
	}
	if report.Passes != 1 || report.Remaining != 0 || len(report.KilledPIDs) != 1 || report.KilledPIDs[0] != 101 {
		t.Fatalf("KillAllEmulators(remote) report mismatch: %#v", report)
	}

	if _, err := m.Run(RunOptions{Name: "bad"}); err == nil {
		t.Fatal("expected Run(remote) parse failure")
	}

	// Timeout branch (serial never reported as booted).
	if err := m.WaitForBootWithProgress("emulator-9999", 1*time.Nanosecond, nil); err == nil {
		t.Fatal("expected WaitForBootWithProgress(remote) timeout")
	}

	withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
		if remoteKey(avdArgs) == remoteKey([]string{"ps", "--json"}) {
			return `[{"serial":"emulator-5580","name":"x","port":5580,"pid":1,"booted":false},{"serial":"emulator-5582","name":"y","port":5582,"pid":2,"booted":false}]`, "", nil
		}
		return "", "", nil
	})
	if _, err := m.FindFreePort(5580, 5584); err == nil {
		t.Fatal("expected FindFreePort(remote) exhaustion error")
	}
}

func TestRemoteAdditionalBranchesForCoverage(t *testing.T) {
	t.Run("runRemote nil context", func(t *testing.T) {
		m := NewWithEnv(Environment{
			SSHTarget: "filippo@192.168.178.167",
			SSHArgs:   []string{"-i", "~/.ssh/id_ed25519"},
			// Intentionally nil context to cover fallback path.
		})
		called := false
		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			called = true
			if remoteKey(avdArgs) != remoteKey([]string{"noop"}) {
				t.Fatalf("unexpected args: %v", avdArgs)
			}
			return "ok", "", nil
		})
		if _, err := m.runRemote("noop"); err != nil {
			t.Fatalf("runRemote(nil context) error: %v", err)
		}
		if !called {
			t.Fatal("expected remote runner invocation")
		}
	})

	t.Run("run and list error branches", func(t *testing.T) {
		m := newRemoteManager(t)
		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			switch remoteKey(avdArgs) {
			case remoteKey([]string{"ps", "--json"}):
				return `[{"serial":"emulator-5580","name":"demo","port":5580,"pid":1,"booted":false}]`, "", nil
			case remoteKey([]string{"list", "--json"}):
				return `not-json`, "", nil
			default:
				return "", "", nil
			}
		})
		if _, err := m.Run(RunOptions{Name: "demo"}); err == nil || !strings.Contains(err.Error(), "already running") {
			t.Fatalf("expected already-running error, got: %v", err)
		}
		if _, err := m.ListRunning(); err != nil {
			t.Fatalf("ListRunning(remote) expected success: %v", err)
		}
		if _, err := m.List(); err == nil {
			t.Fatal("expected List(remote) decode error")
		}
		if err := m.runRemoteJSON(&map[string]any{}, "boom"); err == nil {
			t.Fatal("expected runRemoteJSON error when remote output is not JSON")
		}
	})

	t.Run("RunOnPort port zero delegates to Run", func(t *testing.T) {
		m := newRemoteManager(t)
		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			switch remoteKey(avdArgs) {
			case remoteKey([]string{"ps", "--json"}):
				return `[]`, "", nil
			case remoteKey([]string{"run", "--name", "demo"}):
				return "Started demo on emulator-5590 (log: /tmp/e-5590.log)\n", "", nil
			default:
				return "", "", nil
			}
		})
		serial, logPath, err := m.RunOnPort(RunOptions{Name: "demo", Port: 0})
		if err != nil {
			t.Fatalf("RunOnPort(port=0 remote) error: %v", err)
		}
		if serial != "emulator-5590" || logPath != "" {
			t.Fatalf("RunOnPort(port=0 remote) mismatch: serial=%q log=%q", serial, logPath)
		}
	})

	t.Run("FindFreePort odd start and list error", func(t *testing.T) {
		m := newRemoteManager(t)
		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			if remoteKey(avdArgs) == remoteKey([]string{"ps", "--json"}) {
				return `[]`, "", nil
			}
			return "", "", nil
		})
		port, err := m.FindFreePort(5555, 5565)
		if err != nil {
			t.Fatalf("FindFreePort(odd start remote) error: %v", err)
		}
		if port != 5556 {
			t.Fatalf("FindFreePort(odd start remote) expected 5556, got %d", port)
		}

		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			if remoteKey(avdArgs) == remoteKey([]string{"ps", "--json"}) {
				return "", "boom", errors.New("ssh error")
			}
			return "", "", nil
		})
		if _, err := m.FindFreePort(5554, 5560); err == nil {
			t.Fatal("expected FindFreePort(remote) list error")
		}
	})

	t.Run("KillAllEmulators default, list error, and remaining path", func(t *testing.T) {
		m := newRemoteManager(t)

		// Default path: no procs on first pass.
		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			if remoteKey(avdArgs) == remoteKey([]string{"ps", "--json"}) {
				return `[]`, "", nil
			}
			return "", "", nil
		})
		report, err := m.KillAllEmulators(KillAllEmulatorsOptions{})
		if err != nil {
			t.Fatalf("KillAllEmulators(default remote) error: %v", err)
		}
		if report.Passes != 0 || report.Remaining != 0 {
			t.Fatalf("KillAllEmulators(default remote) report mismatch: %#v", report)
		}

		// List error path.
		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			if remoteKey(avdArgs) == remoteKey([]string{"ps", "--json"}) {
				return "", "perm denied", errors.New("ssh")
			}
			return "", "", nil
		})
		if _, err := m.KillAllEmulators(KillAllEmulatorsOptions{MaxPasses: 1, Delay: time.Millisecond}); err == nil {
			t.Fatal("expected KillAllEmulators(remote) list error")
		}

		// Max-passes exhausted: still remaining processes.
		psCalls := 0
		withRemoteRunner(t, func(_ string, _ []string, avdArgs []string) (string, string, error) {
			switch remoteKey(avdArgs) {
			case remoteKey([]string{"ps", "--json"}):
				psCalls++
				return `[{"serial":"emulator-5580","name":"demo","port":5580,"pid":10,"booted":false}]`, "", nil
			case remoteKey([]string{"stop", "--serial", "emulator-5580"}):
				return "", "", nil
			default:
				return "", "", nil
			}
		})
		report, err = m.KillAllEmulators(KillAllEmulatorsOptions{MaxPasses: 2, Delay: time.Millisecond})
		if err != nil {
			t.Fatalf("KillAllEmulators(remaining remote) error: %v", err)
		}
		if report.Passes != 2 || report.Remaining != 1 || len(report.KilledPIDs) == 0 || psCalls < 3 {
			t.Fatalf("KillAllEmulators(remaining remote) report mismatch: %#v (psCalls=%d)", report, psCalls)
		}
	})
}
