// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForBootWithProgressReportsStages(t *testing.T) {
	tempDir := t.TempDir()
	adbPath := filepath.Join(tempDir, "adb")
	adbScript := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  wait-for-device)\n" +
		"    exit 0\n" +
		"    ;;\n" +
		"  -s)\n" +
		"    echo \"1\"\n" +
		"    exit 0\n" +
		"    ;;\n" +
		"esac\n" +
		"exit 0\n"

	if err := os.WriteFile(adbPath, []byte(adbScript), 0o755); err != nil {
		t.Fatalf("write adb script: %v", err)
	}

	env := Env{
		ADB:     adbPath,
		Context: context.Background(),
	}

	var statuses []string
	err := WaitForBootWithProgress(
		env,
		"emulator-5554",
		5*time.Second,
		func(status string, elapsed time.Duration) {
			_ = elapsed
			statuses = append(statuses, status)
		},
	)
	if err != nil {
		t.Fatalf("WaitForBootWithProgress returned error: %v", err)
	}
	if len(statuses) < 3 {
		t.Fatalf("expected at least 3 progress callbacks, got %d", len(statuses))
	}
	if statuses[0] != "waiting_adb" {
		t.Fatalf("expected first status waiting_adb, got %s", statuses[0])
	}
	if statuses[len(statuses)-1] != "boot_complete" {
		t.Fatalf("expected last status boot_complete, got %s", statuses[len(statuses)-1])
	}
	if !statusSliceContains(statuses, "checking_bootanim") {
		t.Fatalf("expected statuses to include checking_bootanim, got %v", statuses)
	}
}

func statusSliceContains(statuses []string, want string) bool {
	for _, status := range statuses {
		if status == want {
			return true
		}
	}
	return false
}
