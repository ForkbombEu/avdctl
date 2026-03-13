// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package iosmanager

import (
	"context"

	"github.com/forkbombeu/avdctl/internal/ios"
)

type Manager struct {
	env ios.Env
}

type Environment struct {
	XcrunBin      string
	CorrelationID string
	Context       context.Context
}

type SimulatorInfo struct {
	Name      string
	UDID      string
	Runtime   string
	State     string
	Path      string
	SizeBytes int64
}

type ProcessInfo struct {
	Name    string
	UDID    string
	Runtime string
	State   string
	Booted  bool
}

type InitBaseOptions struct {
	Name    string
	Runtime string
	Device  string
}

type RunOptions struct {
	Name string
}

func New() *Manager {
	return &Manager{env: ios.Detect()}
}

func NewWithContext(ctx context.Context) *Manager {
	env := ios.Detect()
	if ctx != nil {
		env.Context = ctx
	}
	return &Manager{env: env}
}

func NewWithEnv(env Environment) *Manager {
	ctx := env.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return &Manager{
		env: ios.Env{
			Xcrun:         env.XcrunBin,
			CorrelationID: env.CorrelationID,
			Context:       ctx,
		},
	}
}

func (m *Manager) List() ([]SimulatorInfo, error) {
	infos, err := ios.List(m.env)
	if err != nil {
		return nil, err
	}
	out := make([]SimulatorInfo, len(infos))
	for i, info := range infos {
		out[i] = SimulatorInfo(info)
	}
	return out, nil
}

func (m *Manager) ListRunning() ([]ProcessInfo, error) {
	procs, err := ios.ListRunning(m.env)
	if err != nil {
		return nil, err
	}
	out := make([]ProcessInfo, len(procs))
	for i, proc := range procs {
		out[i] = ProcessInfo(proc)
	}
	return out, nil
}

func (m *Manager) InitBase(opts InitBaseOptions) (SimulatorInfo, error) {
	info, err := ios.InitBase(m.env, opts.Name, opts.Runtime, opts.Device)
	if err != nil {
		return SimulatorInfo{}, err
	}
	return SimulatorInfo(info), nil
}

func (m *Manager) Run(opts RunOptions) (ProcessInfo, error) {
	proc, err := ios.Run(m.env, opts.Name)
	if err != nil {
		return ProcessInfo{}, err
	}
	return ProcessInfo(proc), nil
}

func (m *Manager) Stop(ref string) error {
	return ios.Stop(m.env, ref)
}

func (m *Manager) Delete(ref string) error {
	return ios.Delete(m.env, ref)
}

func (m *Manager) Find(ref string) (SimulatorInfo, error) {
	info, err := ios.Find(m.env, ref)
	if err != nil {
		return SimulatorInfo{}, err
	}
	return SimulatorInfo(info), nil
}
