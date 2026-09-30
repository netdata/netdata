// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(
		t,
		New(),
		[]byte(
			`{"manifest":"/opt/custom/manifest.yaml","update_every":15,"timeout":3.5,"autodetection_retry":60,"config":{"enabled":false,"count":0,"nested":{"optional":null}}}`,
		),
		[]byte(
			"manifest: /opt/custom/manifest.yaml\nupdate_every: 15\ntimeout: 3.5\nautodetection_retry: 60\nconfig:\n  enabled: false\n  count: 0\n  nested:\n    optional: null\n",
		),
	)
}

func TestCollector_ConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadataWith(
		t,
		"config_schema.json",
		"metadata.dev.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		},
	)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		prepare     func(c *Collector)
		wantErrText string
	}{
		"valid manifest": {
			prepare: func(*Collector) {},
		},
		"relative manifest": {
			prepare:     func(c *Collector) { c.Manifest = "relative.yaml" },
			wantErrText: "absolute",
		},
		"nonpositive timeout": {
			prepare:     func(c *Collector) { c.Timeout = 0 },
			wantErrText: "positive",
		},
		"nonpositive update_every": {
			prepare:     func(c *Collector) { c.UpdateEvery = 0 },
			wantErrText: "positive",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := fixtureCollector(t, "exit 0\n")
			tc.prepare(c)
			err := c.Init(context.Background())
			if tc.wantErrText == "" {
				require.NoError(t, err)
				require.NoError(t, c.Check(context.Background()))
				return
			}
			require.ErrorContains(t, err, tc.wantErrText)
			require.Error(t, c.Check(context.Background()), "a failed Init must not pass Check")
		})
	}
}

func TestCollector_InitUsesProductionPathPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission validation is not implemented on Windows")
	}
	c, _ := fixtureCollector(t, "exit 0\n")
	untrusted := New()
	untrusted.Manifest = c.Manifest
	require.Error(
		t,
		untrusted.Init(context.Background()),
		"production path validation must reject the test-owned executable",
	)
}

// Candidate validation stays local: neither Init nor Check runs the executable.
func TestCollector_InitCheckDoNotRunScript(t *testing.T) {
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			c, dir := fixtureCollector(t, "printf ran > \"$(dirname \"$0\")/executed\"\nexit 99\n")
			appendFile(t, c.Manifest, "mode: "+mode+"\n")
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			requireNoFile(t, filepath.Join(dir, "executed"))
		})
	}
}

// A registered job keeps the startup definition even if package files change.
func TestCollector_InitRegisteredPackage(t *testing.T) {
	registry, dir := configuredFixture(t, "exit 0\n", modeOneshot)
	c := registry["native-fixture"].CreateV2().(*Collector)
	c.ScriptConfig = Settings{
		"text": "synthetic",
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("invalid: manifest"), 0644))
	require.NoError(t, c.Init(context.Background()))
	c.Manifest = "/different/package"
	require.ErrorContains(t, c.Init(context.Background()), "cannot override manifest")
}

func TestCollector_InitRejectsUnregisteredFunctions(t *testing.T) {
	_, dir := functionFixture(t, modeOneshot, true)
	c := newManifestCollector(filepath.Join(dir, "manifest.yaml"))
	require.ErrorContains(t, c.Init(context.Background()), "must be registered")
}

func TestCollector_Configuration(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	c := registry["native-fixture"].CreateV2().(*Collector)
	cfg := c.Configuration().(Config)
	assert.Empty(t, cfg.Manifest, "registered packages do not expose a manifest option")
	assert.Equal(t, float64(17), cfg.ScriptConfig["count"], "registered packages expose effective defaults")
}
