package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	core "github.com/forkbombeu/avdctl/internal/avd"
	ioscore "github.com/forkbombeu/avdctl/internal/ios"
	"github.com/forkbombeu/avdctl/internal/remoteavdctl"
	"github.com/forkbombeu/avdctl/pkg/redroidmanager"
	"github.com/spf13/cobra"
)

func newRootCommand(version string) *cobra.Command {
	androidEnv := core.Detect()
	iosEnv := ioscore.Detect()
	sshTarget := strings.TrimSpace(androidEnv.SSHTarget)
	sshArgs := append([]string(nil), androidEnv.SSHArgs...)

	root := &cobra.Command{
		Use:   "avdctl",
		Short: "Manage Android emulators and iOS simulators",
		Long: `Manage Android emulators and iOS simulators.

Platform-aware commands support explicit platform subcommands:
  avdctl <command> android ...
  avdctl <command> ios ...

If no platform is specified:
  - list and ps include both Android and iOS devices
  - status --all includes both Android and iOS devices
  - run, status, stop, and delete auto-detect the platform by name/reference
  - when Android and iOS share the same name, Android wins
  - init-base and clone still default to Android

Shared platform-aware commands:
  list, init-base, run, clone, delete, ps, status, stop

Android-only commands:
	save-golden, prewarm, customize-start, customize-finish, bake-apk,
  stop-bluetooth, cleanup
`,
		Example: `  avdctl list
  avdctl run --name base-a35
  avdctl run ios --name base-ios
  avdctl clone --base base-a35 --name w-demo --golden ~/avd-golden/base-a35
  avdctl clone ios --base base-ios --name ios-demo`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if shouldDelegateOverSSH(cmd, sshTarget) {
				remoteArgs := stripSSHFlags(os.Args[1:])
				if err := runRemoteAVDCtl(sshTarget, sshArgs, remoteArgs); err != nil {
					return err
				}
				return errRemoteDelegated
			}
			return nil
		},
	}
	root.PersistentFlags().StringVar(&sshTarget, "ssh", "", "SSH target (user@host) to run tool commands remotely")
	root.PersistentFlags().StringArrayVar(&sshArgs, "ssh-arg", sshArgs, "Extra ssh args (repeatable, e.g. --ssh-arg=-i --ssh-arg=~/.ssh/key)")

	root.AddCommand(newVersionCommand(root, version))
	root.AddCommand(newPlatformListCommand(androidEnv, iosEnv))
	root.AddCommand(newPlatformInitBaseCommand(androidEnv, iosEnv))
	root.AddCommand(newPlatformRunCommand(androidEnv, iosEnv))
	root.AddCommand(newPlatformCloneCommand(androidEnv, iosEnv))
	root.AddCommand(newPlatformDeleteCommand(androidEnv, iosEnv))
	root.AddCommand(newPlatformPSCommand(androidEnv, iosEnv))
	root.AddCommand(newPlatformStatusCommand(androidEnv, iosEnv))
	root.AddCommand(newPlatformStopCommand(androidEnv, iosEnv))
	root.AddCommand(newAndroidSaveGoldenCommand(androidEnv))
	root.AddCommand(newAndroidPrewarmCommand(androidEnv))
	root.AddCommand(newAndroidCustomizeStartCommand(androidEnv))
	root.AddCommand(newAndroidCustomizeFinishCommand(androidEnv))
	root.AddCommand(newAndroidBakeCommand(androidEnv))
	root.AddCommand(newAndroidStopBluetoothCommand(androidEnv))
	root.AddCommand(newAndroidCleanupCommand(androidEnv))
	root.AddCommand(newRedroidCommand())

	return root
}

func newVersionCommand(root *cobra.Command, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print binary version",
		Run: func(cmd *cobra.Command, args []string) {
			v := version
			if strings.TrimSpace(v) == "" {
				v = "dev"
			}
			fmt.Fprint(os.Stderr, colophon)
			fmt.Fprintln(os.Stderr, root.Short)
			fmt.Fprintln(os.Stdout, v)
		},
	}
}

func newPlatformListCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	var listJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Android and iOS devices, or use `list android|ios`",
		RunE: func(cmd *cobra.Command, args []string) error {
			androidInfos, err := androidListFn(androidEnv)
			if err != nil {
				return err
			}
			iosInfos, err := iosListIfSupported(iosEnv)
			if err != nil {
				return err
			}
			if listJSON {
				return encodeJSON(platformListOutput{
					Android: androidInfos,
					IOS:     iosInfos,
				})
			}
			fmt.Println("Android")
			printAndroidList(androidInfos)
			if iosInfos != nil {
				fmt.Println()
				fmt.Println("iOS")
				printIOSList(iosInfos)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&listJSON, "json", false, "output JSON")
	cmd.AddCommand(newAndroidListCommand("android", androidEnv))
	cmd.AddCommand(newIOSListCommand("ios", iosEnv))
	return cmd
}

func newPlatformInitBaseCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	cmd := newAndroidInitBaseCommand("init-base", androidEnv)
	cmd.Short = "Create a base device; Android by default, or use `init-base ios`"
	cmd.AddCommand(newAndroidInitBaseCommand("android", androidEnv))
	cmd.AddCommand(newIOSInitBaseCommand("ios", iosEnv))
	return cmd
}

func newPlatformRunCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	var name string
	var port int
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start a device; auto-detect platform by name, or use `run android|ios`",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return errors.New("--name is required")
			}
			_, androidFound, androidErr := findAndroidInfo(androidEnv, name)
			_, iosFound, iosErr := findIOSInfo(iosEnv, name)
			platform, err := resolveTargetPlatform(androidFound, iosFound, name, androidErr, iosErr)
			if err != nil {
				return err
			}
			if platform == "ios" {
				if port != 0 {
					return errors.New("--port is only supported for Android emulators")
				}
				return runIOSWithOutput(iosEnv, name)
			}
			return runAndroidWithOutput(androidEnv, name, port)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Device name or iOS simulator UDID")
	cmd.Flags().IntVar(&port, "port", 0, "even TCP port to bind Android emulator (auto if omitted)")
	cmd.AddCommand(newAndroidRunCommand("android", androidEnv))
	cmd.AddCommand(newIOSRunCommand("ios", iosEnv))
	return cmd
}

func newPlatformDeleteCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete NAME_OR_UDID",
		Short: "Delete a device; auto-detect platform by ref, or use `delete android|ios`",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			_, androidFound, androidErr := findAndroidInfo(androidEnv, ref)
			_, iosFound, iosErr := findIOSInfo(iosEnv, ref)
			platform, err := resolveTargetPlatform(androidFound, iosFound, ref, androidErr, iosErr)
			if err != nil {
				return err
			}
			if platform == "ios" {
				return deleteIOSWithOutput(iosEnv, ref)
			}
			return deleteAndroidWithOutput(androidEnv, ref)
		},
	}
	cmd.AddCommand(newAndroidDeleteCommand("android", androidEnv))
	cmd.AddCommand(newIOSDeleteCommand("ios", iosEnv))
	return cmd
}

func newPlatformCloneCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	cmd := newAndroidCloneCommand("clone", androidEnv)
	cmd.Short = "Create a clone; Android by default, or use `clone ios`"
	cmd.AddCommand(newAndroidCloneCommand("android", androidEnv))
	cmd.AddCommand(newIOSCloneCommand("ios", iosEnv))
	return cmd
}

func newPlatformPSCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	var psJSON bool
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List running Android and iOS devices, or use `ps android|ios`",
		RunE: func(cmd *cobra.Command, args []string) error {
			androidProcs, err := androidListRunningFn(androidEnv)
			if err != nil {
				return err
			}
			iosProcs, err := iosListRunningIfSupported(iosEnv)
			if err != nil {
				return err
			}
			if psJSON {
				return encodeJSON(platformPSOutput{
					Android: androidProcs,
					IOS:     iosProcs,
				})
			}
			fmt.Println("Android")
			printAndroidPS(androidProcs)
			if iosProcs != nil {
				fmt.Println()
				fmt.Println("iOS")
				printIOSPS(iosProcs)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&psJSON, "json", false, "output JSON")
	cmd.AddCommand(newAndroidPSCommand("android", androidEnv))
	cmd.AddCommand(newIOSPSCommand("ios", iosEnv))
	return cmd
}

func newPlatformStatusCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	var name, serial, udid string
	var all bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show device status; auto-detect platform by ref, or use `status android|ios`",
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				fmt.Println("Android")
				if err := printAndroidStatusAll(androidEnv); err != nil {
					return err
				}
				iosInfos, err := iosListIfSupported(iosEnv)
				if err != nil {
					return err
				}
				if iosInfos != nil {
					fmt.Println()
					fmt.Println("iOS")
					return printIOSStatusAll(iosEnv)
				}
				return nil
			}
			if strings.TrimSpace(serial) != "" {
				return printAndroidStatus(androidEnv, "", serial)
			}
			if strings.TrimSpace(udid) != "" {
				return printIOSStatus(iosEnv, udid)
			}
			if strings.TrimSpace(name) == "" {
				return errors.New("use --name, --serial, --udid, or --all")
			}
			_, androidFound, androidErr := findAndroidInfo(androidEnv, name)
			_, iosFound, iosErr := findIOSInfo(iosEnv, name)
			platform, err := resolveTargetPlatform(androidFound, iosFound, name, androidErr, iosErr)
			if err != nil {
				return err
			}
			if platform == "ios" {
				return printIOSStatus(iosEnv, name)
			}
			return printAndroidStatus(androidEnv, name, "")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Device name")
	cmd.Flags().StringVar(&serial, "serial", "", "Android emulator serial (e.g., emulator-5582)")
	cmd.Flags().StringVar(&udid, "udid", "", "iOS simulator UDID")
	cmd.Flags().BoolVar(&all, "all", false, "list all Android and iOS devices with state")
	cmd.AddCommand(newAndroidStatusCommand("android", androidEnv))
	cmd.AddCommand(newIOSStatusCommand("ios", iosEnv))
	return cmd
}

func newPlatformStopCommand(androidEnv core.Env, iosEnv ioscore.Env) *cobra.Command {
	var name, serial, udid string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a device; auto-detect platform by ref, or use `stop android|ios`",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(serial) != "" {
				return stopAndroidWithOutput(androidEnv, "", serial)
			}
			if strings.TrimSpace(udid) != "" {
				return stopIOSWithOutput(iosEnv, udid)
			}
			if strings.TrimSpace(name) == "" {
				return errors.New("use --name, --serial, or --udid")
			}
			_, androidFound, androidErr := findAndroidInfo(androidEnv, name)
			_, iosFound, iosErr := findIOSInfo(iosEnv, name)
			platform, err := resolveTargetPlatform(androidFound, iosFound, name, androidErr, iosErr)
			if err != nil {
				return err
			}
			if platform == "ios" {
				return stopIOSWithOutput(iosEnv, name)
			}
			return stopAndroidWithOutput(androidEnv, name, "")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Device name")
	cmd.Flags().StringVar(&serial, "serial", "", "Android emulator serial (e.g., emulator-5582)")
	cmd.Flags().StringVar(&udid, "udid", "", "iOS simulator UDID")
	cmd.AddCommand(newAndroidStopCommand("android", androidEnv))
	cmd.AddCommand(newIOSStopCommand("ios", iosEnv))
	return cmd
}

func newAndroidListCommand(use string, env core.Env) *cobra.Command {
	var listJSON bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "List Android AVDs under ANDROID_AVD_HOME",
		RunE: func(cmd *cobra.Command, args []string) error {
			ls, err := androidListFn(env)
			if err != nil {
				return err
			}
			if listJSON {
				return encodeJSON(ls)
			}
			printAndroidList(ls)
			return nil
		},
	}
	cmd.Flags().BoolVar(&listJSON, "json", false, "output JSON")
	return cmd
}

func newIOSListCommand(use string, env ioscore.Env) *cobra.Command {
	var listJSON bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "List iOS simulators",
		RunE: func(cmd *cobra.Command, args []string) error {
			ls, err := iosListFn(env)
			if err != nil {
				return err
			}
			if listJSON {
				return encodeJSON(ls)
			}
			printIOSList(ls)
			return nil
		},
	}
	cmd.Flags().BoolVar(&listJSON, "json", false, "output JSON")
	return cmd
}

func newAndroidInitBaseCommand(use string, env core.Env) *cobra.Command {
	var baseName, sysImg, device string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Create a base Android AVD (auto-installs system image if missing)",
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
	cmd.Flags().StringVar(&baseName, "name", "base-a35", "AVD name (include API, e.g. base-a35)")
	cmd.Flags().StringVar(&sysImg, "image", "system-images;android-35;google_apis_playstore;x86_64", "System image ID")
	cmd.Flags().StringVar(&device, "device", "pixel_6", "Device profile")
	return cmd
}

