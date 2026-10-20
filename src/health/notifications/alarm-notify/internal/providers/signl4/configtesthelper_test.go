// SPDX-License-Identifier: GPL-3.0-or-later
package signl4

import (
	"io"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"gopkg.in/yaml.v3"
)

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "signl4", func(cfg Config) error { _, err := New(cfg, nil); return err })
}
func marshalConfig(doc testutil.Document[Config]) ([]byte, error) {
	destinations := make(map[string]any, len(doc.Destinations))
	for name, cfg := range doc.Destinations {
		destinations[name] = struct {
			Type   string `yaml:"type"`
			Config `yaml:",inline"`
		}{"signl4", cfg}
	}
	return yaml.Marshal(map[string]any{"version": doc.Version, "destinations": destinations})
}
