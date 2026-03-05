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
//   - Redroid:   "redroid15;android-35;google_apis_playstore;arm64-v8a;logged-out"
type InitSpec struct {
	Raw  string
	Kind InitKind

	// Google system-image coordinates.
	APILevel string
	Tag      string
	ABI      string

	// AVD fields.
	SystemImage string
	Device      string

	// Redroid fields.
	Profile           string
	RedroidImage      string
	RedroidDataTarURL string
}

type redroidCatalogEntry struct {
	Image            string
	DataTarByProfile map[string]string
}

// hardcodedRedroidCatalog maps redroid descriptor keys to image and profile data-tars.
var hardcodedRedroidCatalog = map[string]redroidCatalogEntry{
	normalizeInitKey("redroid15;android-35;google_apis_playstore;arm64-v8a"): {
		Image: "magsafe/redroid15gappsmagisk:latest",
		DataTarByProfile: map[string]string{
			normalizeInitKey("logged-out"):         "https://example.com/redroid/android-35/logged-out.tar",
			normalizeInitKey("fb-logged-internal"): "https://example.com/redroid/android-35/fb-logged-internal.tar",
		},
	},
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
			APILevel:    parts[1],
			Tag:         parts[2],
			ABI:         parts[3],
			SystemImage: strings.Join(parts[:4], ";"),
			Device:      parts[4],
		}, nil
	}

	if len(parts) == 4 && strings.HasPrefix(strings.ToLower(parts[0]), "android-") {
		return InitSpec{
			Raw:         raw,
			Kind:        InitKindAVD,
			APILevel:    parts[0],
			Tag:         parts[1],
			ABI:         parts[2],
			SystemImage: strings.Join([]string{"system-images", parts[0], parts[1], parts[2]}, ";"),
			Device:      parts[3],
		}, nil
	}

	if len(parts) == 5 && strings.HasPrefix(strings.ToLower(parts[0]), "redroid") {
		redroidKey := strings.Join(parts[:4], ";")
		image, dataTarURL, ok := resolveRedroidMapping(redroidKey, parts[4])
		if !ok {
			return InitSpec{}, fmt.Errorf("unsupported redroid spec %q: unknown mapping for %q and profile %q", raw, redroidKey, parts[4])
		}
		systemImage := strings.Join([]string{"system-images", parts[1], parts[2], parts[3]}, ";")
		return InitSpec{
			Raw:               raw,
			Kind:              InitKindRedroid,
			APILevel:          parts[1],
			Tag:               parts[2],
			ABI:               parts[3],
			SystemImage:       systemImage,
			Profile:           parts[4],
			RedroidImage:      image,
			RedroidDataTarURL: dataTarURL,
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

func resolveRedroidMapping(redroidKey, profile string) (string, string, bool) {
	entry, ok := hardcodedRedroidCatalog[normalizeInitKey(redroidKey)]
	if !ok {
		return "", "", false
	}
	url, ok := entry.DataTarByProfile[normalizeInitKey(profile)]
	if !ok {
		return "", "", false
	}
	return entry.Image, url, true
}

func normalizeInitKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
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