func newIOSInitBaseCommand(use string, env ioscore.Env) *cobra.Command {
	var name, runtimeID, deviceType string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Create an iOS simulator",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return errors.New("--name is required")
			}
			info, err := ioscore.InitBase(env, name, runtimeID, deviceType)
			if err != nil {
				return err
			}
			fmt.Printf("Created %s (%s)\n", info.Name, info.UDID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "base-ios", "Simulator name")
	cmd.Flags().StringVar(&runtimeID, "image", "", "iOS runtime identifier (default: latest available iOS runtime)")
	cmd.Flags().StringVar(&deviceType, "device", "", "Simulator device type identifier (default: latest available iPhone)")
	return cmd
}

func newAndroidRunCommand(use string, env core.Env) *cobra.Command {
	var runName string
	var runPort int
	cmd := &cobra.Command{
		Use:   use,
		Short: "Run an Android AVD headless (no snapshots); supports parallel instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAndroidWithOutput(env, runName, runPort)
		},
	}
	cmd.Flags().StringVar(&runName, "name", "", "AVD name to run")
	cmd.Flags().IntVar(&runPort, "port", 0, "even TCP port to bind emulator (auto if omitted)")
	return cmd
}

func newIOSRunCommand(use string, env ioscore.Env) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Boot an iOS simulator",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIOSWithOutput(env, name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Simulator name or UDID to boot")
	return cmd
}

func newIOSCloneCommand(use string, env ioscore.Env) *cobra.Command {
	var base, name string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Clone a shut down iOS simulator base",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(base) == "" || strings.TrimSpace(name) == "" {
				return errors.New("--base and --name are required")
			}
			info, err := ioscore.Clone(env, base, name)
			if err != nil {
				return err
			}
			fmt.Printf("Clone ready: %s (%s)\n", info.Name, info.UDID)
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base simulator name or UDID")
	cmd.Flags().StringVar(&name, "name", "", "New clone simulator name")
	return cmd
}

func newAndroidDeleteCommand(use string, env core.Env) *cobra.Command {
	return &cobra.Command{
		Use:   use + " NAME",
		Short: "Delete an Android AVD (+ .ini)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return core.Delete(env, args[0])
		},
	}
}

func newIOSDeleteCommand(use string, env ioscore.Env) *cobra.Command {
	return &cobra.Command{
		Use:   use + " NAME_OR_UDID",
		Short: "Delete an iOS simulator",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteIOSWithOutput(env, args[0])
		},
	}
}

func newAndroidPSCommand(use string, env core.Env) *cobra.Command {
	var psJSON bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "List running Android emulators with AVD name, serial, port, PID",
		RunE: func(cmd *cobra.Command, args []string) error {
			procs, err := androidListRunningFn(env)
			if err != nil {
				return err
			}
			if psJSON {
				return encodeJSON(procs)
			}
			printAndroidPS(procs)
			return nil
		},
	}
	cmd.Flags().BoolVar(&psJSON, "json", false, "output JSON")
	return cmd
}

func newIOSPSCommand(use string, env ioscore.Env) *cobra.Command {
	var psJSON bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "List booted iOS simulators",
		RunE: func(cmd *cobra.Command, args []string) error {
			procs, err := iosListRunningFn(env)
			if err != nil {
				return err
			}
			if psJSON {
				return encodeJSON(procs)
			}
			printIOSPS(procs)
			return nil
		},
	}
	cmd.Flags().BoolVar(&psJSON, "json", false, "output JSON")
	return cmd
}

func newAndroidStatusCommand(use string, env core.Env) *cobra.Command {
	var stName, stSerial string
	var stAll bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "Show status for a running Android emulator by --name or --serial",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stAll {
				return printAndroidStatusAll(env)
			}
			return printAndroidStatus(env, stName, stSerial)
		},
	}
	cmd.Flags().StringVar(&stName, "name", "", "AVD name")
	cmd.Flags().StringVar(&stSerial, "serial", "", "emulator serial (e.g., emulator-5582)")
	cmd.Flags().BoolVar(&stAll, "all", false, "list all AVDs with running state")
	return cmd
}

func newIOSStatusCommand(use string, env ioscore.Env) *cobra.Command {
	var name, udid string
	var all bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "Show status for an iOS simulator by --name or --udid",
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				return printIOSStatusAll(env)
			}
			ref := strings.TrimSpace(name)
			if ref == "" {
				ref = strings.TrimSpace(udid)
			}
			return printIOSStatus(env, ref)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Simulator name")
	cmd.Flags().StringVar(&udid, "udid", "", "Simulator UDID")
	cmd.Flags().BoolVar(&all, "all", false, "list all simulators with state")
	return cmd
}

