// Package common hosts shared helpers for WebSphere framework modules.
// SPDX-License-Identifier: GPL-3.0-or-later

package common

import (
	"fmt"
	"regexp"
	"strings"
)

// Identity captures the standard WebSphere identification labels.
type Identity struct {
	Cluster string
	Cell    string
	Node    string
	Server  string
	Edition string
	Version string
}

// Merge applies non-empty values from src onto dst.
func (i *Identity) Merge(src Identity) {
	if src.Cluster != "" {
		i.Cluster = src.Cluster
	}
	if src.Cell != "" {
		i.Cell = src.Cell
	}
	if src.Node != "" {
		i.Node = src.Node
	}
	if src.Server != "" {
		i.Server = src.Server
	}
	if src.Edition != "" {
		i.Edition = src.Edition
	}
	if src.Version != "" {
		i.Version = src.Version
	}
}

// Labels converts the identity into key-value pairs suitable for framework charts.
func (i Identity) Labels() map[string]string {
	labels := make(map[string]string, 6)
	if i.Cluster != "" {
		labels["cluster"] = i.Cluster
	}
	if i.Cell != "" {
		labels["cell"] = i.Cell
	}
	if i.Node != "" {
		labels["node"] = i.Node
	}
	if i.Server != "" {
		labels["server"] = i.Server
	}
	if i.Edition != "" {
		labels["websphere_edition"] = i.Edition
	}
	if i.Version != "" {
		labels["websphere_version"] = i.Version
	}
	return labels
}

var invalidNameChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// NormaliseName converts arbitrary strings into safe chart/instance identifiers.
func NormaliseName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	s = strings.ReplaceAll(s, " ", "_")
	s = invalidNameChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "unknown"
	}
	return strings.ToLower(s)
}

// InstanceKey builds a stable key prefix for dynamic instances.
func InstanceKey(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		cleaned = append(cleaned, NormaliseName(part))
	}
	if len(cleaned) == 0 {
		return "instance_unknown"
	}
	return strings.Join(cleaned, ".")
}

// FormatPercent applies the framework precision (value * 1000).
func FormatPercent(value float64) int64 {
	return int64(value * 1000)
}

// FormatRate transforms a per-second float into fixed precision integer.
func FormatRate(value float64) int64 {
	return int64(value * 1000)
}

// FormatBytes returns value as bytes (already integer) for completeness.
func FormatBytes(value float64) int64 {
	return int64(value)
}

// BuildMetricName concatenates segments with underscores.
func BuildMetricName(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		cleaned = append(cleaned, NormaliseName(p))
	}
	if len(cleaned) == 0 {
		return "metric"
	}
	return strings.Join(cleaned, ".")
}

// CombineErrorContext formats nested error contexts for logging.
func CombineErrorContext(ctx, msg string) string {
	if ctx == "" {
		return msg
	}
	return fmt.Sprintf("%s: %s", ctx, msg)
}
