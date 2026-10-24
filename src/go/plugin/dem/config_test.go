// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
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
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(tc.body), 0600))
			cfg, err := LoadConfig(multipath.New(dir))
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

func TestArtifactDefaultsAndIndependentPolicy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {mode: preloaded, preload_path: /prepared}\nartifacts: {days: 2, max_bytes: 1024}"), 0600))
	cfg, err := LoadConfig(multipath.New(dir))
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig().History, cfg.History)
	assert.Equal(t, RetentionConfig{
		Days:     2,
		MaxBytes: 1024,
	}, cfg.Artifacts)
	assert.Equal(t, "/prepared", cfg.Runtime.PreloadPath)
	// Runtime availability is checked by its owner; RUM configuration remains independent.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("artifacts: {days: 0}"), 0600))
	_, err = LoadConfig(multipath.New(dir))
	require.ErrorContains(t, err, "artifacts")
}

func TestInvalidRetentionIdentifiesSetting(t *testing.T) {
	for _, field := range []string{"history", "artifacts"} {
		for _, setting := range []string{"days", "max_bytes"} {
			t.Run(field+"."+setting, func(t *testing.T) {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(field+": {"+setting+": 0}"), 0600))
				_, err := LoadConfig(multipath.New(dir))
				require.ErrorContains(t, err, field+"."+setting)
			})
		}
	}
}

func TestRuntimeValidationLeavesOtherSectionsOpaque(t *testing.T) {
	dir := t.TempDir()
	body := "enabled: yes\njobs: {future_option: [{duplicate: one, duplicate: two}]}\n" +
		"history: {days: 7, unrelated: value}\nartifacts: {days: 2, unrelated: value}\n" +
		"runtime: {allow_download: false}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(body), 0600))
	cfg, err := LoadConfig(multipath.New(dir))
	require.NoError(t, err)
	require.Equal(t, 7, cfg.History.Days)
	require.Equal(t, DefaultConfig().History.MaxBytes, cfg.History.MaxBytes)
	require.Equal(t, 2, cfg.Artifacts.Days)
	require.Equal(t, DefaultConfig().Artifacts.MaxBytes, cfg.Artifacts.MaxBytes)
	require.False(t, cfg.Runtime.AllowDownload)
}
