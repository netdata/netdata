// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func configuredLighthouse() *Lighthouse {
	c := NewLighthouse(Dependencies{
		Executor: syntheticjobtest.Executor{
			ExecuteFunc: func(context.Context, model.Request, func(model.JobState)) model.Execution {
				panic("unexpected execution during config validation")
			},
		},
		Registry: syntheticregistry.New(),
	})
	c.Name = "home"
	c.URL = "https://example.org/"
	return c
}

func TestLighthouseConfiguration(t *testing.T) {
	cfgJSON := []byte(
		`{"vnode":"storefront","name":"home","update_every":10,"check_every":1800,"timeout":180,"url":"https://example.org/","save_report":false}`,
	)
	cfgYAML := []byte(
		"vnode: storefront\nname: home\nupdate_every: 10\ncheck_every: 1800\ntimeout: 180\nurl: https://example.org/\nsave_report: false\n",
	)
	collecttest.TestConfigurationSerialize(t, NewLighthouse(Dependencies{}), cfgJSON, cfgYAML)
	for format, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
		t.Run(format, func(t *testing.T) {
			c := NewLighthouse(Dependencies{})
			require.NoError(t, decode([]byte(`{"name":"home","url":"https://example.org/","save_report":false}`), c))
			assert.Equal(
				t,
				LighthouseConfig{
					Name:        "home",
					UpdateEvery: 10,
					CheckEvery:  1800,
					Timeout:     180000000000,
					URL:         "https://example.org/",
				},
				c.Configuration(),
			)
			raw, err := json.Marshal(c.Configuration())
			require.NoError(t, err)
			copy := NewLighthouse(Dependencies{})
			require.NoError(t, json.Unmarshal(raw, copy))
			assert.Equal(t, c.Configuration(), copy.Configuration())
		})
	}
	creator := LighthouseCreator(Dependencies{})
	assert.Equal(t, 10, creator.Defaults.UpdateEvery)
	assert.True(t, creator.StoreFirst)
	assert.Equal(t, NewLighthouse(Dependencies{}).Configuration(), creator.CreateV2().Configuration())
}

func TestLighthouseInitValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(*Lighthouse)
		valid  bool
	}{
		"https":                   {func(*Lighthouse) {}, true},
		"http":                    {func(c *Lighthouse) { c.URL = "http://127.0.0.1:8000/" }, true},
		"empty":                   {func(c *Lighthouse) { c.URL = "" }, false},
		"relative":                {func(c *Lighthouse) { c.URL = "/home" }, false},
		"protocol relative":       {func(c *Lighthouse) { c.URL = "//example.org" }, false},
		"credentials":             {func(c *Lighthouse) { c.URL = "https://user:pass@example.org" }, false},
		"username only":           {func(c *Lighthouse) { c.URL = "https://user@example.org" }, false},
		"file scheme":             {func(c *Lighthouse) { c.URL = "file:///tmp/index.html" }, false},
		"empty host":              {func(c *Lighthouse) { c.URL = "http://:80" }, false},
		"invalid":                 {func(c *Lighthouse) { c.URL = "https://bad host/" }, false},
		"missing name":            {func(c *Lighthouse) { c.Name = "" }, false},
		"zero check interval":     {func(c *Lighthouse) { c.CheckEvery = 0 }, false},
		"negative check interval": {func(c *Lighthouse) { c.CheckEvery = -1 }, false},
		"duration capacity":       {func(c *Lighthouse) { c.CheckEvery = int(^uint(0) >> 1) }, time.Duration(int(^uint(0)>>1)) <= time.Duration(1<<63-1)/time.Second},
		"zero interval":           {func(c *Lighthouse) { c.UpdateEvery = 0 }, false},
		"submillisecond timeout":  {func(c *Lighthouse) { c.Timeout = 999999 }, false},
		"zero timeout":            {func(c *Lighthouse) { c.Timeout = 0 }, false},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredLighthouse()
			tc.change(c)
			err := c.Init(context.Background())
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	c := NewLighthouse(Dependencies{})
	c.Name = "home"
	c.URL = "https://example.org/"
	require.Error(t, c.Init(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, configuredLighthouse().Init(ctx), context.Canceled)
}

// DynCfg detects Test by method set, so a Test promoted from the shared job would change how drafts are validated.
func TestLighthouseDraftValidationUsesCheck(t *testing.T) {
	_, testable := any(NewLighthouse(Dependencies{})).(dyncfg.Testable)
	assert.False(t, testable)
}

func TestLighthouseYAMLNullVnodeRemainsUnassigned(t *testing.T) {
	c := configuredLighthouse()
	require.NoError(t, yaml.Unmarshal([]byte("vnode: null\n"), c))
	require.NoError(t, c.Init(context.Background()))
	assert.Empty(t, c.Vnode)
}

func TestLighthousePreparedCheck(t *testing.T) {
	c := configuredLighthouse()
	require.Error(t, c.Check(context.Background()))
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	assert.Equal(t, 3*time.Minute, time.Duration(c.Timeout))
}
