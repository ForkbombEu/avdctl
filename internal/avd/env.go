// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

type Env struct {
	SDKRoot    string // ANDROID_SDK_ROOT
	AVDHome    string // ANDROID_AVD_HOME (default ~/.android/avd)
	GoldenDir  string // AVDCTL_GOLDEN_DIR (default ~/avd-golden)
	ClonesDir  string // AVDCTL_CLONES_DIR (default ~/avd-clones)
	ConfigTpl  string // AVDCTL_CONFIG_TEMPLATE (optional)
	Emulator   string // emulator
	ADB        string // adb
	AvdMgr     string // avdmanager
	SdkManager string // sdkmanager
	QemuImg    string // qemu-img
	SSHTarget  string // AVDCTL_SSH_TARGET (optional, e.g. user@host)
	SSHArgs    []string
	// EmulatorSerialTimeout is the default time to wait for adb to report the
	// emulator serial after launch.
	EmulatorSerialTimeout time.Duration
	// CorrelationID is used to tie logs to a specific workflow/activity.
	CorrelationID string
	// Context is used to parent OpenTelemetry spans.
	Context context.Context
}

func Detect() Env {
	usr, _ := user.Current()
	home := ""
	if usr != nil {
		home = usr.HomeDir
	} else if h := os.Getenv("HOME"); h != "" {
		home = h
	}

	sdk := getenv("ANDROID_SDK_ROOT", "")
	avd := getenv("ANDROID_AVD_HOME", filepath.Join(home, ".android", "avd"))
	gold := getenv("AVDCTL_GOLDEN_DIR", filepath.Join(home, "avd-golden"))
	clns := getenv("AVDCTL_CLONES_DIR", filepath.Join(home, "avd-clones"))
	tpl := os.Getenv("AVDCTL_CONFIG_TEMPLATE")
	sshTarget := os.Getenv("AVDCTL_SSH_TARGET")
	sshArgs := strings.Fields(os.Getenv("AVDCTL_SSH_ARGS"))
	correlationID := getenv("AVDCTL_CORRELATION_ID", "")

	return Env{
		SDKRoot:               sdk,
		AVDHome:               avd,
		GoldenDir:             gold,
		ClonesDir:             clns,
		ConfigTpl:             tpl,
		Emulator:              "emulator",
		ADB:                   "adb",
		AvdMgr:                "avdmanager",
		SdkManager:            "sdkmanager",
		QemuImg:               "qemu-img",
		SSHTarget:             sshTarget,
		SSHArgs:               sshArgs,
		EmulatorSerialTimeout: 4 * time.Minute,
		CorrelationID:         correlationID,
		Context:               context.Background(),
	}
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func (e Env) emulatorSerialTimeout() time.Duration {
	if e.EmulatorSerialTimeout > 0 {
		return e.EmulatorSerialTimeout
	}
	return 4 * time.Minute
}

func DefaultGoldenDir() string { return Detect().GoldenDir }
