package ios

import "testing"

func TestEnsureSupportedUsesBuildTarget(t *testing.T) {
	orig := platformSupported
	t.Cleanup(func() {
		platformSupported = orig
	})

	platformSupported = func() bool { return false }
	if err := EnsureSupported(); err == nil {
		t.Fatal("expected unsupported platform error")
	}

	platformSupported = func() bool { return true }
	if err := EnsureSupported(); err != nil {
		t.Fatalf("expected supported platform, got %v", err)
	}
}

func TestCompareVersion(t *testing.T) {
	if got := compareVersion([]int{18, 2}, []int{18, 1}); got <= 0 {
		t.Fatalf("expected newer version to compare higher, got %d", got)
	}
	if got := compareVersion([]int{18, 2}, []int{18, 2}); got != 0 {
		t.Fatalf("expected equal versions, got %d", got)
	}
	if got := compareVersion([]int{17, 4}, []int{18, 0}); got >= 0 {
		t.Fatalf("expected older version to compare lower, got %d", got)
	}
}
