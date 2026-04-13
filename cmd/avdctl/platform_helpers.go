package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	core "github.com/forkbombeu/avdctl/internal/avd"
	ioscore "github.com/forkbombeu/avdctl/internal/ios"
)

var (
	androidDeleteFn      = core.Delete
	androidListFn        = core.List
	androidListRunningFn = core.ListRunning
	androidRunAVDFn      = func(env core.Env, name string) (string, error) { return core.RunAVD(env, name) }
	androidStartOnPortFn = func(env core.Env, name string, port int) (*exec.Cmd, string, string, error) {
		return core.StartEmulatorOnPort(env, name, port)
	}
	androidStopBySerialFn = core.StopBySerial

	iosDeleteFn          = ioscore.Delete
	iosEnsureSupportedFn = ioscore.EnsureSupported
	iosFindFn            = ioscore.Find
	iosListFn            = ioscore.List
	iosListRunningFn     = ioscore.ListRunning
	iosRunFn             = ioscore.Run
	iosStopFn            = ioscore.Stop
)

type platformListOutput struct {
	Android []core.Info    `json:"android"`
	IOS     []ioscore.Info `json:"ios"`
}

type platformPSOutput struct {
	Android []core.ProcInfo    `json:"android"`
	IOS     []ioscore.ProcInfo `json:"ios"`
}

func encodeJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printAndroidList(infos []core.Info) {
	if len(infos) == 0 {
		fmt.Println("(no emulators)")
		return
	}
	for _, info := range infos {
		fmt.Printf("%-18s %s\n  userdata: %s (%d bytes)\n", info.Name, info.Path, info.Userdata, info.SizeBytes)
	}
}

func printIOSList(infos []ioscore.Info) {
	if len(infos) == 0 {
		fmt.Println("(no simulators)")
		return
	}
	for _, info := range infos {
		fmt.Printf("%-24s %-12s %s\n  runtime: %s\n  path: %s (%d bytes)\n", info.Name, info.State, info.UDID, info.Runtime, info.Path, info.SizeBytes)
	}
}

func printAndroidPS(procs []core.ProcInfo) {
	if len(procs) == 0 {
		fmt.Println("(no emulators)")
		return
	}
	for _, proc := range procs {
		state := "booting"
		if proc.Booted {
			state = "ready"
		}
		fmt.Printf("%-18s %-14s port=%-5d pid=%-7d %s\n", proc.Name, proc.Serial, proc.Port, proc.PID, state)
	}
}

func printIOSPS(procs []ioscore.ProcInfo) {
	if len(procs) == 0 {
		fmt.Println("(no simulators)")
		return
	}
	for _, proc := range procs {
		fmt.Printf("%-24s %-36s %s\n", proc.Name, proc.UDID, proc.Runtime)
	}
}

func printAndroidStatusAll(env core.Env) error {
	all, err := androidListFn(env)
	if err != nil {
		return err
	}
	running, err := androidListRunningFn(env)
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

func printIOSStatusAll(env ioscore.Env) error {
	allInfos, err := iosListIfSupported(env)
	if err != nil {
		return err
	}
	if len(allInfos) == 0 {
		fmt.Println("(no simulators)")
		return nil
	}
	for _, info := range allInfos {
		fmt.Printf("%-24s %-12s %s\n", info.Name, info.State, info.UDID)
	}
	return nil
}

func printAndroidStatus(env core.Env, name, serial string) error {
	if name == "" && serial == "" {
		return fmt.Errorf("use --name, --serial, or --all")
	}
	procs, err := androidListRunningFn(env)
	if err != nil {
		return err
	}
	for _, proc := range procs {
		if (name != "" && proc.Name == name) || (serial != "" && proc.Serial == serial) {
			fmt.Printf("Name:   %s\nSerial: %s\nPort:   %d\nPID:    %d\nBooted: %v\n", proc.Name, proc.Serial, proc.Port, proc.PID, proc.Booted)
			return nil
		}
	}
	return fmt.Errorf("not found (name=%q serial=%q)", name, serial)
}

func printIOSStatus(env ioscore.Env, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("use --name, --udid, or --all")
	}
	info, err := iosFindFn(env, ref)
	if err != nil {
		return err
	}
	fmt.Printf("Name:    %s\nUDID:    %s\nRuntime: %s\nState:   %s\nPath:    %s\n", info.Name, info.UDID, info.Runtime, info.State, info.Path)
	return nil
}

func runAndroidWithOutput(env core.Env, name string, port int) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}
	if port > 0 {
		if port%2 != 0 {
			return fmt.Errorf("--port must be even")
		}
		_, _, logPath, err := androidStartOnPortFn(env, name, port)
		if err != nil {
			return err
		}
		fmt.Printf("Started %s on emulator-%d (log: %s)\n", name, port, logPath)
		return nil
	}
	_, err := androidRunAVDFn(env, name)
	return err
}

