package avd

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestLogEventIncludesCorrelationAndTimestamp(t *testing.T) {
	var buf bytes.Buffer
	previous := avdLogger
	avdLogger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))
	t.Cleanup(func() { avdLogger = previous })

	env := Env{CorrelationID: "corr-123"}
	logEvent(env, "test message", "key", "value")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("failed to parse log line: %v", err)
	}

	if record["correlation_id"] != "corr-123" {
		t.Fatalf("expected correlation_id corr-123, got %#v", record["correlation_id"])
	}
	if _, ok := record["timestamp_ns"]; !ok {
		t.Fatal("expected timestamp_ns field in log record")
	}
}

func TestCommandLogWriterIncludesFields(t *testing.T) {
	var buf bytes.Buffer
	previous := avdLogger
	avdLogger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))
	t.Cleanup(func() { avdLogger = previous })

	env := Env{CorrelationID: "corr-456"}
	writer := newCommandLogWriter(env, "adb", []string{"devices"})
	_, _ = writer.Write([]byte("boom\n"))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("failed to parse log line: %v", err)
	}

	if record["msg"] != "command stderr" {
		t.Fatalf("expected message 'command stderr', got %#v", record["msg"])
	}
	if record["command"] != "adb" {
		t.Fatalf("expected command adb, got %#v", record["command"])
	}
	if record["args"] != "devices" {
		t.Fatalf("expected args devices, got %#v", record["args"])
	}
	if record["line"] != "boom" {
		t.Fatalf("expected line boom, got %#v", record["line"])
	}
	if record["correlation_id"] != "corr-456" {
		t.Fatalf("expected correlation_id corr-456, got %#v", record["correlation_id"])
	}
}
