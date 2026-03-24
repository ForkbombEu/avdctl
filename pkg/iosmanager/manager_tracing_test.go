package iosmanager

import (
	"context"
	"testing"

	"github.com/forkbombeu/avdctl/internal/ios"
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
		env: ios.Env{
			Context:       context.Background(),
			CorrelationID: "corr-123",
		},
	}

	_, span := manager.startSpan(
		"iosmanager.Clone",
		attribute.String("name", "clone-1"),
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
	if attrs["name"] != "clone-1" {
		t.Fatalf("expected name to be clone-1, got %v", attrs["name"])
	}
}
