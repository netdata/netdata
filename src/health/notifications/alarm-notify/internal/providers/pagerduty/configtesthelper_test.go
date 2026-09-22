// SPDX-License-Identifier: GPL-3.0-or-later
package pagerduty

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"gopkg.in/yaml.v3"
	"io"
)

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "pagerduty", func(cfg Config) error { _, err := New(cfg, nil); return err })
}
func marshalConfig(doc testutil.Document[Config]) ([]byte, error) {
	destinations := make(map[string]any, len(doc.Destinations))
	for name, cfg := range doc.Destinations {
		destinations[name] = struct {
			Type   string `yaml:"type"`
			Config `yaml:",inline"`
		}{"pagerduty", cfg}
	}
	return yaml.Marshal(map[string]any{"version": doc.Version, "destinations": destinations})
}

type testBody struct {
	io.Reader
	closed bool
}

func (b *testBody) Close() error { b.closed = true; return nil }

type formErrorReader struct{ err error }

func (r formErrorReader) Read([]byte) (int, error) { return 0, r.err }
