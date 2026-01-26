// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
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
	correlationID := getenv("AVDCTL_CORRELATION_ID", "")
	if correlationID == "" {
		correlationID = os.Getenv("CREDIMI_CORRELATION_ID")
	}

	return Env{
		SDKRoot:       sdk,
		AVDHome:       avd,
		GoldenDir:     gold,
		ClonesDir:     clns,
		ConfigTpl:     tpl,
		Emulator:      "emulator",
		ADB:           "adb",
		AvdMgr:        "avdmanager",
		SdkManager:    "sdkmanager",
		QemuImg:       "qemu-img",
		CorrelationID: correlationID,
		Context:       context.Background(),
	}
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func DefaultGoldenDir() string { return Detect().GoldenDir }
