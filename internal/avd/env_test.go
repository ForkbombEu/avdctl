package avd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultGoldenDirUsesEnvOverride(t *testing.T) {
	old := os.Getenv("AVDCTL_GOLDEN_DIR")
	t.Cleanup(func() {
		_ = os.Setenv("AVDCTL_GOLDEN_DIR", old)
	})

	want := filepath.Join(t.TempDir(), "golden")
	if err := os.Setenv("AVDCTL_GOLDEN_DIR", want); err != nil {
		t.Fatalf("set env: %v", err)
	}

	if got := DefaultGoldenDir(); got != want {
		t.Fatalf("DefaultGoldenDir() = %q, want %q", got, want)
	}
}
