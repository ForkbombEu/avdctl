// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

/*
Package avdmanager provides a Go library for managing Android Virtual Devices (AVDs)
with golden image and clone workflows.

# Overview

This library enables programmatic control of Android emulators using a golden-image/clone
pattern, where a base AVD is configured once, saved as a compressed QCOW2 image, and then
many lightweight clones are created for parallel execution.

# Quick Start

	import "github.com/forkbombeu/avdctl/pkg/avdmanager"

	func main() {
		// Create manager
		mgr := avdmanager.New()

		// Create base AVD
		mgr.InitBase(avdmanager.InitBaseOptions{
			Name:        "base-a35",
			SystemImage: "system-images;android-35;google_apis_playstore;x86_64",
			Device:      "pixel_6",
		})

		// Save golden image (after manual configuration)
		mgr.SaveGolden(avdmanager.SaveGoldenOptions{
			Name: "base-a35",
			Destination: "/tmp/golden.qcow2",
		})

		// Create clones
		mgr.Clone(avdmanager.CloneOptions{
			BaseName:   "base-a35",
			CloneName:  "customer1",
			GoldenPath: "/tmp/golden.qcow2",
		})

		// Run
		mgr.Run(avdmanager.RunOptions{Name: "customer1"})
	}

# Key Concepts

**Base AVD**: A clean Android system image created via avdmanager. Configure this once
with Google account, fingerprint, settings, etc.

**Golden Image**: A compressed QCOW2 snapshot of the base AVD's userdata after configuration.
This is the "template" that clones are based on.

**Clone**: A lightweight AVD that symlinks read-only files from the base and uses a thin
QCOW2 overlay backed by the golden image. Changes are written only to the overlay.

# Workflow

1. Create base AVD
2. Boot it manually with GUI, configure (Google account, fingerprint, etc.)
3. Save as golden image
4. Create N clones from golden
5. Run clones in parallel (each needs unique port)
6. Monitor with ListRunning()
7. Stop when done

# Parallel Execution

The library supports running multiple emulator instances in parallel. Each instance needs:
- Unique AVD name
- Unique even port pair (port and port+1)
- Sufficient system resources (CPU/RAM)

Use RunOnPort() to specify explicit ports for parallel instances.

# Environment Configuration

By default, the manager auto-detects paths from environment variables:
- ANDROID_SDK_ROOT
- ANDROID_AVD_HOME
- AVDCTL_GOLDEN_DIR
- AVDCTL_CONFIG_TEMPLATE

Use NewWithEnv() to override with custom paths.

# Thread Safety

Manager instances are not thread-safe. Create separate instances for concurrent use,
or synchronize access with a mutex.

# Requirements

- Android SDK with emulator, adb, avdmanager, sdkmanager
- qemu-img (from qemu-utils package)
- KVM for hardware acceleration (Linux)
- Sufficient disk space for golden images and clones

# License

AGPL-3.0-only

Copyright (C) 2025 Forkbomb B.V.
*/
package avdmanager
