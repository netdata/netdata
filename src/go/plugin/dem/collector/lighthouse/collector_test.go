// SPDX-License-Identifier: GPL-3.0-or-later

package lighthouse

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type prepared struct{}

func (prepared) Check(context.Context, model.Kind) error { return nil }
func (prepared) Execute(context.Context, model.Request, func(string)) model.Execution {
	panic("unexpected audit during validation")
}
func configured() *Collector {
	c := New(Dependencies{
		Executor: prepared{},
		Hub:      model.NewHub(),
	})
	c.Name = "home"
	c.URL = "https://example.org/"
	return c
}

func TestConfiguration(t *testing.T) {
	cfgJSON := []byte(
		`{"name":"home","update_every":1800,"timeout":180,"url":"https://example.org/","save_report":false}`,
	)
	cfgYAML := []byte("name: home\nupdate_every: 1800\ntimeout: 180\nurl: https://example.org/\nsave_report: false\n")
	collecttest.TestConfigurationSerialize(t, New(Dependencies{}), cfgJSON, cfgYAML)
	for format, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
		t.Run(format, func(t *testing.T) {
			c := New(Dependencies{})
			require.NoError(t, decode([]byte(`{"name":"home","url":"https://example.org/","save_report":false}`), c))
			assert.Equal(
				t,
				Config{
					Name:        "home",
					UpdateEvery: 1800,
					Timeout:     180000000000,
					URL:         "https://example.org/",
				},
				c.Configuration(),
			)
			raw, err := json.Marshal(c.Configuration())
			require.NoError(t, err)
			copy := New(Dependencies{})
			require.NoError(t, json.Unmarshal(raw, copy))
			assert.Equal(t, c.Configuration(), copy.Configuration())
		})
	}
	creator := Creator(Dependencies{})
	assert.Equal(t, 1800, creator.Defaults.UpdateEvery)
	assert.True(t, creator.StoreFirst)
	assert.Equal(t, New(Dependencies{}).Configuration(), creator.CreateV2().Configuration())
}

func TestInitValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(*Collector)
		valid  bool
	}{
		"https":                  {func(*Collector) {}, true},
		"http":                   {func(c *Collector) { c.URL = "http://127.0.0.1:8000/" }, true},
		"empty":                  {func(c *Collector) { c.URL = "" }, false},
		"relative":               {func(c *Collector) { c.URL = "/home" }, false},
		"protocol relative":      {func(c *Collector) { c.URL = "//example.org" }, false},
		"credentials":            {func(c *Collector) { c.URL = "https://user:pass@example.org" }, false},
		"username only":          {func(c *Collector) { c.URL = "https://user@example.org" }, false},
		"file scheme":            {func(c *Collector) { c.URL = "file:///tmp/index.html" }, false},
		"empty host":             {func(c *Collector) { c.URL = "http://:80" }, false},
		"invalid":                {func(c *Collector) { c.URL = "https://bad host/" }, false},
		"missing name":           {func(c *Collector) { c.Name = "" }, false},
		"zero interval":          {func(c *Collector) { c.UpdateEvery = 0 }, false},
		"submillisecond timeout": {func(c *Collector) { c.Timeout = 999999 }, false},
		"zero timeout":           {func(c *Collector) { c.Timeout = 0 }, false},
	} {
		t.Run(name, func(t *testing.T) {
			c := configured()
			tc.change(c)
			err := c.Init(context.Background())
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	c := New(Dependencies{})
	c.Name = "home"
	c.URL = "https://example.org/"
	require.Error(t, c.Init(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, configured().Init(ctx), context.Canceled)
}

func TestPreparedCheckAndArtifacts(t *testing.T) {
	c := configured()
	require.Error(t, c.Check(context.Background()))
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	collecttest.AssertChartTemplateSchema(t, charts)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(
		t,
		metadata,
		charts,
		map[string][]string{
			"execution_state": {"unknown", "success", "failed", "timeout", "inconclusive", "error", "cancelled"},
		},
	)
	collecttest.AssertConfigSchemaMatchesMetadataWith(
		t,
		"config_schema.json",
		"metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		},
	)
}
