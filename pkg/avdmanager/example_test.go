// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avdmanager_test

import (
	"fmt"
	"log"
	"time"

	"github.com/forkbombeu/avdctl/pkg/avdmanager"
)

func Example_basicUsage() {
	// Create a new manager with auto-detected environment
	mgr := avdmanager.New()

	// Create a base AVD
	base, err := mgr.InitBase(avdmanager.InitBaseOptions{
		Name:        "base-a35",
		SystemImage: "system-images;android-35;google_apis_playstore;x86_64",
		Device:      "pixel_6",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created base AVD: %s\n", base.Name)

	// Save as golden image (after manual configuration)
	goldenPath, size, err := mgr.SaveGolden(avdmanager.SaveGoldenOptions{
		Name:        "base-a35",
		Destination: "/tmp/base-a35-golden.qcow2",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Golden saved: %s (%d bytes)\n", goldenPath, size)

	// Create clones
	clone1, err := mgr.Clone(avdmanager.CloneOptions{
		BaseName:   "base-a35",
		CloneName:  "customer1",
		GoldenPath: goldenPath,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Clone created: %s\n", clone1.Name)

	// Run the clone
	serial, err := mgr.Run(avdmanager.RunOptions{Name: "customer1"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Started on serial: %s\n", serial)

	// List running instances
	running, err := mgr.ListRunning()
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range running {
		fmt.Printf("Running: %s on %s (port %d, booted: %v)\n",
			p.Name, p.Serial, p.Port, p.Booted)
	}

	// Stop
	if err := mgr.StopByName("customer1"); err != nil {
		log.Fatal(err)
	}
}

func Example_customEnvironment() {
	// Create manager with custom paths
	mgr := avdmanager.NewWithEnv(avdmanager.Environment{
		SDKRoot:   "/opt/android-sdk",
		AVDHome:   "/custom/avd/home",
		GoldenDir: "/custom/golden",
	})

	// Use as normal
	avds, err := mgr.List()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found %d AVDs\n", len(avds))
}

func Example_prewarmWorkflow() {
	mgr := avdmanager.New()

	// Create base
	_, err := mgr.InitBase(avdmanager.InitBaseOptions{
		Name:        "base-a35",
		SystemImage: "system-images;android-35;google_apis_playstore;x86_64",
		Device:      "pixel_6",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Prewarm (automated boot + save)
	goldenPath, size, err := mgr.Prewarm(avdmanager.PrewarmOptions{
		Name:        "base-a35",
		Destination: "/tmp/base-a35-prewarmed.qcow2",
		ExtraSettle: 45 * time.Second,
		BootTimeout: 5 * time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Prewarmed golden: %s (%d bytes)\n", goldenPath, size)
}

func Example_bakeAPKs() {
	mgr := avdmanager.New()

	// Bake APKs into a new golden image
	_, _, err := mgr.BakeAPK(avdmanager.BakeAPKOptions{
		BaseName:    "base-a35",
		CloneName:   "customer-baked",
		GoldenPath:  "/tmp/base-a35-golden.qcow2",
		APKPaths:    []string{"/path/app1.apk", "/path/app2.apk"},
		Destination: "/tmp/customer-baked.qcow2",
		BootTimeout: 5 * time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("APKs baked into golden image")
}

func Example_parallelInstances() {
	mgr := avdmanager.New()

	// Start multiple instances on specific ports
	for i := 0; i < 3; i++ {
		port := 5580 + (i * 2)
		serial, logPath, err := mgr.RunOnPort(avdmanager.RunOptions{
			Name: fmt.Sprintf("customer%d", i+1),
			Port: port,
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Started on %s (log: %s)\n", serial, logPath)
	}

	// Monitor
	running, _ := mgr.ListRunning()
	fmt.Printf("Running %d instances\n", len(running))
}
