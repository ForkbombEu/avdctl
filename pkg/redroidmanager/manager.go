// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

// Package redroidmanager provides a Go library for managing Redroid containers.
package redroidmanager

import (
	"context"

	"github.com/forkbombeu/avdctl/internal/redroid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

	CorrelationID string
	Context       context.Context
}

type StartOptions = redroid.StartOptions
type WaitOptions = redroid.WaitOptions

var managerTracer = otel.Tracer("avdctl/redroidmanager")

func New() *Manager {
	return &Manager{env: redroid.Detect()}
}

func NewWithCorrelationID(correlationID string) *Manager {
	return NewWithContextAndCorrelationID(context.Background(), correlationID)
}

func NewWithContext(ctx context.Context) *Manager {
	return NewWithContextAndCorrelationID(ctx, "")
}

func NewWithContextAndCorrelationID(ctx context.Context, correlationID string) *Manager {
	env := redroid.Detect()
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
		env: redroid.Env{
			DockerHost:    env.DockerHost,
			ADBBin:        env.ADBBin,
			TarBin:        env.TarBin,
			SudoBin:       env.SudoBin,
			Sudo:          env.Sudo,
			SudoPass:      env.SudoPass,
			CorrelationID: env.CorrelationID,
			SSHTarget:     env.SSHTarget,
			SSHArgs:       env.SSHArgs,
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

func (m *Manager) withContext(ctx context.Context) redroid.Env {
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

func (m *Manager) Start(opts StartOptions) (string, error) {
	ctx, span := m.startSpan(
		"redroidmanager.Start",
		attribute.String("name", opts.Name),
		attribute.String("image", opts.Image),
		attribute.Int("host_port", opts.HostPort),
	)
	defer span.End()
	containerID, err := redroid.Start(m.withContext(ctx), opts)
	recordSpanError(span, err)
	if err == nil && containerID != "" {
		span.SetAttributes(attribute.String("container_id", containerID))
	}
	return containerID, err
}

func (m *Manager) WaitForBoot(opts WaitOptions) error {
	ctx, span := m.startSpan(
		"redroidmanager.WaitForBoot",
		attribute.String("serial", opts.Serial),
	)
	defer span.End()
	err := redroid.WaitForBoot(m.withContext(ctx), opts)
	recordSpanError(span, err)
	return err
}

func (m *Manager) Stop(name string) error {
	ctx, span := m.startSpan(
		"redroidmanager.Stop",
		attribute.String("name", name),
	)
	defer span.End()
	err := redroid.Stop(m.withContext(ctx), name)
	recordSpanError(span, err)
	return err
}

func (m *Manager) Delete(name string) error {
	ctx, span := m.startSpan(
		"redroidmanager.Delete",
		attribute.String("name", name),
	)
	defer span.End()
	err := redroid.Delete(m.withContext(ctx), name)
	recordSpanError(span, err)
	return err
}