func runIOSWithOutput(env ioscore.Env, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("--name is required")
	}
	proc, err := iosRunFn(env, ref)
	if err != nil {
		return err
	}
	fmt.Printf("Started %s on %s\n", proc.Name, proc.UDID)
	return nil
}

func stopAndroidWithOutput(env core.Env, name, serial string) error {
	if serial == "" && name == "" {
		return fmt.Errorf("use --name or --serial")
	}
	resolvedSerial := serial
	if resolvedSerial == "" {
		procs, err := androidListRunningFn(env)
		if err != nil {
			return err
		}
		for _, proc := range procs {
			if proc.Name == name {
				resolvedSerial = proc.Serial
				break
			}
		}
		if resolvedSerial == "" {
			return fmt.Errorf("no running emulator named %s", name)
		}
	}
	if err := androidStopBySerialFn(env, resolvedSerial); err != nil {
		return err
	}
	fmt.Printf("Stopped %s\n", resolvedSerial)
	return nil
}

func stopIOSWithOutput(env ioscore.Env, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("use --name or --udid")
	}
	if err := iosStopFn(env, ref); err != nil {
		return err
	}
	fmt.Printf("Stopped %s\n", ref)
	return nil
}

func deleteAndroidWithOutput(env core.Env, name string) error {
	return androidDeleteFn(env, name)
}

func deleteIOSWithOutput(env ioscore.Env, ref string) error {
	if err := iosDeleteFn(env, ref); err != nil {
		return err
	}
	fmt.Printf("Deleted %s\n", ref)
	return nil
}

func iosListIfSupported(env ioscore.Env) ([]ioscore.Info, error) {
	if err := iosEnsureSupportedFn(); err != nil {
		return nil, nil
	}
	return iosListFn(env)
}

func iosListRunningIfSupported(env ioscore.Env) ([]ioscore.ProcInfo, error) {
	if err := iosEnsureSupportedFn(); err != nil {
		return nil, nil
	}
	return iosListRunningFn(env)
}

func findAndroidInfo(env core.Env, ref string) (core.Info, bool, error) {
	infos, err := androidListFn(env)
	if err != nil {
		return core.Info{}, false, err
	}
	for _, info := range infos {
		if info.Name == ref {
			return info, true, nil
		}
	}
	return core.Info{}, false, nil
}

func findIOSInfo(env ioscore.Env, ref string) (ioscore.Info, bool, error) {
	infos, err := iosListIfSupported(env)
	if err != nil {
		return ioscore.Info{}, false, err
	}
	for _, info := range infos {
		if info.Name == ref || info.UDID == ref {
			return info, true, nil
		}
	}
	return ioscore.Info{}, false, nil
}

func resolveTargetPlatform(androidFound bool, iosFound bool, ref string, androidErr, iosErr error) (string, error) {
	switch {
	case androidFound:
		return "android", nil
	case iosFound:
		return "ios", nil
	}

	var issues []string
	if androidErr != nil {
		issues = append(issues, fmt.Sprintf("android lookup failed: %v", androidErr))
	}
	if iosErr != nil {
		issues = append(issues, fmt.Sprintf("ios lookup failed: %v", iosErr))
	}
	if len(issues) > 0 {
		return "", fmt.Errorf("could not resolve %q: %s", ref, strings.Join(issues, "; "))
	}
	return "", fmt.Errorf("device %q not found on android or ios", ref)
}

func restorePlatformHelperStubs() func() {
	prevAndroidDelete := androidDeleteFn
	prevAndroidList := androidListFn
	prevAndroidListRunning := androidListRunningFn
	prevAndroidRunAVD := androidRunAVDFn
	prevAndroidStartOnPort := androidStartOnPortFn
	prevAndroidStopBySerial := androidStopBySerialFn
	prevIOSEnsureSupported := iosEnsureSupportedFn
	prevIOSDelete := iosDeleteFn
	prevIOSFind := iosFindFn
	prevIOSList := iosListFn
	prevIOSListRunning := iosListRunningFn
	prevIOSRun := iosRunFn
	prevIOSStop := iosStopFn

	return func() {
		androidDeleteFn = prevAndroidDelete
		androidListFn = prevAndroidList
		androidListRunningFn = prevAndroidListRunning
		androidRunAVDFn = prevAndroidRunAVD
		androidStartOnPortFn = prevAndroidStartOnPort
		androidStopBySerialFn = prevAndroidStopBySerial
		iosEnsureSupportedFn = prevIOSEnsureSupported
		iosDeleteFn = prevIOSDelete
		iosFindFn = prevIOSFind
		iosListFn = prevIOSList
		iosListRunningFn = prevIOSListRunning
		iosRunFn = prevIOSRun
		iosStopFn = prevIOSStop
	}
}
