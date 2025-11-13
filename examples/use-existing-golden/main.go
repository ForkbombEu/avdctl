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

	// Use your existing golden image created with customize-start/finish
	baseName := "credimi" // Your base AVD
	goldenPath := filepath.Join(os.Getenv("HOME"), "avd-golden", "credimi-golden")
	customers := []string{"acme", "globex", "initech"}

	// Verify golden exists
	if _, err := os.Stat(goldenPath); err != nil {
		log.Fatalf("Golden directory not found: %s\nCreate it with: avdctl customize-start --name %s && avdctl customize-finish --name %s --dest %s",
			goldenPath, baseName, baseName, goldenPath)
	}

	fmt.Printf("Using existing golden: %s\n", goldenPath)

	// Step 1: Create customer clones
	fmt.Println("\nStep 1: Creating customer clones from existing golden...")
	for _, customer := range customers {
		cloneName := "w-" + customer
		cloneInfo, err := avd.CloneFromGolden(env, baseName, cloneName, goldenPath)
		if err != nil {
			log.Fatalf("Failed to clone AVD for %s: %v", customer, err)
		}
		fmt.Printf("✓ Clone created: %s (size: %.2f MB)\n", cloneInfo.Name, float64(cloneInfo.SizeBytes)/(1024*1024))
	}

	// Step 2: Run all emulators in parallel
	fmt.Println("\nStep 2: Starting emulators in parallel...")
	var wg sync.WaitGroup
	results := make(chan string, len(customers)*2)

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

	// Wait a bit to ensure all results are printed
	time.Sleep(500 * time.Millisecond)

	// Step 3: List running emulators
	fmt.Println("\nStep 3: Listing running emulators...")
	running, err := avd.ListRunning(env)
	if err != nil {
		log.Fatalf("Failed to list running emulators: %v", err)
	}

	fmt.Printf("\n%d emulator(s) running:\n", len(running))
	for _, proc := range running {
		fmt.Printf("  - %s: %s (port %d, PID %d, boot: %v)\n",
			proc.Name, proc.Serial, proc.Port, proc.PID, proc.Booted)
	}

	// Step 4: Cleanup instructions
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Example completed successfully!")
	fmt.Println("\nTo stop all emulators:")
	for _, customer := range customers {
		fmt.Printf("  avdctl stop --name w-%s\n", customer)
	}
	fmt.Println("\nTo delete cloned AVDs:")
	for _, customer := range customers {
		fmt.Printf("  avdctl delete w-%s\n", customer)
	}
	fmt.Println("\nGolden image is preserved at:", goldenPath)
	fmt.Println(strings.Repeat("=", 60))
}
