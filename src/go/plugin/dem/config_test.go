// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want RetentionConfig
		fail bool
	}{
		"partial overrides": {"enabled: yes\nhistory:\n  days: 7\n", RetentionConfig{
			Days:     7,
			MaxBytes: 1 << 30,
		}, false},
		"zero days":                {"history: {days: 0}", RetentionConfig{}, true},
		"too many days":            {"history: {days: 366}", RetentionConfig{}, true},
		"zero byte budget":         {"history: {max_bytes: 0}", RetentionConfig{}, true},
		"below allocation quantum": {"history: {max_bytes: 8388607}", RetentionConfig{}, true},
		"allocation quantum": {"history: {max_bytes: 8388608}", RetentionConfig{
			Days:     30,
			MaxBytes: 8388608,
		}, false},
		"malformed": {"history: [", RetentionConfig{}, true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadConfigText(t, tc.body)
			if tc.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.History)
		})
	}
}

func TestLoadConfigUsesHighestPriorityFile(t *testing.T) {
	user, stock := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(user, "dem.conf"), []byte("history: {days: 2}"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(stock, "dem.conf"), []byte("history: {days: 80}"), 0600))
	cfg, err := LoadConfig(multipath.New(user, stock))
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.History.Days)
}

func TestBrowserConcurrencyConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want int
		fail bool
	}{
		"default":          {want: 1},
		"explicit budget":  {body: "browser: {concurrency: 4}", want: 4},
		"zero":             {body: "browser: {concurrency: 0}", fail: true},
		"negative":         {body: "browser: {concurrency: -1}", fail: true},
		"fractional":       {body: "browser: {concurrency: 1.5}", fail: true},
		"text":             {body: "browser: {concurrency: four}", fail: true},
		"null concurrency": {body: "browser: {concurrency: null}", fail: true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadConfigText(t, tc.body)
			if tc.fail {
				require.ErrorContains(t, err, "browser.concurrency")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.Browser.Concurrency)
		})
	}
}

// A section that is present but empty, including one whose settings are all
// commented out, loads that section's defaults.
func TestEmptySectionsLoadDefaults(t *testing.T) {
	forms := map[string]string{
		"commented settings": "%s:\n  # setting: value\n",
		"null":               "%s: null\n",
		"tilde":              "%s: ~\n",
		"null alias":         "empty: &empty null\n%s: *empty\n",
		"empty mapping":      "%s: {}\n",
	}
	for _, section := range []string{"history", "artifacts", "runtime", "browser"} {
		for form, body := range forms {
			t.Run(section+"/"+form, func(t *testing.T) {
				cfg, err := loadConfigText(t, fmt.Sprintf(body, section))
				require.NoError(t, err)
				assert.Equal(t, DefaultConfig(), cfg)
			})
		}
	}
}

func TestArtifactDefaultsAndIndependentPolicy(t *testing.T) {
	cfg, err := loadConfigText(
		t,
		"runtime: {mode: preloaded, preload_path: /prepared}\nartifacts: {days: 2, max_bytes: 1024}",
	)
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig().History, cfg.History)
	assert.Equal(t, RetentionConfig{
		Days:     2,
		MaxBytes: 1024,
	}, cfg.Artifacts)
	assert.Equal(t, "/prepared", cfg.Runtime.PreloadPath)
	// Runtime availability is checked by its owner; RUM configuration remains independent.
	_, err = loadConfigText(t, "artifacts: {days: 0}")
	require.ErrorContains(t, err, "artifacts")
}

func TestInvalidRetentionIdentifiesSetting(t *testing.T) {
	for _, field := range []string{"history", "artifacts"} {
		for _, setting := range []string{"days", "max_bytes"} {
			t.Run(field+"."+setting, func(t *testing.T) {
				_, err := loadConfigText(t, field+": {"+setting+": 0}")
				require.ErrorContains(t, err, field+"."+setting)
			})
		}
	}
}

