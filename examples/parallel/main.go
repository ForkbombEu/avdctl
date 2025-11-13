// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/forkbombeu/avdctl/internal/avd"
)

func main() {
	// Detect environment
	env := avd.Detect()

	// Configuration
	baseName := "base-a35-example"
	sysImage := "system-images;android-35;google_apis;x86_64"
	device := "pixel_6"
	goldenDir := filepath.Join(os.Getenv("HOME"), "avd-golden")
	goldenPath := filepath.Join(goldenDir, baseName+"-prewarmed") // Directory with raw IMG files

	// Ensure golden directory exists
	if err := os.MkdirAll(goldenDir, 0755); err != nil {
		log.Fatalf("Failed to create golden directory: %v", err)
	}

	// Step 1: Create base AVD
	fmt.Println("Step 1: Creating base AVD...")
	baseInfo, err := avd.InitBase(env, baseName, sysImage, device)
	if err != nil {
		log.Fatalf("Failed to create base AVD: %v", err)
	}
	fmt.Printf("✓ Base AVD created: %s (size: %.2f MB)\n", baseInfo.Name, float64(baseInfo.SizeBytes)/(1024*1024))

	// Step 2: Prewarm and create golden image
	fmt.Println("\nStep 2: Creating golden image (this will boot the emulator once)...")
	actualGoldenPath, goldenSize, err := avd.PrewarmGolden(env, baseName, goldenPath, 10*time.Second, 300*time.Second)
	if err != nil {
		log.Fatalf("Failed to create golden image: %v", err)
	}
	fmt.Printf("✓ Golden image created: %s (size: %.2f MB)\n", actualGoldenPath, float64(goldenSize)/(1024*1024))
	goldenPath = actualGoldenPath // Use the returned directory path

	// Step 3: Create customer clones
	customers := []string{"acme", "globex", "initech"}
	fmt.Println("\nStep 3: Creating customer clones...")
	for _, customer := range customers {
		cloneName := "w-" + customer
		cloneInfo, err := avd.CloneFromGolden(env, baseName, cloneName, goldenPath)
		if err != nil {
			log.Fatalf("Failed to clone AVD for %s: %v", customer, err)
		}
		fmt.Printf("✓ Clone created: %s (size: %.2f MB)\n", cloneInfo.Name, float64(cloneInfo.SizeBytes)/(1024*1024))
	}

	// Step 4: Run all emulators in parallel
	fmt.Println("\nStep 4: Starting emulators in parallel...")
	var wg sync.WaitGroup
	results := make(chan string, len(customers)*2)
	serials := make(chan string, len(customers))

	for _, customer := range customers {
		wg.Add(1)
		go func(cust string) {
			defer wg.Done()
			cloneName := "w-" + cust

			// Start emulator in background
			serial, err := avd.RunAVD(env, cloneName)
			if err != nil {
				results <- fmt.Sprintf("✗ %s: failed to start (%v)", cust, err)
				return
			}

			results <- fmt.Sprintf("→ %s: starting emulator on %s...", cust, serial)
			serials <- serial

			// Wait for boot
			if err := avd.WaitForBoot(env, serial, 120*time.Second); err != nil {
				results <- fmt.Sprintf("✗ %s: boot timeout (%v)", cust, err)
				return
			}

			results <- fmt.Sprintf("✓ %s: running on %s", cust, serial)
		}(customer)
	}

	// Print results as they come in
	go func() {
		for msg := range results {
			fmt.Println(msg)
		}
	}()

	wg.Wait()
	close(results)
	close(serials)

	// Wait a bit to ensure all results are printed
	time.Sleep(500 * time.Millisecond)

	// Step 5: List running emulators
	fmt.Println("\nStep 5: Listing running emulators...")
	running, err := avd.ListRunning(env)
	if err != nil {
		log.Fatalf("Failed to list running emulators: %v", err)
	}

	fmt.Printf("\n%d emulator(s) running:\n", len(running))
	for _, proc := range running {
		fmt.Printf("  - %s: %s (port %d, PID %d, boot: %v)\n",
			proc.Name, proc.Serial, proc.Port, proc.PID, proc.Booted)
	}

	// Step 6: Cleanup instructions
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Example completed successfully!")
	fmt.Println("\nTo stop all emulators:")
	for _, customer := range customers {
		fmt.Printf("  avdctl stop --name w-%s\n", customer)
	}
	fmt.Println("\nTo delete all test AVDs:")
	fmt.Printf("  avdctl delete %s\n", baseName)
	for _, customer := range customers {
		fmt.Printf("  avdctl delete w-%s\n", customer)
	}
	fmt.Printf("  rm -rf %s\n", goldenPath)
	fmt.Println(strings.Repeat("=", 60))
}