func newAndroidStopCommand(use string, env core.Env) *cobra.Command {
	var stopName, stopSerial string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Stop a running Android emulator by --name or --serial",
		RunE: func(cmd *cobra.Command, args []string) error {
			return stopAndroidWithOutput(env, stopName, stopSerial)
		},
	}
	cmd.Flags().StringVar(&stopName, "name", "", "AVD name")
	cmd.Flags().StringVar(&stopSerial, "serial", "", "emulator serial (e.g., emulator-5582)")
	return cmd
}

func newIOSStopCommand(use string, env ioscore.Env) *cobra.Command {
	var name, udid string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Shutdown a booted iOS simulator by --name or --udid",
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := strings.TrimSpace(name)
			if ref == "" {
				ref = strings.TrimSpace(udid)
			}
			return stopIOSWithOutput(env, ref)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Simulator name")
	cmd.Flags().StringVar(&udid, "udid", "", "Simulator UDID")
	return cmd
}

func newAndroidSaveGoldenCommand(env core.Env) *cobra.Command {
	var sgName, sgDest string
	cmd := &cobra.Command{
		Use:   "save-golden",
		Short: "Export Android AVD userdata to compressed QCOW2 golden",
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
	cmd.Flags().StringVar(&sgName, "name", "", "AVD name")
	cmd.Flags().StringVar(&sgDest, "dest", "", "Destination qcow2 (default: $AVDCTL_GOLDEN_DIR/<name>-userdata.qcow2)")
	return cmd
}

func newAndroidPrewarmCommand(env core.Env) *cobra.Command {
	var pwName, pwDest string
	var pwExtra, pwTimeout time.Duration
	cmd := &cobra.Command{
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
	cmd.Flags().StringVar(&pwName, "name", "", "AVD name")
	cmd.Flags().StringVar(&pwDest, "dest", "", "Destination qcow2 (default: $AVDCTL_GOLDEN_DIR/<name>-prewarmed.qcow2)")
	cmd.Flags().DurationVar(&pwExtra, "extra", 30*time.Second, "extra settle time after boot")
	cmd.Flags().DurationVar(&pwTimeout, "timeout", 3*time.Minute, "boot timeout")
	return cmd
}

func newAndroidCustomizeStartCommand(env core.Env) *cobra.Command {
	var csName string
	cmd := &cobra.Command{
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
	cmd.Flags().StringVar(&csName, "name", "", "AVD name")
	return cmd
}

func newAndroidCustomizeFinishCommand(env core.Env) *cobra.Command {
	var cfName, cfDest string
	cmd := &cobra.Command{
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
	cmd.Flags().StringVar(&cfName, "name", "", "AVD name")
	cmd.Flags().StringVar(&cfDest, "dest", "", "Destination directory (default: $AVDCTL_GOLDEN_DIR/<name>-custom)")
	return cmd
}

func newAndroidCloneCommand(use string, env core.Env) *cobra.Command {
	var clBase, clName, clGolden string
	cmd := &cobra.Command{
		Use:   use,
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
	cmd.Flags().StringVar(&clBase, "base", "", "Base AVD name (e.g., base-a35)")
	cmd.Flags().StringVar(&clName, "name", "", "New clone name (e.g., w-<slug>)")
	cmd.Flags().StringVar(&clGolden, "golden", "", "Path to golden directory")
	return cmd
}

func newAndroidBakeCommand(env core.Env) *cobra.Command {
	var bkBase, bkName, bkGolden, bkOut string
	var apks []string
	cmd := &cobra.Command{
		Use:   "bake-apk",
		Short: "Clone -> boot -> install APK(s) -> shutdown -> export new golden",
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
			dst2, sz2, err := core.SaveGolden(env, bkName, bkOut)
			if err != nil {
				return err
			}
			fmt.Printf("Baked clone at %s (%d bytes)\n", dst, sz)
			fmt.Printf("Exported baked golden: %s (%d bytes)\n", dst2, sz2)
			return nil
		},
	}
	cmd.Flags().StringVar(&bkBase, "base", "", "Base AVD name")
	cmd.Flags().StringVar(&bkName, "name", "", "New baked clone name (e.g., w-<slug>)")
	cmd.Flags().StringVar(&bkGolden, "golden", "", "Path to base golden qcow2")
	cmd.Flags().StringSliceVar(&apks, "apk", nil, "APK file(s) to install (repeatable)")
	cmd.Flags().StringVar(&bkOut, "dest", "", "Destination golden qcow2 for baked image")
	return cmd
}

func newAndroidStopBluetoothCommand(env core.Env) *cobra.Command {
	var stopBtName, stopBtSerial string
	cmd := &cobra.Command{
		Use:   "stop-bluetooth",
		Short: "Disable Bluetooth and scanning on a running emulator to prevent 'Bluetooth keeps stopping' errors",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stopBtSerial == "" && stopBtName == "" {
				return fmt.Errorf("either --name or --serial must be specified")
			}
			serial := stopBtSerial
			if serial == "" {
				procs, err := core.ListRunning(env)
				if err != nil {
					return err
				}
				for _, p := range procs {
					if p.Name == stopBtName {
						serial = p.Serial
						break
					}
				}
				if serial == "" {
					return fmt.Errorf("no running emulator named %s", stopBtName)
				}
			}
			if err := core.StopBluetooth(env, serial); err != nil {
				return err
			}
			fmt.Printf("Bluetooth disabled on %s\n", serial)
			return nil
		},
	}
	cmd.Flags().StringVar(&stopBtName, "name", "", "AVD name")
	cmd.Flags().StringVar(&stopBtSerial, "serial", "", "emulator serial (e.g., emulator-5582)")
	return cmd
}

func newAndroidCleanupCommand(env core.Env) *cobra.Command {
	var cleanupForce bool
	var cleanupDryRun bool
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Detect and optionally clean orphaned Android emulators and clones",
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
	cmd.Flags().BoolVar(&cleanupForce, "force", false, "delete orphans")
	cmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "show what would be cleaned")
	return cmd
}

func newRedroidCommand() *cobra.Command {
	configDir := ""
	if c, err := os.UserConfigDir(); err == nil {
		configDir = c
	}
	defaultRedroidDir := filepath.Join(configDir, "avdctl", "golden")
	defaultDataDir := filepath.Join(defaultRedroidDir, "redroid-data")
	defaultDataTar := filepath.Join(defaultRedroidDir, "redroid-data.tar")
	redroidCmd := &cobra.Command{
		Use:   "redroid",
		Short: "Manage Redroid docker containers",
	}

	var rdName, rdImage, rdDataDir, rdDataTar, rdShmSize, rdMemory, rdCPUs, rdBinderFS string
	var rdSudo bool
	var rdPort, rdWidth, rdHeight, rdDPI int
	redroidStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Restore redroid data tar and start Redroid container",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := redroidmanager.New()
			containerID, err := mgr.Start(redroidmanager.StartOptions{
				Name:     rdName,
				Image:    rdImage,
				DataDir:  rdDataDir,
				DataTar:  rdDataTar,
				UseSudo:  rdSudo,
				HostPort: rdPort,
				ShmSize:  rdShmSize,
				Memory:   rdMemory,
				CPUs:     rdCPUs,
				BinderFS: rdBinderFS,
				Width:    rdWidth,
				Height:   rdHeight,
				DPI:      rdDPI,
			})
			if err != nil {
				return err
			}
			if containerID == "" {
				fmt.Printf("Started %s\n", rdName)
				return nil
			}
			fmt.Printf("Started %s (%s)\n", rdName, containerID)
			return nil
		},
	}
	redroidStartCmd.Flags().StringVar(&rdName, "name", "redroid15", "container name")
	redroidStartCmd.Flags().StringVar(&rdImage, "image", "magsafe/redroid15gappsmagisk:latest", "docker image")
	redroidStartCmd.Flags().StringVar(&rdDataDir, "data-dir", defaultDataDir, "redroid data directory to mount at /data")
	redroidStartCmd.Flags().StringVar(&rdDataTar, "data-tar", defaultDataTar, "tar archive to restore before start")
	redroidStartCmd.Flags().BoolVar(&rdSudo, "sudo", false, "run data restore steps via sudo (or set AVDCTL_SUDO=1)")
	redroidStartCmd.Flags().IntVar(&rdPort, "port", 5555, "host port mapped to container adb port 5555")
	redroidStartCmd.Flags().StringVar(&rdShmSize, "shm-size", "3g", "docker --shm-size value")
	redroidStartCmd.Flags().StringVar(&rdMemory, "memory", "5g", "docker --memory value")
	redroidStartCmd.Flags().StringVar(&rdCPUs, "cpus", "4", "docker --cpus value")
	redroidStartCmd.Flags().StringVar(&rdBinderFS, "binderfs", "/dev/binderfs", "binderfs mount source path")
	redroidStartCmd.Flags().IntVar(&rdWidth, "width", 1080, "androidboot.redroid_width")
	redroidStartCmd.Flags().IntVar(&rdHeight, "height", 2400, "androidboot.redroid_height")
	redroidStartCmd.Flags().IntVar(&rdDPI, "dpi", 360, "androidboot.redroid_dpi")
	redroidCmd.AddCommand(redroidStartCmd)

	var rdWaitSerial string
	var rdWaitTimeout, rdWaitPoll time.Duration
	redroidWaitCmd := &cobra.Command{
		Use:   "wait",
		Short: "Wait until Redroid boot and framework services are ready",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := redroidmanager.New()
			if err := mgr.WaitForBoot(redroidmanager.WaitOptions{
				Serial:       rdWaitSerial,
				Timeout:      rdWaitTimeout,
				PollInterval: rdWaitPoll,
			}); err != nil {
				return err
			}
			fmt.Printf("Redroid ready on %s\n", rdWaitSerial)
			return nil
		},
	}
	redroidWaitCmd.Flags().StringVar(&rdWaitSerial, "serial", "127.0.0.1:5555", "adb serial, e.g. 127.0.0.1:5555")
	redroidWaitCmd.Flags().DurationVar(&rdWaitTimeout, "timeout", 3*time.Minute, "wait timeout")
	redroidWaitCmd.Flags().DurationVar(&rdWaitPoll, "poll", 1*time.Second, "poll interval")
	redroidCmd.AddCommand(redroidWaitCmd)

	var rdStopName string
	redroidStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a Redroid container by name",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(rdStopName) == "" {
				return errors.New("--name is required")
			}
			mgr := redroidmanager.New()
			if err := mgr.Stop(rdStopName); err != nil {
				return err
			}
			fmt.Printf("Stopped %s\n", rdStopName)
			return nil
		},
	}
	redroidStopCmd.Flags().StringVar(&rdStopName, "name", "", "container name")
	redroidCmd.AddCommand(redroidStopCmd)

	var rdDeleteName string
	redroidDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Force remove a Redroid container by name",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(rdDeleteName) == "" {
				return errors.New("--name is required")
			}
			mgr := redroidmanager.New()
			if err := mgr.Delete(rdDeleteName); err != nil {
				return err
			}
			fmt.Printf("Deleted %s\n", rdDeleteName)
			return nil
		},
	}
	redroidDeleteCmd.Flags().StringVar(&rdDeleteName, "name", "", "container name")
	redroidCmd.AddCommand(redroidDeleteCmd)

	return redroidCmd
}

