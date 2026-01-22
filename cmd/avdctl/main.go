// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	core "github.com/forkbombeu/avdctl/internal/avd"
	"github.com/forkbombeu/avdctl/pkg/avdmanager"
)

func main() {
	shutdownTracing, err := avdmanager.SetupTracing(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize tracing: %v\n", err)
	}
	if shutdownTracing != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownTracing(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "failed to shutdown tracing: %v\n", err)
			}
		}()
	}
	env := core.Detect()

	root := &cobra.Command{
		Use:   "avdctl",
		Short: "AVD golden/clone lifecycle tool (Linux, CI-friendly)",
	}

	// list
	var listJSON bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List AVDs under ANDROID_AVD_HOME",
		RunE: func(cmd *cobra.Command, args []string) error {
			ls, err := core.List(env)
			if err != nil {
				return err
			}
			if listJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(ls)
			}
			for _, i := range ls {
				fmt.Printf("%-18s %s\n  userdata: %s (%d bytes)\n", i.Name, i.Path, i.Userdata, i.SizeBytes)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output JSON")
	root.AddCommand(listCmd)

	// init-base
	var baseName, sysImg, device string
	initCmd := &cobra.Command{
		Use:   "init-base",
		Short: "Create a base AVD (auto-installs system image if missing)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if baseName == "" {
				return errors.New("--name is required")
			}
			inf, err := core.InitBase(env, baseName, sysImg, device)
			if err != nil {
				return err
			}
			fmt.Printf("Created %s at %s\n", inf.Name, inf.Path)
			return nil
		},
	}
	initCmd.Flags().StringVar(&baseName, "name", "base-a35", "AVD name (include API, e.g. base-a35)")
	initCmd.Flags().StringVar(&sysImg, "image", "system-images;android-35;google_apis_playstore;x86_64", "System image ID")
	initCmd.Flags().StringVar(&device, "device", "pixel_6", "Device profile")
	root.AddCommand(initCmd)

	// save-golden
	var sgName, sgDest string
	saveCmd := &cobra.Command{
		Use:   "save-golden",
		Short: "Export AVD userdata to compressed QCOW2 golden",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sgName == "" {
				return errors.New("--name is required")
			}
			if sgDest == "" {
				dir := core.DefaultGoldenDir()
				_ = os.MkdirAll(dir, 0o755)
				sgDest = filepath.Join(dir, fmt.Sprintf("%s-userdata.qcow2", sgName))
			}
			dst, sz, err := core.SaveGolden(env, sgName, sgDest)
			if err != nil {
				return err
			}
			fmt.Printf("Golden saved: %s (%d bytes)\n", dst, sz)
			return nil
		},
	}
	saveCmd.Flags().StringVar(&sgName, "name", "", "AVD name")
	saveCmd.Flags().StringVar(&sgDest, "dest", "", "Destination qcow2 (default: $AVDCTL_GOLDEN_DIR/<name>-userdata.qcow2)")
	root.AddCommand(saveCmd)

	// prewarm
	var pwName, pwDest string
	var pwExtra, pwTimeout time.Duration
	prewarmCmd := &cobra.Command{
		Use:   "prewarm",
		Short: "Boot once (no snapshots), wait for boot, settle caches, then save golden QCOW2",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pwName == "" {
				return errors.New("--name is required")
			}
			if pwDest == "" {
				dir := core.DefaultGoldenDir()
				_ = os.MkdirAll(dir, 0o755)
				pwDest = filepath.Join(dir, fmt.Sprintf("%s-prewarmed.qcow2", pwName))
			}
			dst, sz, err := core.PrewarmGolden(env, pwName, pwDest, pwExtra, pwTimeout)
			if err != nil {
				return err
			}
			fmt.Printf("Prewarmed golden saved: %s (%d bytes)\n", dst, sz)
			return nil
		},
	}
	prewarmCmd.Flags().StringVar(&pwName, "name", "", "AVD name")
	prewarmCmd.Flags().StringVar(&pwDest, "dest", "", "Destination qcow2 (default: $AVDCTL_GOLDEN_DIR/<name>-prewarmed.qcow2)")
	prewarmCmd.Flags().DurationVar(&pwExtra, "extra", 30*time.Second, "extra settle time after boot")
	prewarmCmd.Flags().DurationVar(&pwTimeout, "timeout", 3*time.Minute, "boot timeout")
	root.AddCommand(prewarmCmd)

	// customize-start
	var csName string
	customizeStartCmd := &cobra.Command{
		Use:   "customize-start",
		Short: "Prepare AVD and start GUI for manual customization (no snapshots)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if csName == "" {
				return errors.New("--name is required")
			}
			logPath, err := core.CustomizeStart(env, csName)
			if err != nil {
				return err
			}
			fmt.Printf("Customize started (log: %s)\n", logPath)
			return nil
		},
	}
	customizeStartCmd.Flags().StringVar(&csName, "name", "", "AVD name")
	root.AddCommand(customizeStartCmd)

	// customize-finish
	var cfName, cfDest string
	customizeFinishCmd := &cobra.Command{
		Use:   "customize-finish",
		Short: "Stop emulator (if running) and export userdata to golden directory (raw IMG format)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfName == "" {
				return errors.New("--name is required")
			}
			dst, sz, err := core.CustomizeFinish(env, cfName, cfDest)
			if err != nil {
				return err
			}
			fmt.Printf("Golden saved: %s (%d bytes)\n", dst, sz)
			return nil
		},
	}
	customizeFinishCmd.Flags().StringVar(&cfName, "name", "", "AVD name")
	customizeFinishCmd.Flags().StringVar(&cfDest, "dest", "", "Destination directory (default: $AVDCTL_GOLDEN_DIR/<name>-custom)")
	root.AddCommand(customizeFinishCmd)

	// clone
	var clBase, clName, clGolden string
	cloneCmd := &cobra.Command{
		Use:   "clone",
		Short: "Create clone by copying raw IMG files from golden directory (preserves all customizations)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clBase == "" || clName == "" {
				return errors.New("--base and --name are required")
			}
			if clGolden == "" {
				return errors.New("--golden is required")
			}
			inf, err := core.CloneFromGolden(env, clBase, clName, clGolden)
			if err != nil {
				return err
			}
			fmt.Printf("Clone ready: %s at %s\n", inf.Name, inf.Path)
			return nil
		},
	}
	cloneCmd.Flags().StringVar(&clBase, "base", "", "Base AVD name (e.g., base-a35)")
	cloneCmd.Flags().StringVar(&clName, "name", "", "New clone name (e.g., w-<slug>)")
	cloneCmd.Flags().StringVar(&clGolden, "golden", "", "Path to golden directory")
	root.AddCommand(cloneCmd)

	// run (supports optional --port for parallel instances)
	var runName string
	var runPort int
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run an AVD headless (no snapshots); supports parallel instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runName == "" {
				return fmt.Errorf("--name is required")
			}
			env := core.Detect()

			if runPort > 0 {
				if runPort%2 != 0 {
					return fmt.Errorf("--port must be even")
				}
				// Deterministic port
				_, _, logPath, err := core.StartEmulatorOnPort(env, runName, runPort)
				if err != nil {
					return err
				}
				fmt.Printf("Started %s on emulator-%d (log: %s)\n", runName, runPort, logPath)
				return nil
			}

			// Auto-pick a free even port
			_, err := core.RunAVD(env, runName)
			if err != nil {
				return err
			}
			// RunAVD prints "Started <name> on emulator-<port>" itself
			return nil
		},
	}
	runCmd.Flags().StringVar(&runName, "name", "", "AVD name to run")
	runCmd.Flags().IntVar(&runPort, "port", 0, "even TCP port to bind emulator (auto if omitted)")
	root.AddCommand(runCmd)

	// bake-apk
	var bkBase, bkName, bkGolden, bkOut string
	var apks []string
	bakeCmd := &cobra.Command{
		Use:   "bake-apk",
		Short: "Clone → boot → install APK(s) → shutdown → export new golden",
		RunE: func(cmd *cobra.Command, args []string) error {
			if bkBase == "" || bkName == "" || bkGolden == "" {
				return errors.New("--base, --name, --golden are required")
			}
			if len(apks) == 0 {
				return errors.New("--apk must be provided at least once")
			}
			if bkOut == "" {
				dir := core.DefaultGoldenDir()
				_ = os.MkdirAll(dir, 0o755)
				bkOut = filepath.Join(dir, fmt.Sprintf("%s-baked.qcow2", bkName))
			}
			dst, sz, err := core.BakeAPK(env, bkBase, bkName, bkGolden, apks, 3*time.Minute)
			if err != nil {
				return err
			}
			// Export baked clone to golden
			dst2, sz2, err := core.SaveGolden(env, bkName, bkOut)
			if err != nil {
				return err
			}
			fmt.Printf("Baked clone at %s (%d bytes)\n", dst, sz)
			fmt.Printf("Exported baked golden: %s (%d bytes)\n", dst2, sz2)
			return nil
		},
	}
	bakeCmd.Flags().StringVar(&bkBase, "base", "", "Base AVD name")
	bakeCmd.Flags().StringVar(&bkName, "name", "", "New baked clone name (e.g., w-<slug>)")
	bakeCmd.Flags().StringVar(&bkGolden, "golden", "", "Path to base golden qcow2")
	bakeCmd.Flags().StringSliceVar(&apks, "apk", nil, "APK file(s) to install (repeatable)")
	bakeCmd.Flags().StringVar(&bkOut, "dest", "", "Destination golden qcow2 for baked image")
	root.AddCommand(bakeCmd)

	// delete
	delCmd := &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete an AVD (+ .ini)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return core.Delete(env, args[0])
		},
	}
	root.AddCommand(delCmd)

	// ps
	var psJSON bool
	psCmd := &cobra.Command{
		Use:   "ps",
		Short: "List running emulators with AVD name, serial, port, PID",
		RunE: func(cmd *cobra.Command, args []string) error {
			env := core.Detect()
			procs, err := core.ListRunning(env)
			if err != nil {
				return err
			}
			if psJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(procs)
			}
			if len(procs) == 0 {
				fmt.Println("(no emulators)")
				return nil
			}
			for _, p := range procs {
				state := "booting"
				if p.Booted {
					state = "ready"
				}
				fmt.Printf("%-18s %-14s port=%-5d pid=%-7d %s\n", p.Name, p.Serial, p.Port, p.PID, state)
			}
			return nil
		},
	}
	psCmd.Flags().BoolVar(&psJSON, "json", false, "output JSON")
	root.AddCommand(psCmd)

	// status
	var stName, stSerial string
	var stAll bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show status for a running emulator by --name or --serial",
		RunE: func(cmd *cobra.Command, args []string) error {
			env := core.Detect()
			if stAll {
				all, err := core.List(env)
				if err != nil {
					return err
				}
				running, err := core.ListRunning(env)
				if err != nil {
					return err
				}
				runningByName := make(map[string]core.ProcInfo, len(running))
				for _, proc := range running {
					if proc.Name != "" {
						runningByName[proc.Name] = proc
					}
				}
				if len(all) == 0 {
					fmt.Println("(no avds)")
					return nil
				}
				for _, info := range all {
					proc, ok := runningByName[info.Name]
					state := "stopped"
					serial := "-"
					port := "-"
					pid := "-"
					if ok {
						state = "booting"
						if proc.Booted {
							state = "ready"
						}
						serial = proc.Serial
						port = fmt.Sprintf("%d", proc.Port)
						pid = fmt.Sprintf("%d", proc.PID)
					}
					fmt.Printf("%-18s %-14s port=%-5s pid=%-7s %s\n", info.Name, serial, port, pid, state)
				}
				return nil
			}

			if stName == "" && stSerial == "" {
				return fmt.Errorf("use --name, --serial, or --all")
			}

			procs, err := core.ListRunning(env)
			if err != nil {
				return err
			}

			var pick *core.ProcInfo
			for _, p := range procs {
				if (stName != "" && p.Name == stName) || (stSerial != "" && p.Serial == stSerial) {
					pick = &p
					break
				}
			}
			if pick == nil {
				return fmt.Errorf("not found (name=%q serial=%q)", stName, stSerial)
			}
			fmt.Printf("Name:   %s\nSerial: %s\nPort:   %d\nPID:    %d\nBooted: %v\n", pick.Name, pick.Serial, pick.Port, pick.PID, pick.Booted)
			return nil
		},
	}
	statusCmd.Flags().StringVar(&stName, "name", "", "AVD name")
	statusCmd.Flags().StringVar(&stSerial, "serial", "", "emulator serial (e.g., emulator-5582)")
	statusCmd.Flags().BoolVar(&stAll, "all", false, "list all AVDs with running state")
	root.AddCommand(statusCmd)

	// stop
	var stopName, stopSerial string
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a running emulator by --name or --serial",
		RunE: func(cmd *cobra.Command, args []string) error {
			env := core.Detect()
			if stopSerial == "" && stopName == "" {
				return fmt.Errorf("use --name or --serial")
			}
			serial := stopSerial
			if serial == "" {
				procs, err := core.ListRunning(env)
				if err != nil {
					return err
				}
				for _, p := range procs {
					if p.Name == stopName {
						serial = p.Serial
						break
					}
				}
				if serial == "" {
					return fmt.Errorf("no running emulator named %s", stopName)
				}
			}
			if err := core.StopBySerial(env, serial); err != nil {
				return err
			}
			fmt.Printf("Stopped %s\n", serial)
			return nil
		},
	}
	stopCmd.Flags().StringVar(&stopName, "name", "", "AVD name")
	stopCmd.Flags().StringVar(&stopSerial, "serial", "", "emulator serial (e.g., emulator-5582)")
	root.AddCommand(stopCmd)

	// cleanup
	var cleanupForce bool
	var cleanupDryRun bool
	cleanupCmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Detect and optionally clean orphaned emulators and clones",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun := !cleanupForce
			if cleanupDryRun {
				dryRun = true
			}
			report, err := core.CleanupOrphans(env, !dryRun)
			if err != nil {
				return err
			}
			if len(report.OrphanedProcesses) == 0 && len(report.OrphanedAVDs) == 0 {
				fmt.Println("No orphans found.")
				return nil
			}
			if dryRun {
				fmt.Printf("Orphans detected (%d processes, %d AVDs). Use --force to clean.\n",
					len(report.OrphanedProcesses), len(report.OrphanedAVDs))
			} else {
				fmt.Printf("Orphans cleaned (%d processes, %d AVDs).\n",
					len(report.OrphanedProcesses), len(report.OrphanedAVDs))
			}
			for _, proc := range report.OrphanedProcesses {
				fmt.Printf("process: serial=%s name=%s port=%d pid=%d\n", proc.Serial, proc.Name, proc.Port, proc.PID)
			}
			for _, info := range report.OrphanedAVDs {
				fmt.Printf("avd: name=%s path=%s\n", info.Name, info.Path)
			}
			return nil
		},
	}
	cleanupCmd.Flags().BoolVar(&cleanupForce, "force", false, "delete orphans")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "show what would be cleaned")
	root.AddCommand(cleanupCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
