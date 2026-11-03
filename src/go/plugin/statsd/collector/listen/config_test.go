// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"context"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/statsd/collector/listen/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
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
	both := func(addr string) ListenerConfig {
		return ListenerConfig{
			Protocol: protocolBoth,
			Address:  addr,
		}
	}
	pair := []ListenerConfig{udp(":18125"), tcp(":18125")}
	listeners := func(ls ...ListenerConfig) func(*Config) { return func(c *Config) { c.Listeners = ls } }
	for name, tc := range map[string]struct {
		change  func(*Config)
		wantErr bool
	}{
		"udp and tcp listeners on a port": {change: listeners(pair...)},
		"both transports in one listener": {change: listeners(both(":18125"))},
		"both beside udp on another port": {change: listeners(udp(":18125"), both(":18126"))},
		"both overlaps udp":               {change: listeners(udp(":18125"), both(":18125")), wantErr: true},
		"both overlaps tcp":               {change: listeners(both(":18125"), tcp(":18125")), wantErr: true},
		"duplicate both":                  {change: listeners(both(":18125"), both(":18125")), wantErr: true},
		"empty protocol means both":       {change: listeners(ListenerConfig{Address: ":18125"})},
		"empty protocol overlaps udp": {
			change:  listeners(udp(":18125"), ListenerConfig{Address: ":18125"}),
			wantErr: true,
		},
		"idle expiry disabled": {change: func(c *Config) { c.Listeners, c.MetricIdleTimeout = pair, 0 }},
		"zero series cap":      {change: func(c *Config) { c.Listeners, c.MaxSeries = pair, 0 }, wantErr: true},
		"negative series cap":  {change: func(c *Config) { c.Listeners, c.MaxSeries = pair, -1 }, wantErr: true},
		"negative idle timeout": {
			change:  func(c *Config) { c.Listeners, c.MetricIdleTimeout = pair, -1 },
			wantErr: true,
		},
		"zero tcp cap": {
			change:  func(c *Config) { c.Listeners, c.MaxTCPConnections = pair, 0 },
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
		"duplicate":      {change: listeners(pair[0], pair[0]), wantErr: true},
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

// TestEmptyProtocolMeansBoth covers a DynCfg or file job whose listener leaves
// the protocol blank or omits it. Jobs reach the collector the way the job
// manager applies them: the config map encoded to YAML and decoded into the
// collector.
func TestEmptyProtocolMeansBoth(t *testing.T) {
	tests := map[string]struct {
		decode func([]byte, any) error
		input  string
	}{
		"dyncfg json": {
			decode: json.Unmarshal,
			input:  `{"listeners":[{"address":"127.0.0.1:18125"},{"protocol":"","address":"127.0.0.1:18126"}]}`,
		},
		"file yaml": {
			decode: yaml.Unmarshal,
			input:  "listeners:\n  - address: 127.0.0.1:18125\n  - protocol: ''\n    address: 127.0.0.1:18126\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var job map[string]any
			require.NoError(t, tc.decode([]byte(tc.input), &job))
			payload, err := yaml.Marshal(job)
			require.NoError(t, err)
			c := New()
			require.NoError(t, yaml.Unmarshal(payload, c))
			require.NoError(t, c.Init(context.Background()))

			bs, err := json.Marshal(c.Configuration())
			require.NoError(t, err)
			var got struct {
				Listeners []map[string]string `json:"listeners"`
			}
			require.NoError(t, json.Unmarshal(bs, &got))
			assert.Equal(t, []map[string]string{
				{"protocol": "both", "address": "127.0.0.1:18125"},
				{"protocol": "both", "address": "127.0.0.1:18126"},
			}, got.Listeners)
			assert.Equal(t, []server.Endpoint{
				{Network: "udp", Address: "127.0.0.1:18125", Name: "listeners[0]"},
				{Network: "tcp", Address: "127.0.0.1:18125", Name: "listeners[0]"},
				{Network: "udp", Address: "127.0.0.1:18126", Name: "listeners[1]"},
				{Network: "tcp", Address: "127.0.0.1:18126", Name: "listeners[1]"},
			}, c.serverConfig().Endpoints)
		})
	}
}
