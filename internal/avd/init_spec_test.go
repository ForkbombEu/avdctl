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
	if spec.APILevel != "android-35" || spec.Tag != "google_apis_playstore" || spec.ABI != "x86_64" {
		t.Fatalf("unexpected coordinates: api=%q tag=%q abi=%q", spec.APILevel, spec.Tag, spec.ABI)
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
	if spec.APILevel != "android-35" || spec.Tag != "google_apis_playstore" || spec.ABI != "x86_64" {
		t.Fatalf("unexpected coordinates: api=%q tag=%q abi=%q", spec.APILevel, spec.Tag, spec.ABI)
	}
	if spec.Device != "pixel_6" {
		t.Fatalf("unexpected device: %q", spec.Device)
	}
}

func TestParseInitSpecRedroid(t *testing.T) {
	spec, err := ParseInitSpec("redroid15;android-35;google_apis_playstore;arm64-v8a;logged-out")
	if err != nil {
		t.Fatalf("ParseInitSpec() error: %v", err)
	}
	if spec.Kind != InitKindRedroid {
		t.Fatalf("expected redroid kind, got %q", spec.Kind)
	}
	if spec.RedroidImage != "magsafe/redroid15gappsmagisk:latest" ||
		spec.APILevel != "android-35" ||
		spec.Tag != "google_apis_playstore" ||
		spec.ABI != "arm64-v8a" ||
		spec.Profile != "logged-out" ||
		spec.RedroidDataTarURL != "https://example.com/redroid/android-35/logged-out.tar" {
		t.Fatalf("unexpected redroid parse result: %#v", spec)
	}
}

func TestParseInitSpecRedroidUnknownMapping(t *testing.T) {
	if _, err := ParseInitSpec("redroid15;android-35;google_apis_playstore;arm64-v8a;missing-profile"); err == nil {
		t.Fatal("expected error for unknown redroid mapping")
	}
}

func TestParseInitSpecSystemImagesProfileIsAVD(t *testing.T) {
	spec, err := ParseInitSpec("system-images;android-35;google_apis_playstore;arm64-v8a;logged-out")
	if err != nil {
		t.Fatalf("ParseInitSpec() error: %v", err)
	}
	if spec.Kind != InitKindAVD {
		t.Fatalf("expected AVD kind, got %q", spec.Kind)
	}
	if spec.Device != "logged-out" {
		t.Fatalf("unexpected device: %q", spec.Device)
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
