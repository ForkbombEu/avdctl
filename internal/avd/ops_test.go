package avd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	env := Detect()
	if env.AVDHome == "" {
		t.Fatal("AVDHome should not be empty")
	}
}

func TestCreateSDCardNormalizesQEMUImageSize(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.ini")
	if err := os.WriteFile(configPath, []byte("sdcard.size=512 MB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "qemu-size")
	qemuPath := filepath.Join(dir, "qemu-img")
	if err := os.WriteFile(qemuPath, []byte("#!/bin/sh\nprintf '%s' \"$5\" > \"$QEMU_SIZE_OUTPUT\"\ntouch \"$4\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QEMU_SIZE_OUTPUT", outputPath)

	if err := createSDCard(Env{Context: context.Background(), QemuImg: qemuPath}, dir, configPath); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "512M" {
		t.Fatalf("qemu-img size = %q, want 512M", got)
	}
}