func shouldDelegateOverSSH(cmd *cobra.Command, sshTarget string) bool {
	if strings.TrimSpace(sshTarget) == "" || cmd == nil {
		return false
	}
	if cmd == cmd.Root() {
		return false
	}
	switch cmd.Name() {
	case "version", "help", "__complete", "__completeNoDesc":
		return false
	}
	return true
}

func runRemoteAVDCtl(sshTarget string, sshArgs, avdArgs []string) error {
	return remoteavdctl.Run(
		context.Background(),
		sshTarget,
		sshArgs,
		avdArgs,
		os.Stdin,
		os.Stdout,
		os.Stderr,
		shouldAllocateTTY(sshArgs),
	)
}

func shouldAllocateTTY(sshArgs []string) bool {
	if !isTerminalFile(os.Stdin) || !isTerminalFile(os.Stdout) {
		return false
	}
	for _, arg := range sshArgs {
		switch arg {
		case "-t", "-tt", "-T":
			return false
		}
	}
	return true
}

func isTerminalFile(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func stripSSHFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--ssh" || arg == "--ssh-arg":
			if i+1 < len(args) {
				i++
			}
		case strings.HasPrefix(arg, "--ssh="),
			strings.HasPrefix(arg, "--ssh-arg="):
			continue
		default:
			out = append(out, arg)
		}
	}
	return out
}
