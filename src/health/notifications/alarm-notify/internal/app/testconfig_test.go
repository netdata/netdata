// SPDX-License-Identifier: GPL-3.0-or-later
package app

import (
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

// Test documents exercise the public YAML shape independently of provider structs.
type testConfig struct {
	Version      int                       `yaml:"version"`
	Destinations map[string]map[string]any `yaml:"destinations"`
	Routing      notifier.Routing          `yaml:"routing,omitempty"`
}

func fixturePath(name string) string {
	provider, _, _ := strings.Cut(name, "-")
	return filepath.Join("..", "providers", provider, "testdata", name)
}
