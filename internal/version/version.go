// Package version exposes build version metadata.
package version

import (
	"runtime/debug"
	"strings"
)

// Value is overridden by release builds using -ldflags.
var Value = "dev"

// Current returns the release tag, including its leading v.
func Current() string {
	if Value != "" && Value != "dev" {
		return normalize(Value)
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return normalize(info.Main.Version)
	}
	return "dev"
}

func normalize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "dev" {
		return "dev"
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value
}
