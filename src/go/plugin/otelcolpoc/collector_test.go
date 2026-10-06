// SPDX-License-Identifier: GPL-3.0-or-later
package otelcolpoc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func configured(t *testing.T, kind string) *Collector {
	t.Helper()
	c := New(kind, filepath.Join(t.TempDir(), "otel-worker"), t.TempDir(), "127.0.0.1:4317")
	c.Name = "fixture"
	if kind == "filelogs" {
		c.Include = []string{filepath.Join(t.TempDir(), "app.log")}
	}
	return c
}
func TestConfigurationAndNativePipeline(t *testing.T) {
	for _, kind := range []string{"hostmetrics", "filelogs"} {
		t.Run(kind, func(t *testing.T) {
			c := configured(t, kind)
			c.ServiceName = "literal-${env:PRIVATE}"
			for _, codec := range []struct {
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
			}{{json.Marshal, json.Unmarshal}, {yaml.Marshal, yaml.Unmarshal}} {
				data, err := codec.marshal(c.Configuration())
				require.NoError(t, err)
				var decoded Config
				require.NoError(t, codec.unmarshal(data, &decoded))
				assert.Equal(t, c.Config, decoded)
			}
			require.NoError(t, c.Init(context.Background()))
			var native map[string]any
			require.NoError(t, json.Unmarshal(c.payload, &native))
			assert.Contains(t, string(c.payload), "literal-${env:PRIVATE}", "escaping belongs to the worker provider")
			receiver, signal := "host_metrics", "metrics"
			if kind == "filelogs" {
				receiver, signal = "file_log", "logs"
			}
			assert.Contains(t, native["receivers"], receiver)
			service := native["service"].(map[string]any)
			assert.Contains(t, service["pipelines"], signal)
			assert.Error(t, c.Collect(context.Background()), "no uptime observation before pipeline readiness")
			c.Cleanup(context.Background())
		})
	}
}
func TestConfigurationRejection(t *testing.T) {
	for name, tc := range map[string]struct {
		kind   string
		mutate func(*Collector)
	}{
		"interval":             {"hostmetrics", func(c *Collector) { c.CollectionInterval = "1ms" }},
		"unsupported scraper":  {"hostmetrics", func(c *Collector) { c.Scrapers = []string{"invalid"} }},
		"relative path":        {"filelogs", func(c *Collector) { c.Include = []string{"app.log"} }},
		"position":             {"filelogs", func(c *Collector) { c.StartAt = "invalid" }},
		"cross module options": {"filelogs", func(c *Collector) { c.Scrapers = []string{"cpu"} }},
	} {
		t.Run(name, func(t *testing.T) {
			c := configured(t, tc.kind)
			tc.mutate(c)
			require.Error(t, c.Init(context.Background()))
		})
	}
}
func TestStorageIdentity(t *testing.T) {
	c := configured(t, "filelogs")
	initial := c.storageDir()
	c.Include = []string{"/different/*.log"}
	c.ServiceName = "changed"
	assert.Equal(t, initial, c.storageDir(), "configuration replacement must preserve offsets")
	c.Name = "../other"
	assert.NotEqual(t, initial, c.storageDir(), "different jobs must not share storage")
	relative, err := filepath.Rel(c.state, c.storageDir())
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(relative), relative, "job names cannot traverse state paths")
}
