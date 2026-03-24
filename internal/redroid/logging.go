package redroid

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

var redroidLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

func logEvent(env Env, message string, fields ...any) {
	baseFields := []any{"timestamp_ns", time.Now().UTC().UnixNano()}
	if env.CorrelationID != "" {
		baseFields = append(baseFields, "correlation_id", env.CorrelationID)
	}
	if span := trace.SpanContextFromContext(spanContext(env)); span.IsValid() {
		baseFields = append(baseFields,
			"trace_id", span.TraceID().String(),
			"span_id", span.SpanID().String(),
		)
	}
	allFields := append(baseFields, fields...)
	redroidLogger.Info(message, allFields...)
	emitOTelLog(env, message, allFields...)
}

func emitOTelLog(env Env, message string, fields ...any) {
	ctx := spanContext(env)
	var record otellog.Record
	now := time.Now().UTC()
	record.SetTimestamp(now)
	record.SetObservedTimestamp(now)
	record.SetSeverity(otellog.SeverityInfo)
	record.SetSeverityText("info")
	record.SetBody(otellog.StringValue(message))
	record.AddAttributes(logFields(fields)...)
	logglobal.Logger("avdctl").Emit(ctx, record)
}

func logFields(fields []any) []otellog.KeyValue {
	if len(fields) == 0 {
		return nil
	}
	attrs := make([]otellog.KeyValue, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		attrs = append(attrs, otellog.String(key, toLogValue(fields[i+1])))
	}
	return attrs
}

func toLogValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		return strings.TrimSpace(strings.Join(v, " "))
	case nil:
		return ""
	default:
		return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(v), "\n", " "), "\t", " "))
	}
}

type lineLogWriter struct {
	env    Env
	fields []any
	buffer []byte
	msg    string
}

func (writer *lineLogWriter) Write(payload []byte) (int, error) {
	writer.buffer = append(writer.buffer, payload...)
	for {
		newlineIndex := bytes.IndexByte(writer.buffer, '\n')
		if newlineIndex == -1 {
			break
		}
		line := strings.TrimSpace(string(writer.buffer[:newlineIndex]))
		writer.buffer = writer.buffer[newlineIndex+1:]
		if line != "" {
			logEvent(writer.env, writer.msg, append(writer.fields, "line", line)...)
		}
	}
	return len(payload), nil
}

func newLineLogWriterWithMessage(env Env, message string, fields ...any) io.Writer {
	return &lineLogWriter{
		env:    env,
		fields: fields,
		msg:    message,
	}
}

func newCommandLogWriter(env Env, command string, args []string) io.Writer {
	fields := []any{"command", command, "stream", "stderr"}
	if len(args) > 0 {
		fields = append(fields, "args", strings.Join(args, " "))
	}
	return newLineLogWriterWithMessage(env, "command stderr", fields...)
}
