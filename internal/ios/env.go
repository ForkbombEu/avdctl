// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package ios

import (
	"context"
	"fmt"
	"os"
	"runtime"
)

type Env struct {
	Xcrun string

	CorrelationID string
	Context       context.Context
}

var platformSupported = func() bool {
	return runtime.GOOS == "darwin"
}

func Detect() Env {
	return Env{
		Xcrun:         getenv("IOS_XCRUN_BIN", "xcrun"),
		CorrelationID: getenv("AVDCTL_CORRELATION_ID", ""),
		Context:       context.Background(),
	}
}

func EnsureSupported() error {
	if platformSupported() {
		return nil
	}
	return fmt.Errorf("warning: ios commands require a macOS build of avdctl (current target: %s)", runtime.GOOS)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
