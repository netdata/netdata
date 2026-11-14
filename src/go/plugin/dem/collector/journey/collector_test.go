// SPDX-License-Identifier: GPL-3.0-or-later

package journey

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func configured() *Collector {
	c := New(Dependencies{
		Executor: syntheticjobtest.Executor{
			ExecuteFunc: func(context.Context, model.Request, func(model.JobState)) model.Execution {
				panic("unexpected execution during config validation")
			},
		},
		Registry: syntheticregistry.New(),
	})
	c.Name = "login"
	c.Script = "test('login', async () => {});"
	return c
}

// request returns the request Init stores for the configuration of c, as the executor receives it.
func request(t *testing.T, c *Collector) model.Request {
	t.Helper()
	var sent model.Request
	probe := New(Dependencies{
		Executor: syntheticjobtest.Executor{
			RehearseFunc: func(_ context.Context, r model.Request) error {
				sent = r
				return nil
			},
		},
		Registry: syntheticregistry.New(),
	})
	probe.Config = c.Config
	require.NoError(t, probe.Init(t.Context()))
	require.NoError(t, probe.Test(t.Context()))
	return sent
}

var _ dyncfg.Testable = (*Collector)(nil)

func TestConfiguration(t *testing.T) {
	cfgJSON := []byte(
		`{"vnode":"storefront","name":"login","update_every":10,"check_every":900,"timeout":120,"script":"test('login', async () => {});","secrets":[{"name":"DEM_SECRET_PASSWORD","value":"${env:TEST_PASSWORD}"}],"screenshot_on_failure":true,"trace_on_failure":true,"browser_error_summary":true}`,
	)
	cfgYAML := []byte(
		"vnode: storefront\nname: login\nupdate_every: 10\ncheck_every: 900\ntimeout: 120\nscript: test('login', async () => {});\nsecrets:\n  - name: DEM_SECRET_PASSWORD\n    value: ${env:TEST_PASSWORD}\nscreenshot_on_failure: true\ntrace_on_failure: true\nbrowser_error_summary: true\n",
	)
	collecttest.TestConfigurationSerialize(t, New(Dependencies{}), cfgJSON, cfgYAML)
	for format, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
		t.Run(format, func(t *testing.T) {
			c := New(Dependencies{})
			input := []byte(
				`{"name":"login","script":"test('login', async () => {});","screenshot_on_failure":false,"trace_on_failure":false}`,
			)
			require.NoError(t, decode(input, c))
			want := Config{
				Name:        "login",
				UpdateEvery: 10,
				CheckEvery:  900,
				Timeout:     120000000000,
				Script:      "test('login', async () => {});",
			}
			assert.Equal(t, want, c.Configuration())
			raw, err := json.Marshal(c.Configuration())
			require.NoError(t, err)
			assert.Contains(t, string(raw), `"trace_on_failure":false`)
			copy := New(Dependencies{})
			require.NoError(t, json.Unmarshal(raw, copy))
			assert.Equal(t, c.Configuration(), copy.Configuration())
		})
	}
	creator := Creator(Dependencies{})
	assert.Equal(t, 10, creator.Defaults.UpdateEvery)
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
		"trace needs screenshot":  {func(c *Collector) { c.TraceOnFailure = true }, false},
		"trace enabled":           {func(c *Collector) { c.TraceOnFailure = true; c.ScreenshotOnFailure = true }, true},
		"inline":                  {func(*Collector) {}, true},
		"file":                    {func(c *Collector) { c.Script = ""; c.ScriptPath = file }, true},
		"missing source":          {func(c *Collector) { c.Script = "" }, false},
		"whitespace source":       {func(c *Collector) { c.Script = "  \n" }, false},
		"blank inline with path":  {func(c *Collector) { c.Script = "  "; c.ScriptPath = file }, false},
		"both sources":            {func(c *Collector) { c.ScriptPath = file }, false},
		"relative path":           {func(c *Collector) { c.Script = ""; c.ScriptPath = "login.spec.ts" }, false},
		"missing file":            {func(c *Collector) { c.Script = ""; c.ScriptPath = file + ".missing.ts" }, false},
		"unsupported extension":   {func(c *Collector) { c.Script = ""; c.ScriptPath = file + "x" }, false},
		"directory":               {func(c *Collector) { c.Script = ""; c.ScriptPath = t.TempDir() }, false},
		"missing name":            {func(c *Collector) { c.Name = " " }, false},
		"zero check interval":     {func(c *Collector) { c.CheckEvery = 0 }, false},
		"negative check interval": {func(c *Collector) { c.CheckEvery = -1 }, false},
		"duration capacity":       {func(c *Collector) { c.CheckEvery = int(^uint(0) >> 1) }, time.Duration(int(^uint(0)>>1)) <= time.Duration(1<<63-1)/time.Second},
		"zero interval":           {func(c *Collector) { c.UpdateEvery = 0 }, false},
		"submillisecond timeout":  {func(c *Collector) { c.Timeout = 999999 }, false},
		"zero timeout":            {func(c *Collector) { c.Timeout = 0 }, false},
		"negative timeout":        {func(c *Collector) { c.Timeout = -1 }, false},
		"secret":                  {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN", Value: "test"}} }, true},
		"empty secret value":      {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN"}} }, true},
		"unscoped secret":         {func(c *Collector) { c.Secrets = []Secret{{Name: "PATH"}} }, false},
		"empty secret suffix":     {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_"}} }, false},
		"secret punctuation":      {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_A-B"}} }, false},
		"duplicate secret":        {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN"}, {Name: "DEM_SECRET_TOKEN"}} }, false},
		"NUL secret":              {func(c *Collector) { c.Secrets = []Secret{{Name: "DEM_SECRET_TOKEN", Value: "a\x00b"}} }, false},
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
			"execution_state": {
				"unknown",
				"success",
				"failed",
				"timeout",
				"inconclusive",
				"error",
				"cancelled",
				"stale",
			},
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
}

func TestYAMLNullVnodeRemainsUnassigned(t *testing.T) {
	c := configured()
	require.NoError(t, yaml.Unmarshal([]byte("vnode: null\n"), c))
	require.NoError(t, c.Init(context.Background()))
	assert.Empty(t, c.Vnode)
}

func TestBrowserErrorSummaryOption(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			for format, codec := range map[string]struct {
				encode func(any) ([]byte, error)
				decode func([]byte, any) error
			}{
				"json": {json.Marshal, json.Unmarshal},
				"yaml": {yaml.Marshal, yaml.Unmarshal},
			} {
				t.Run(format, func(t *testing.T) {
					c := configured()
					assert.False(t, c.BrowserErrorSummary)
					raw, err := codec.encode(map[string]any{"browser_error_summary": enabled})
					require.NoError(t, err)
					require.NoError(t, codec.decode(raw, c))
					require.NoError(t, c.Init(context.Background()))
					assert.Equal(t, enabled, request(t, c).BrowserErrorSummary)
					raw, err = codec.encode(c.Configuration())
					require.NoError(t, err)
					copy := configured()
					require.NoError(t, codec.decode(raw, copy))
					require.NoError(t, copy.Init(context.Background()))
					assert.Equal(t, enabled, request(t, copy).BrowserErrorSummary)
				})
			}
		})
	}
	c := configured()
	require.NoError(t, c.Init(context.Background()))
	assert.False(t, request(t, c).BrowserErrorSummary)
}

func TestTraceRequestAndDependency(t *testing.T) {
	c := configured()
	assert.False(t, c.TraceOnFailure)
	c.TraceOnFailure = true
	require.EqualError(t, c.Init(t.Context()), "trace_on_failure requires screenshot_on_failure")
	c.ScreenshotOnFailure = true
	require.NoError(t, c.Init(t.Context()))
	assert.True(t, request(t, c).Trace)
	assert.True(t, request(t, c).Capture)
}