func TestRuntimeValidationLeavesOtherSectionsOpaque(t *testing.T) {
	body := "enabled: yes\njobs: {future_option: [{duplicate: one, duplicate: two}]}\n" +
		"history: {days: 7, unrelated: value}\nartifacts: {days: 2, unrelated: value}\n" +
		"runtime: {max_bytes: 2048}\n"
	cfg, err := loadConfigText(t, body)
	require.NoError(t, err)
	require.Equal(t, 7, cfg.History.Days)
	require.Equal(t, DefaultConfig().History.MaxBytes, cfg.History.MaxBytes)
	require.Equal(t, 2, cfg.Artifacts.Days)
	require.Equal(t, DefaultConfig().Artifacts.MaxBytes, cfg.Artifacts.MaxBytes)
	require.EqualValues(t, 2048, cfg.Runtime.MaxBytes)
}

func TestRuntimeConfigOverrides(t *testing.T) {
	loaded, err := loadConfigText(t, "runtime: {max_bytes: 2147483648, previous_retention_days: 0}")
	require.NoError(t, err)
	assert.Equal(t, runtimehost.ModeManaged, loaded.Runtime.Mode)
	assert.EqualValues(t, 2147483648, loaded.Runtime.MaxBytes)
	assert.Equal(t, 0, loaded.Runtime.PreviousRetentionDays)
}

func TestLoadConfigImageRuntimeDefaults(t *testing.T) {
	const imagePath = "/opt/netdata-dem-runtime"
	mode, path := runtimehost.DefaultMode, runtimehost.DefaultPreloadPath
	t.Cleanup(func() { runtimehost.DefaultMode, runtimehost.DefaultPreloadPath = mode, path })
	runtimehost.DefaultMode, runtimehost.DefaultPreloadPath = string(runtimehost.ModePreloaded), imagePath

	t.Run("no dem.conf", func(t *testing.T) {
		cfg, err := LoadConfig(multipath.New(t.TempDir()))
		require.NoError(t, err)
		assert.Equal(t, runtimehost.ModePreloaded, cfg.Runtime.Mode)
		assert.Equal(t, imagePath, cfg.Runtime.PreloadPath)
	})
	for name, tc := range map[string]struct {
		body string
		mode runtimehost.Mode
		path string
		err  string
	}{
		"omitted runtime": {
			body: "history: {days: 14}",
			mode: runtimehost.ModePreloaded,
			path: imagePath,
		},
		"null runtime": {
			body: "runtime:\n  # mode: managed",
			mode: runtimehost.ModePreloaded,
			path: imagePath,
		},
		"partial runtime": {
			body: "runtime: {max_bytes: 2048}",
			mode: runtimehost.ModePreloaded,
			path: imagePath,
		},
		"explicit preload": {
			body: "runtime: {mode: preloaded}",
			mode: runtimehost.ModePreloaded,
			path: imagePath,
		},
		"explicit managed": {
			body: "runtime: {mode: managed}",
			mode: runtimehost.ModeManaged,
		},
		"explicit native": {
			body: "runtime: {mode: native}",
			mode: runtimehost.ModeNative,
		},
		"custom path": {
			body: "runtime: {preload_path: /custom}",
			mode: runtimehost.ModePreloaded,
			path: "/custom",
		},
		"explicit empty": {
			body: "runtime: {preload_path: ''}",
			mode: runtimehost.ModePreloaded,
		},
		"explicit null": {
			body: "runtime: {preload_path: null}",
			mode: runtimehost.ModePreloaded,
		},
		"aliased path": {
			body: "path: &path /custom\nruntime: {<<: {preload_path: *path}}",
			mode: runtimehost.ModePreloaded,
			path: "/custom",
		},
		"conflicting path": {
			body: "runtime: {mode: managed, preload_path: " + imagePath + "}",
			err:  "runtime.preload_path requires mode: preloaded",
		},
		"conflicting cache": {
			body: "runtime: {cache_path: /custom}",
			err:  "runtime.cache_path is only valid in managed mode",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadConfigText(t, tc.body)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.mode, cfg.Runtime.Mode)
			assert.Equal(t, tc.path, cfg.Runtime.PreloadPath)
		})
	}
}

func TestRuntimeConfigRejectsAmbiguousAndInvalidPolicy(t *testing.T) {
	for _, body := range []string{"mode: auto", "max_bytes: 0", "previous_retention_days: -1", "previous_retention_days: 36501", "release_url: http://example.invalid/latest", "release_url: https://user:pass@example.invalid/latest", "release_url: https://example.invalid/latest#fragment", "offline_keyring: relative.gpg"} {
		t.Run(body, func(t *testing.T) {
			_, err := loadConfigText(t, "runtime: {"+body+"}")
			require.ErrorContains(t, err, "runtime.")
		})
	}
}

