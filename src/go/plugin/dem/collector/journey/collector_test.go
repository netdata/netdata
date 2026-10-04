// SPDX-License-Identifier: GPL-3.0-or-later

package journey

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type prepared struct{}

func (prepared) Check(context.Context, model.Kind) error { return nil }
func (prepared) Execute(context.Context, model.Request, func(string)) model.Execution {
	panic("unexpected execution during config validation")
}
func configured() *Collector {
	c := New(Dependencies{
		Executor: prepared{},
		Hub:      model.NewHub(),
	})
	c.Name = "login"
	c.Script = "test('login', async () => {});"
	return c
}

func TestConfiguration(t *testing.T) {
	cfgJSON := []byte(
		`{"name":"login","update_every":900,"timeout":120,"script":"test('login', async () => {});","secrets":[{"name":"DEM_SECRET_PASSWORD","value":"${env:TEST_PASSWORD}"}],"screenshot_on_failure":false}`,
	)
	cfgYAML := []byte(
		"name: login\nupdate_every: 900\ntimeout: 120\nscript: test('login', async () => {});\nsecrets:\n  - name: DEM_SECRET_PASSWORD\n    value: ${env:TEST_PASSWORD}\nscreenshot_on_failure: false\n",
	)
	collecttest.TestConfigurationSerialize(t, New(Dependencies{}), cfgJSON, cfgYAML)
	for format, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
		t.Run(format, func(t *testing.T) {
			c := New(Dependencies{})
			input := []byte(`{"name":"login","script":"test('login', async () => {});","screenshot_on_failure":false}`)
			require.NoError(t, decode(input, c))
			want := Config{
				Name:        "login",
				UpdateEvery: 900,
				Timeout:     120000000000,
				Script:      "test('login', async () => {});",
			}
			assert.Equal(t, want, c.Configuration())
			raw, err := json.Marshal(c.Configuration())
			require.NoError(t, err)
			copy := New(Dependencies{})
			require.NoError(t, json.Unmarshal(raw, copy))
			assert.Equal(t, c.Configuration(), copy.Configuration())
		})
	}
	creator := Creator(Dependencies{})
	assert.Equal(t, 900, creator.Defaults.UpdateEvery)
	assert.True(t, creator.StoreFirst)
	assert.Equal(t, New(Dependencies{}).Configuration(), creator.CreateV2().Configuration())
}

func TestInitValidation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "login.spec.ts")
	require.NoError(t, os.WriteFile(file, []byte("export {};"), 0600))
	for name, tc := range map[string]struct {
		change func(*Collector)
		valid  bool
	}{
		"inline":                 {func(*Collector) {}, true},
		"file":                   {func(c *Collector) { c.Script = ""; c.ScriptPath = file }, true},
		"missing source":         {func(c *Collector) { c.Script = "" }, false},
		"whitespace source":      {func(c *Collector) { c.Script = "  \n" }, false},
		"blank inline with path": {func(c *Collector) { c.Script = "  "; c.ScriptPath = file }, false},
		"both sources":           {func(c *Collector) { c.ScriptPath = file }, false},
		"relative path":          {func(c *Collector) { c.Script = ""; c.ScriptPath = "login.spec.ts" }, false},
		"missing file":           {func(c *Collector) { c.Script = ""; c.ScriptPath = file + ".missing.ts" }, false},
		"unsupported extension":  {func(c *Collector) { c.Script = ""; c.ScriptPath = file + "x" }, false},
		"directory":              {func(c *Collector) { c.Script = ""; c.ScriptPath = t.TempDir() }, false},
		"missing name":           {func(c *Collector) { c.Name = " " }, false},
		"zero interval":          {func(c *Collector) { c.UpdateEvery = 0 }, false},
		"submillisecond timeout": {func(c *Collector) { c.Timeout = 999999 }, false},
		"zero timeout":           {func(c *Collector) { c.Timeout = 0 }, false},
		"negative timeout":       {func(c *Collector) { c.Timeout = -1 }, false},
		"secret":                 {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN", Value: "test"}} }, true},
		"empty secret value":     {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN"}} }, true},
		"unscoped secret":        {func(c *Collector) { c.Secrets = []Secret{{Name: "PATH"}} }, false},
		"empty secret suffix":    {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_"}} }, false},
		"secret punctuation":     {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_A-B"}} }, false},
		"duplicate secret":       {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN"}, {Name: "DEM_SECRET_TOKEN"}} }, false},
		"NUL secret":             {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN", Value: "a\x00b"}} }, false},
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
	c.Name = "login"
	c.Script = "export {};"
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
	assert.Equal(t, 2*time.Minute, time.Duration(c.Timeout))
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
	health, err := os.ReadFile("../../../../../health/health.d/dem.conf")
	require.NoError(t, err)
	collecttest.AssertHealthAlertsTargetChartTemplateWith(
		t,
		health,
		charts,
		collecttest.HealthAlertsCheck{
			ContextPrefix: "dem_synthetic.",
		},
	)
	collecttest.AssertMetadataAlertsMatchHealthConfigWith(
		t,
		metadata,
		health,
		collecttest.MetadataAlertsCheck{
			SharedHealthConfig: true,
		},
	)
}
