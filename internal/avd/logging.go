// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
)

var avdLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

func logEvent(env Env, message string, fields ...any) {
	baseFields := []any{}
	if env.CorrelationID != "" {
		baseFields = append(baseFields, "correlation_id", env.CorrelationID)
	}
	allFields := append(baseFields, fields...)
	avdLogger.Info(message, allFields...)
}

type lineLogWriter struct {
	env    Env
	fields []any
	buffer []byte
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
			logEvent(writer.env, "emulator stderr", append(writer.fields, "line", line)...)
		}
	}
	return len(payload), nil
}

func newLineLogWriter(env Env, fields ...any) io.Writer {
	return &lineLogWriter{
		env:    env,
		fields: fields,
	}
}
