package avd

import "testing"

func TestParseInitSpecAVDFull(t *testing.T) {
	spec, err := ParseInitSpec("system-images;android-35;google_apis_playstore;x86_64;pixel_6")
	if err != nil {
		t.Fatalf("ParseInitSpec() error: %v", err)
	}
	if spec.Kind != InitKindAVD {
		t.Fatalf("expected AVD kind, got %q", spec.Kind)
	}
	if spec.SystemImage != "system-images;android-35;google_apis_playstore;x86_64" {
		t.Fatalf("unexpected system image: %q", spec.SystemImage)
	}
	if spec.Device != "pixel_6" {
		t.Fatalf("unexpected device: %q", spec.Device)
	}
}

func TestParseInitSpecAVDShort(t *testing.T) {
	spec, err := ParseInitSpec("android-35;google_apis_playstore;x86_64;pixel_6")
	if err != nil {
		t.Fatalf("ParseInitSpec() error: %v", err)
	}
	if spec.Kind != InitKindAVD {
		t.Fatalf("expected AVD kind, got %q", spec.Kind)
	}
	if spec.SystemImage != "system-images;android-35;google_apis_playstore;x86_64" {
		t.Fatalf("unexpected system image: %q", spec.SystemImage)
	}
	if spec.Device != "pixel_6" {
		t.Fatalf("unexpected device: %q", spec.Device)
	}
}

func TestParseInitSpecRedroid(t *testing.T) {
	spec, err := ParseInitSpec("redroid15;android-35;google_apis_playstore;ARM64;logged-credimitest")
	if err != nil {
		t.Fatalf("ParseInitSpec() error: %v", err)
	}
	if spec.Kind != InitKindRedroid {
		t.Fatalf("expected redroid kind, got %q", spec.Kind)
	}
	if spec.ImageName != "redroid15" || spec.AndroidVersion != "android-35" || spec.Flavor != "google_apis_playstore" || spec.Arch != "ARM64" || spec.Profile != "logged-credimitest" {
		t.Fatalf("unexpected redroid parse result: %#v", spec)
	}
}

func TestParseInitSpecInvalid(t *testing.T) {
	if _, err := ParseInitSpec("system-images;android-35;google_apis_playstore"); err == nil {
		t.Fatal("expected error for invalid init spec")
	}
}

func TestDefaultInitNameDeterministic(t *testing.T) {
	a := DefaultInitName("system-images;android-35;google_apis_playstore;x86_64;pixel_6")
	b := DefaultInitName("system-images;android-35;google_apis_playstore;x86_64;pixel_6")
	if a != b {
		t.Fatalf("expected deterministic name, got %q vs %q", a, b)
	}
	if a == "" {
		t.Fatal("expected non-empty default name")
	}
}
