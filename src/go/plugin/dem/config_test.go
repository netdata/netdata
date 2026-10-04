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
		want HistoryConfig
		fail bool
	}{
		"partial overrides": {"enabled: yes\nhistory:\n  days: 7\n", HistoryConfig{
			Days:     7,
			MaxBytes: 1 << 30,
		}, false},
		"zero days":        {"history: {days: 0}", HistoryConfig{}, true},
		"too many days":    {"history: {days: 366}", HistoryConfig{}, true},
		"zero byte budget": {"history: {max_bytes: 0}", HistoryConfig{}, true},
		"malformed":        {"history: [", HistoryConfig{}, true},
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {node_path: /prepared/node, dependencies_path: /prepared}\nartifacts: {days: 2, max_bytes: 1024}"), 0600))
	cfg, err := LoadConfig(multipath.New(dir))
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig().History, cfg.History)
	assert.Equal(t, HistoryConfig{Days: 2, MaxBytes: 1024}, cfg.Artifacts)
	assert.Equal(t, "/prepared/node", cfg.Runtime.NodePath)
	// Preparation belongs to browser Check; RUM-only configuration needs no runtime.
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
