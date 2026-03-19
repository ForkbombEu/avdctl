// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

// Package redroidmanager provides a Go library for managing Redroid containers.
package redroidmanager

import (
	"context"

	"github.com/forkbombeu/avdctl/internal/redroid"
)

type Manager struct {
	env redroid.Env
}

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

type StartOptions = redroid.StartOptions
type WaitOptions = redroid.WaitOptions

func New() *Manager {
	return &Manager{env: redroid.Detect()}
}

func NewWithEnv(env Environment) *Manager {
	ctx := env.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return &Manager{
		env: redroid.Env{
			DockerHost: env.DockerHost,
			ADBBin:     env.ADBBin,
			TarBin:     env.TarBin,
			SudoBin:    env.SudoBin,
			Sudo:       env.Sudo,
			SudoPass:   env.SudoPass,
			SSHTarget:  env.SSHTarget,
			SSHArgs:    env.SSHArgs,
			Context:    ctx,
		},
	}
}

func (m *Manager) Start(opts StartOptions) (string, error) {
	return redroid.Start(m.env, opts)
}

func (m *Manager) WaitForBoot(opts WaitOptions) error {
	return redroid.WaitForBoot(m.env, opts)
}

func (m *Manager) Stop(name string) error {
	return redroid.Stop(m.env, name)
}

func (m *Manager) Delete(name string) error {
	return redroid.Delete(m.env, name)
}
