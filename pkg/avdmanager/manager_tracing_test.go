// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avdmanager

import (
	"context"
	"testing"

	"github.com/forkbombeu/avdctl/internal/avd"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestManagerStartSpanAttributes(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(provider)
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	manager := &Manager{
		env: avd.Env{
			Context:       context.Background(),
			CorrelationID: "corr-123",
		},
	}

	_, span := manager.startSpan(
		"avdmanager.Clone",
		attribute.String("avd_name", "clone-1"),
		attribute.Int("port", 5580),
	)
	span.End()

	spans := spanRecorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrs := map[string]any{}
	for _, attr := range spans[0].Attributes() {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}

	if attrs["correlation_id"] != "corr-123" {
		t.Fatalf("expected correlation_id to be corr-123, got %v", attrs["correlation_id"])
	}
	if attrs["avd_name"] != "clone-1" {
		t.Fatalf("expected avd_name to be clone-1, got %v", attrs["avd_name"])
	}
	if attrs["port"] != int64(5580) {
		t.Fatalf("expected port to be 5580, got %v", attrs["port"])
	}
}
