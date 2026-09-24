// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"context"
	_ "embed"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/config.json
var configJSON []byte

//go:embed testdata/config.yaml
var configYAML []byte

func TestConfigurationSerialize(t *testing.T) {
	c := New()
	collecttest.TestConfigurationSerialize(t, c, configJSON, configYAML)
	c.profileDirs = testProfileDirs
	require.NoError(t, c.Init(context.Background()))
	assert.Zero(t, c.receiver.idle) // Explicit zero survives New, decode, retrieval and Init.
}

func TestConfigValidation(t *testing.T) {
	udp := func(addr string) ListenerConfig {
		return ListenerConfig{
			Protocol: protocolUDP,
			Address:  addr,
		}
	}
	tcp := func(addr string) ListenerConfig {
		return ListenerConfig{
			Protocol: protocolTCP,
			Address:  addr,
		}
	}
	both := []ListenerConfig{udp(":18125"), tcp(":18125")}
	listeners := func(ls ...ListenerConfig) func(*Config) { return func(c *Config) { c.Listeners = ls } }
	for name, tc := range map[string]struct {
		change  func(*Config)
		wantErr bool
	}{
		"same port on both transports": {change: listeners(both...)},
		"idle expiry disabled":         {change: func(c *Config) { c.Listeners, c.MetricIdleTimeout = both, 0 }},
		"zero series cap":              {change: func(c *Config) { c.Listeners, c.MaxSeries = both, 0 }, wantErr: true},
		"negative series cap":          {change: func(c *Config) { c.Listeners, c.MaxSeries = both, -1 }, wantErr: true},
		"negative idle timeout": {
			change:  func(c *Config) { c.Listeners, c.MetricIdleTimeout = both, -1 },
			wantErr: true,
		},
		"zero tcp cap": {
			change:  func(c *Config) { c.Listeners, c.MaxTCPConnections = both, 0 },
			wantErr: true,
		},
		"no listener": {change: listeners(), wantErr: true},
		"unknown protocol": {change: listeners(ListenerConfig{
			Protocol: "unix",
			Address:  "127.0.0.1:1",
		}), wantErr: true},
		"uppercase protocol": {change: listeners(ListenerConfig{
			Protocol: "UDP",
			Address:  "127.0.0.1:1",
		}), wantErr: true},
		"missing port":   {change: listeners(udp("127.0.0.1")), wantErr: true},
		"port zero":      {change: listeners(udp("127.0.0.1:0")), wantErr: true},
		"port too large": {change: listeners(tcp("127.0.0.1:65536")), wantErr: true},
		"named port":     {change: listeners(tcp("127.0.0.1:statsd")), wantErr: true},
		"duplicate":      {change: listeners(both[0], both[0]), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			tc.change(&c.Config)
			if err := c.Init(context.Background()); tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
