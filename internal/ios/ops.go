// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package ios

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Info struct {
	Name      string `json:"name"`
	UDID      string `json:"udid"`
	Runtime   string `json:"runtime"`
	State     string `json:"state"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type ProcInfo struct {
	Name    string `json:"name"`
	UDID    string `json:"udid"`
	Runtime string `json:"runtime"`
	State   string `json:"state"`
	Booted  bool   `json:"booted"`
}

type simctlList struct {
	DeviceTypes []simDeviceType        `json:"devicetypes"`
	Runtimes    []simRuntime           `json:"runtimes"`
	Devices     map[string][]simDevice `json:"devices"`
}

type simDeviceType struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

type simRuntime struct {
	Name        string `json:"name"`
	Identifier  string `json:"identifier"`
	Version     string `json:"version"`
	IsAvailable bool   `json:"isAvailable"`
}

type simDevice struct {
	Name                 string `json:"name"`
	UDID                 string `json:"udid"`
	State                string `json:"state"`
	IsAvailable          bool   `json:"isAvailable"`
	DataPath             string `json:"dataPath"`
	DataPathSize         int64  `json:"dataPathSize"`
	DeviceTypeIdentifier string `json:"deviceTypeIdentifier"`
}

func List(env Env) ([]Info, error) {
	if err := EnsureSupported(); err != nil {
		return nil, err
	}
	payload, err := loadList(env)
	if err != nil {
		return nil, err
	}
	infos := make([]Info, 0)
	for runtimeID, devices := range payload.Devices {
		runtimeName := runtimeDisplayName(payload.Runtimes, runtimeID)
		for _, device := range devices {
			if !device.IsAvailable {
				continue
			}
			infos = append(infos, Info{
				Name:      device.Name,
				UDID:      device.UDID,
				Runtime:   runtimeName,
				State:     device.State,
				Path:      device.DataPath,
				SizeBytes: device.DataPathSize,
			})
		}
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Name != infos[j].Name {
			return infos[i].Name < infos[j].Name
		}
		return infos[i].UDID < infos[j].UDID
	})
	return infos, nil
}

func ListRunning(env Env) ([]ProcInfo, error) {
	if err := EnsureSupported(); err != nil {
		return nil, err
	}
	payload, err := loadList(env)
	if err != nil {
		return nil, err
	}
	procs := make([]ProcInfo, 0)
	for runtimeID, devices := range payload.Devices {
		runtimeName := runtimeDisplayName(payload.Runtimes, runtimeID)
		for _, device := range devices {
			if !device.IsAvailable || !strings.EqualFold(device.State, "Booted") {
				continue
			}
			procs = append(procs, ProcInfo{
				Name:    device.Name,
				UDID:    device.UDID,
				Runtime: runtimeName,
				State:   device.State,
				Booted:  true,
			})
		}
	}
	sort.Slice(procs, func(i, j int) bool {
		if procs[i].Name != procs[j].Name {
			return procs[i].Name < procs[j].Name
		}
		return procs[i].UDID < procs[j].UDID
	})
	return procs, nil
}

func InitBase(env Env, name, runtimeID, deviceTypeID string) (Info, error) {
	if err := EnsureSupported(); err != nil {
		return Info{}, err
	}
	if strings.TrimSpace(name) == "" {
		return Info{}, errors.New("empty simulator name")
	}
	payload, err := loadList(env)
	if err != nil {
		return Info{}, err
	}
	if strings.TrimSpace(runtimeID) == "" {
		runtimeID, err = defaultRuntime(payload.Runtimes)
		if err != nil {
			return Info{}, err
		}
	}
	if strings.TrimSpace(deviceTypeID) == "" {
		deviceTypeID, err = defaultDeviceType(payload.DeviceTypes)
		if err != nil {
			return Info{}, err
		}
	}
	out, errOut, err := runCommandOutput(env.Context, nil, env.Xcrun, "simctl", "create", name, deviceTypeID, runtimeID)
	if err != nil {
		return Info{}, fmt.Errorf("xcrun simctl create failed: %v\n%s%s", err, out, errOut)
	}
	udid := strings.TrimSpace(out)
	return Find(env, udid)
}

func Clone(env Env, sourceRef, newName string) (Info, error) {
	if err := EnsureSupported(); err != nil {
		return Info{}, err
	}
	if strings.TrimSpace(sourceRef) == "" {
		return Info{}, errors.New("empty source simulator reference")
	}
	if strings.TrimSpace(newName) == "" {
		return Info{}, errors.New("empty clone simulator name")
	}
	device, _, err := resolveDevice(env, sourceRef)
	if err != nil {
		return Info{}, err
	}
	if strings.EqualFold(device.State, "Booted") {
		return Info{}, fmt.Errorf("source simulator %q must be shutdown before cloning", sourceRef)
	}
	out, errOut, runErr := runCommandOutput(env.Context, nil, env.Xcrun, "simctl", "clone", device.UDID, newName)
	if runErr != nil {
		return Info{}, fmt.Errorf("xcrun simctl clone failed: %v\n%s%s", runErr, out, errOut)
	}
	udid := strings.TrimSpace(out)
	if udid != "" {
		return Find(env, udid)
	}
	return Find(env, newName)
}

func Run(env Env, ref string) (ProcInfo, error) {
	if err := EnsureSupported(); err != nil {
		return ProcInfo{}, err
	}
	device, runtimeName, err := resolveDevice(env, ref)
	if err != nil {
		return ProcInfo{}, err
	}
	if !strings.EqualFold(device.State, "Booted") {
		if out, errOut, runErr := runCommandOutput(env.Context, nil, env.Xcrun, "simctl", "boot", device.UDID); runErr != nil {
			return ProcInfo{}, fmt.Errorf("xcrun simctl boot failed: %v\n%s%s", runErr, out, errOut)
		}
	}
	if out, errOut, runErr := runCommandOutput(env.Context, nil, env.Xcrun, "simctl", "bootstatus", device.UDID, "-b"); runErr != nil {
		return ProcInfo{}, fmt.Errorf("xcrun simctl bootstatus failed: %v\n%s%s", runErr, out, errOut)
	}
	return ProcInfo{
		Name:    device.Name,
		UDID:    device.UDID,
		Runtime: runtimeName,
		State:   "Booted",
		Booted:  true,
	}, nil
}

func Stop(env Env, ref string) error {
	if err := EnsureSupported(); err != nil {
		return err
	}
	device, _, err := resolveDevice(env, ref)
	if err != nil {
		return err
	}
	out, errOut, runErr := runCommandOutput(env.Context, nil, env.Xcrun, "simctl", "shutdown", device.UDID)
	if runErr != nil {
		return fmt.Errorf("xcrun simctl shutdown failed: %v\n%s%s", runErr, out, errOut)
	}
	return nil
}

func Delete(env Env, ref string) error {
	if err := EnsureSupported(); err != nil {
		return err
	}
	device, _, err := resolveDevice(env, ref)
	if err != nil {
		return err
	}
	out, errOut, runErr := runCommandOutput(env.Context, nil, env.Xcrun, "simctl", "delete", device.UDID)
	if runErr != nil {
		return fmt.Errorf("xcrun simctl delete failed: %v\n%s%s", runErr, out, errOut)
	}
	return nil
}

func Find(env Env, ref string) (Info, error) {
	if err := EnsureSupported(); err != nil {
		return Info{}, err
	}
	device, runtimeName, err := resolveDevice(env, ref)
	if err != nil {
		return Info{}, err
	}
	return Info{
		Name:      device.Name,
		UDID:      device.UDID,
		Runtime:   runtimeName,
		State:     device.State,
		Path:      device.DataPath,
		SizeBytes: device.DataPathSize,
	}, nil
}

func loadList(env Env) (simctlList, error) {
	out, errOut, err := runCommandOutput(env.Context, nil, env.Xcrun, "simctl", "list", "--json")
	if err != nil {
		return simctlList{}, fmt.Errorf("xcrun simctl list failed: %v\n%s%s", err, out, errOut)
	}
	var payload simctlList
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return simctlList{}, fmt.Errorf("decode simctl list: %w", err)
	}
	return payload, nil
}

func resolveDevice(env Env, ref string) (simDevice, string, error) {
	payload, err := loadList(env)
	if err != nil {
		return simDevice{}, "", err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return simDevice{}, "", errors.New("empty simulator reference")
	}
	var matches []struct {
		device  simDevice
		runtime string
	}
	for runtimeID, devices := range payload.Devices {
		runtimeName := runtimeDisplayName(payload.Runtimes, runtimeID)
		for _, device := range devices {
			if !device.IsAvailable {
				continue
			}
			if device.UDID == ref || device.Name == ref {
				matches = append(matches, struct {
					device  simDevice
					runtime string
				}{
					device:  device,
					runtime: runtimeName,
				})
			}
		}
	}
	switch len(matches) {
	case 0:
		return simDevice{}, "", fmt.Errorf("simulator %q not found", ref)
	case 1:
		return matches[0].device, matches[0].runtime, nil
	default:
		return simDevice{}, "", fmt.Errorf("simulator %q is ambiguous; use UDID", ref)
	}
}

func runtimeDisplayName(runtimes []simRuntime, runtimeID string) string {
	for _, runtime := range runtimes {
		if runtime.Identifier == runtimeID {
			return runtime.Name
		}
	}
	return runtimeID
}

func defaultRuntime(runtimes []simRuntime) (string, error) {
	type candidate struct {
		id      string
		version []int
	}
	var candidates []candidate
	for _, runtime := range runtimes {
		if !runtime.IsAvailable {
			continue
		}
		if !strings.Contains(strings.ToLower(runtime.Identifier), ".ios-") {
			continue
		}
		candidates = append(candidates, candidate{
			id:      runtime.Identifier,
			version: parseRuntimeVersion(runtime),
		})
	}
	if len(candidates) == 0 {
		return "", errors.New("no available iOS runtime found")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareVersion(candidates[i].version, candidates[j].version) > 0
	})
	return candidates[0].id, nil
}

func parseRuntimeVersion(runtime simRuntime) []int {
	if runtime.Version != "" {
		return parseVersionParts(runtime.Version)
	}
	parts := strings.Split(runtime.Identifier, ".")
	if len(parts) == 0 {
		return nil
	}
	last := parts[len(parts)-1]
	last = strings.TrimPrefix(last, "iOS-")
	last = strings.ReplaceAll(last, "-", ".")
	return parseVersionParts(last)
}

func parseVersionParts(value string) []int {
	raw := strings.Split(value, ".")
	out := make([]int, 0, len(raw))
	for _, part := range raw {
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func compareVersion(a, b []int) int {
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		var ai, bi int
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		switch {
		case ai > bi:
			return 1
		case ai < bi:
			return -1
		}
	}
	return 0
}

func defaultDeviceType(deviceTypes []simDeviceType) (string, error) {
	preferred := []string{
		"iPhone 16 Pro",
		"iPhone 16",
		"iPhone 15 Pro",
		"iPhone 15",
		"iPhone 14 Pro",
		"iPhone 14",
	}
	byName := make(map[string]string, len(deviceTypes))
	var iphoneFallback []simDeviceType
	for _, deviceType := range deviceTypes {
		byName[deviceType.Name] = deviceType.Identifier
		if strings.Contains(deviceType.Name, "iPhone") {
			iphoneFallback = append(iphoneFallback, deviceType)
		}
	}
	for _, name := range preferred {
		if identifier := byName[name]; identifier != "" {
			return identifier, nil
		}
	}
	if len(iphoneFallback) == 0 {
		return "", errors.New("no iPhone simulator type found")
	}
	sort.Slice(iphoneFallback, func(i, j int) bool {
		return iphoneFallback[i].Name > iphoneFallback[j].Name
	})
	return iphoneFallback[0].Identifier, nil
}
