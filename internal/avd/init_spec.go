// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

type InitKind string

const (
	InitKindAVD     InitKind = "avd"
	InitKindRedroid InitKind = "redroid"
)

// InitSpec represents a parsed init descriptor string.
//
// Supported formats:
//   - AVD full:  "system-images;android-35;google_apis_playstore;x86_64;pixel_6"
//   - AVD short: "android-35;google_apis_playstore;x86_64;pixel_6"
//   - Redroid:   "redroid15;android-35;google_apis_playstore;ARM64;logged-out"
type InitSpec struct {
	Raw  string
	Kind InitKind

	// AVD fields
	SystemImage string
	Device      string

	// Redroid fields
	ImageName      string
	AndroidVersion string
	Flavor         string
	Arch           string
	Profile        string
}

func ParseInitSpec(input string) (InitSpec, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return InitSpec{}, fmt.Errorf("empty init spec")
	}

	parts := strings.Split(raw, ";")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return InitSpec{}, fmt.Errorf("invalid init spec %q: empty segment at position %d", raw, i+1)
		}
	}

	if len(parts) == 5 && strings.EqualFold(parts[0], "system-images") {
		return InitSpec{
			Raw:         raw,
			Kind:        InitKindAVD,
			SystemImage: strings.Join(parts[:4], ";"),
			Device:      parts[4],
		}, nil
	}

	if len(parts) == 4 && strings.HasPrefix(strings.ToLower(parts[0]), "android-") {
		return InitSpec{
			Raw:         raw,
			Kind:        InitKindAVD,
			SystemImage: strings.Join([]string{"system-images", parts[0], parts[1], parts[2]}, ";"),
			Device:      parts[3],
		}, nil
	}

	if len(parts) == 5 && strings.HasPrefix(strings.ToLower(parts[0]), "redroid") {
		return InitSpec{
			Raw:            raw,
			Kind:           InitKindRedroid,
			ImageName:      parts[0],
			AndroidVersion: parts[1],
			Flavor:         parts[2],
			Arch:           parts[3],
			Profile:        parts[4],
		}, nil
	}

	return InitSpec{}, fmt.Errorf(
		"unsupported init spec %q: expected one of "+
			"\"system-images;android-<api>;<flavor>;<arch>;<device>\", "+
			"\"android-<api>;<flavor>;<arch>;<device>\", "+
			"or \"redroid<version>;android-<api>;<flavor>;<arch>;<profile>\"",
		raw,
	)
}

// DefaultInitName returns a deterministic, shell-safe resource name derived from a spec.
func DefaultInitName(spec string) string {
	raw := strings.TrimSpace(strings.ToLower(spec))
	if raw == "" {
		return "init"
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "init"
	}
	if len(name) > 48 {
		name = strings.Trim(name[:48], "-")
		if name == "" {
			name = "init"
		}
	}

	sum := sha1.Sum([]byte(spec))
	return fmt.Sprintf("%s-%s", name, hex.EncodeToString(sum[:])[:8])
}
