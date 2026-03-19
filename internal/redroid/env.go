package redroid

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Env struct {
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

func Detect() Env {
	return Env{
		DockerHost: strings.TrimSpace(os.Getenv("DOCKER_HOST")),
		ADBBin:     "adb",
		TarBin:     "tar",
		SudoBin:    strings.TrimSpace(os.Getenv("AVDCTL_SUDO_BIN")),
		Sudo:       getenvBool("AVDCTL_SUDO"),
		SudoPass:   os.Getenv("AVDCTL_SUDO_PASSWORD"),
		SSHTarget:  os.Getenv("AVDCTL_SSH_TARGET"),
		SSHArgs:    strings.Fields(os.Getenv("AVDCTL_SSH_ARGS")),
		Context:    context.Background(),
	}
}

func DefaultDataDir() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "avdctl", "golden", "redroid-data")
}

func DefaultDataTar() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "avdctl", "golden", "redroid-data.tar")
}

func getenvBool(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return false
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	switch strings.ToLower(value) {
	case "yes", "y", "on":
		return true
	default:
		return false
	}
}
