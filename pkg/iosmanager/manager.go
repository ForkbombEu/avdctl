// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package iosmanager

import (
	"context"

	"github.com/forkbombeu/avdctl/internal/ios"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

type CloneOptions struct {
	Source string
	Name   string
}

type RunOptions struct {
	Name string
}

var managerTracer = otel.Tracer("avdctl/iosmanager")

func New() *Manager {
	return &Manager{env: ios.Detect()}
}

func NewWithCorrelationID(correlationID string) *Manager {
	return NewWithContextAndCorrelationID(context.Background(), correlationID)
}

func NewWithContext(ctx context.Context) *Manager {
	return NewWithContextAndCorrelationID(ctx, "")
}

func NewWithContextAndCorrelationID(ctx context.Context, correlationID string) *Manager {
	env := ios.Detect()
	if ctx == nil {
		ctx = context.Background()
	}
	env.Context = ctx
	env.CorrelationID = correlationID
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

func (m *Manager) spanContext() context.Context {
	if m.env.Context != nil {
		return m.env.Context
	}
	return context.Background()
}

func (m *Manager) startSpan(name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if m.env.CorrelationID != "" {
		attrs = append(attrs, attribute.String("correlation_id", m.env.CorrelationID))
	}
	return managerTracer.Start(m.spanContext(), name, trace.WithAttributes(attrs...))
}

func (m *Manager) withContext(ctx context.Context) ios.Env {
	env := m.env
	if ctx != nil {
		env.Context = ctx
	}
	return env
}

func recordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
}

func (m *Manager) List() ([]SimulatorInfo, error) {
	ctx, span := m.startSpan("iosmanager.List")
	defer span.End()
	infos, err := ios.List(m.withContext(ctx))
	if err != nil {
		recordSpanError(span, err)
		return nil, err
	}
	span.SetAttributes(attribute.Int("device_count", len(infos)))
	out := make([]SimulatorInfo, len(infos))
	for i, info := range infos {
		out[i] = SimulatorInfo(info)
	}
	return out, nil
}

func (m *Manager) ListRunning() ([]ProcessInfo, error) {
	ctx, span := m.startSpan("iosmanager.ListRunning")
	defer span.End()
	procs, err := ios.ListRunning(m.withContext(ctx))
	if err != nil {
		recordSpanError(span, err)
		return nil, err
	}
	span.SetAttributes(attribute.Int("running_count", len(procs)))
	out := make([]ProcessInfo, len(procs))
	for i, proc := range procs {
		out[i] = ProcessInfo(proc)
	}
	return out, nil
}

func (m *Manager) InitBase(opts InitBaseOptions) (SimulatorInfo, error) {
	ctx, span := m.startSpan(
		"iosmanager.InitBase",
		attribute.String("name", opts.Name),
	)
	defer span.End()
	info, err := ios.InitBase(m.withContext(ctx), opts.Name, opts.Runtime, opts.Device)
	if err != nil {
		recordSpanError(span, err)
		return SimulatorInfo{}, err
	}
	span.SetAttributes(attribute.String("udid", info.UDID))
	return SimulatorInfo(info), nil
}

func (m *Manager) Clone(opts CloneOptions) (SimulatorInfo, error) {
	ctx, span := m.startSpan(
		"iosmanager.Clone",
		attribute.String("source", opts.Source),
		attribute.String("name", opts.Name),
	)
	defer span.End()
	info, err := ios.Clone(m.withContext(ctx), opts.Source, opts.Name)
	if err != nil {
		recordSpanError(span, err)
		return SimulatorInfo{}, err
	}
	span.SetAttributes(attribute.String("udid", info.UDID))
	return SimulatorInfo(info), nil
}

func (m *Manager) Run(opts RunOptions) (ProcessInfo, error) {
	ctx, span := m.startSpan(
		"iosmanager.Run",
		attribute.String("name", opts.Name),
	)
	defer span.End()
	proc, err := ios.Run(m.withContext(ctx), opts.Name)
	if err != nil {
		recordSpanError(span, err)
		return ProcessInfo{}, err
	}
	span.SetAttributes(attribute.String("udid", proc.UDID))
	return ProcessInfo(proc), nil
}

func (m *Manager) Stop(ref string) error {
	ctx, span := m.startSpan(
		"iosmanager.Stop",
		attribute.String("ref", ref),
	)
	defer span.End()
	err := ios.Stop(m.withContext(ctx), ref)
	recordSpanError(span, err)
	return err
}

func (m *Manager) Delete(ref string) error {
	ctx, span := m.startSpan(
		"iosmanager.Delete",
		attribute.String("ref", ref),
	)
	defer span.End()
	err := ios.Delete(m.withContext(ctx), ref)
	recordSpanError(span, err)
	return err
}

func (m *Manager) Find(ref string) (SimulatorInfo, error) {
	ctx, span := m.startSpan(
		"iosmanager.Find",
		attribute.String("ref", ref),
	)
	defer span.End()
	info, err := ios.Find(m.withContext(ctx), ref)
	if err != nil {
		recordSpanError(span, err)
		return SimulatorInfo{}, err
	}
	span.SetAttributes(attribute.String("udid", info.UDID))
	return SimulatorInfo(info), nil
}
