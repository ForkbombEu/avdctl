package avdmanager

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func resetTracingGlobalsForTest() {
	setupOnce = sync.Once{}
	setupErr = nil
	shutdownFn = nil
}

func TestSetupTracingNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	resetTracingGlobalsForTest()

	shutdown, err := SetupTracing(context.Background())
	if err != nil {
		t.Fatalf("SetupTracing() error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestNewTraceExporterNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	exporter, useBatch, err := newTraceExporter(context.Background())
	if err != nil {
		t.Fatalf("newTraceExporter() error: %v", err)
	}
	if exporter != nil || useBatch {
		t.Fatalf("expected nil exporter and useBatch=false, got exporter=%v useBatch=%v", exporter, useBatch)
	}
}

func TestNewTraceExporterWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	exporter, useBatch, err := newTraceExporter(context.Background())
	if err != nil {
		t.Fatalf("newTraceExporter() error: %v", err)
	}
	if exporter == nil || !useBatch {
		t.Fatalf("expected exporter with useBatch=true, got exporter=%v useBatch=%v", exporter, useBatch)
	}
	_ = exporter.Shutdown(context.Background())
}

func TestNewTraceExporterWithHostPortEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")
	exporter, useBatch, err := newTraceExporter(context.Background())
	if err != nil {
		t.Fatalf("newTraceExporter() error: %v", err)
	}
	if exporter == nil || !useBatch {
		t.Fatalf("expected exporter with useBatch=true, got exporter=%v useBatch=%v", exporter, useBatch)
	}
	_ = exporter.Shutdown(context.Background())
}

func TestStdoutTraceExporterWritesSpans(t *testing.T) {
	var buf bytes.Buffer
	exporter := newStdoutTraceExporter(&buf)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
	)
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	tr := provider.Tracer("test")
	_, span := tr.Start(context.Background(), "unit-span", trace.WithAttributes(attribute.String("k", "v")))
	span.End()

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error: %v", err)
	}

	out := buf.String()
	if out == "" || !bytes.Contains([]byte(out), []byte("unit-span")) {
		t.Fatalf("expected span output, got %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("attributes:")) {
		t.Fatalf("expected attributes in output, got %q", out)
	}
}

func TestSetupTracingRespectsNilContext(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	resetTracingGlobalsForTest()

	shutdown, err := setupTracing(nil)
	if err != nil {
		t.Fatalf("setupTracing(nil) error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
}

func TestSetupTracingWithOTLPEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	resetTracingGlobalsForTest()

	shutdown, err := setupTracing(context.Background())
	if err != nil {
		t.Fatalf("setupTracing() error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}
