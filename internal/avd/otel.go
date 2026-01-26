// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("avdctl")

func spanContext(env Env) context.Context {
	if env.Context != nil {
		return env.Context
	}
	return context.Background()
}

func startSpan(env Env, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if env.CorrelationID != "" {
		attrs = append(attrs, attribute.String("correlation_id", env.CorrelationID))
	}
	ctx := spanContext(env)
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func recordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
}