func TestRuntimeConfigRejectsUnknownAndConflictingSettings(t *testing.T) {
	for _, body := range []string{"allow_downlaod: false", "mode: managed, preload_path: /bundle", "mode: preloaded, preload_path: /bundle, cache_path: /cache"} {
		t.Run(body, func(t *testing.T) {
			_, err := loadConfigText(t, "enabled: true\nruntime: {"+body+"}\n")
			require.ErrorContains(t, err, "runtime")
		})
	}
}

func TestRuntimeConfigErrorsReferToOriginalInput(t *testing.T) {
	for name, tc := range map[string]struct {
		body, line, detail string
	}{
		"unknown":   {"allow_downlaod: false", "line 4:", "field allow_downlaod not found"},
		"duplicate": {"max_bytes: 4096\n  max_bytes: 2048", "line 5:", "field max_bytes already set"},
		"type":      {"max_bytes: [2048]", "line 4:", "cannot unmarshal !!seq into int64"},
	} {
		t.Run(name, func(t *testing.T) {
			body := "enabled: true\n# original configuration\nruntime:\n  " + tc.body + "\n"
			_, err := loadConfigText(t, body)
			require.ErrorContains(t, err, tc.line)
			require.ErrorContains(t, err, tc.detail)
			require.NotContains(t, err.Error(), "dem.plain")
		})
	}
}

func TestRuntimeConfigMergeValidation(t *testing.T) {
	for name, tc := range map[string]struct{ body, detail string }{
		"merged unknown":   {"runtime:\n  <<: {node_path: /tmp/node}\n", "field node_path not found"},
		"merged duplicate": {"runtime:\n  <<: {max_bytes: 2048, max_bytes: 4096}\n", "field max_bytes already set"},
		"alias unknown":    {"base: &base {allow_downlaod: false}\nruntime: {<<: *base}\n", "field allow_downlaod not found"},
		"alias duplicate":  {"base: &base {mode: managed, mode: preloaded}\nruntime: {<<: *base}\n", "field mode already set"},
		"top duplicate":    {"enabled: true\nenabled: false\nruntime: {max_bytes: 2048}\n", `key "enabled" already set`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadConfigText(t, tc.body)
			require.ErrorContains(t, err, tc.detail)
		})
	}
}

func TestRuntimeConfigLegitimateMergesPreserveDefaults(t *testing.T) {
	for name, body := range map[string]string{
		"override":  "runtime: {<<: {max_bytes: 4096}, max_bytes: 2048}\n",
		"sequence":  "runtime: {<<: [{max_bytes: 2048}, {max_bytes: 4096}]}\n",
		"alias":     "base: &base {max_bytes: 2048}\nruntime: {<<: *base}\n",
		"top merge": "base: &base {runtime: {max_bytes: 2048}}\n<<: *base\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadConfigText(t, body)
			require.NoError(t, err)
			expected := runtimehost.DefaultConfig()
			expected.MaxBytes = 2048
			require.Equal(t, expected, cfg.Runtime)
		})
	}
}

func TestRuntimeConfigMergeErrorsUseOriginalSourceLines(t *testing.T) {
	for name, tc := range map[string]struct{ source, line, detail string }{
		"unknown":   {"defaults: &defaults\n  node_path: /tmp/node\n\nruntime: {<<: *defaults}\n", "line 2:", "node_path"},
		"duplicate": {"defaults: &defaults\n  max_bytes: 4096\n  max_bytes: 2048\n\nruntime: {<<: *defaults}\n", "line 3:", "max_bytes"},
		"type":      {"defaults: &defaults\n  max_bytes: [2048]\n\nruntime: {<<: *defaults}\n", "line 2:", "int64"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadConfigText(t, tc.source)
			require.ErrorContains(t, err, tc.line)
			require.ErrorContains(t, err, tc.detail)
			require.NotContains(t, err.Error(), "dem.plain")
		})
	}
}

// loadConfigText loads body as the only dem.conf.
func loadConfigText(t *testing.T, body string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(body), 0600))
	return LoadConfig(multipath.New(dir))
}
