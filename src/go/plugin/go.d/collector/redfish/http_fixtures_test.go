// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func testConfig(rawURL, auth string) Config {
	cfg := Config{
		URL:        rawURL,
		AuthMethod: auth,
		Username:   "user",
		Password:   "test-password",
	}
	if auth == "none" {
		cfg.Username = ""
		cfg.Password = ""
	}
	cfg.applyDefaults()
	return cfg
}

func newTestCollector(t *testing.T, cfg Config) *Collector {
	t.Helper()
	c := New()
	c.Config = cfg
	if c.Name == "" {
		c.Name = "test-job"
	}
	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c
}
