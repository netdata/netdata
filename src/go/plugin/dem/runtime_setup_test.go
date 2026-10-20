// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trustedFixture() runtime.Manifest {
	return runtime.Manifest{Format: 1, Version: "test", Platform: "linux/" + goruntime.GOARCH,
		Archive:  runtime.Archive{SHA256: strings.Repeat("a", 64), Bytes: 100, TarBytes: 10240, ExpandedBytes: 200, Entries: 2},
		Layout:   runtime.Layout{Node: "node", Dependencies: "dependencies", Browser: "browser", Loader: "loader", Fonts: "fonts"},
		Bindings: []runtime.Binding{{Path: "node", Expected: []byte("loader\x00")}, {Path: "browser", Expected: []byte("loader\x00")}}}
}
func TestInstalledManifestTrust(t *testing.T) {
	manifest := trustedFixture()
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "runtime.json")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	loaded, err := LoadRuntimeManifest(path)
	require.NoError(t, err)
	assert.Equal(t, manifest, *loaded)
	for name, body := range map[string][]byte{
		"trailing JSON":  append(append([]byte{}, raw...), []byte(" {}")...),
		"unknown field":  []byte(strings.Replace(string(raw), `"format":1`, `"format":1,"download_override":"https://untrusted.invalid"`, 1)),
		"digest missing": []byte(strings.Replace(string(raw), strings.Repeat("a", 64), "", 1)),
		"oversized":      []byte(strings.Repeat(" ", 1<<20+1)),
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, body, 0600))
			_, err := LoadRuntimeManifest(path)
			require.Error(t, err)
		})
	}
	_, err = LoadRuntimeManifest("relative.json")
	require.Error(t, err)
	_, err = LoadRuntimeManifest(t.TempDir())
	require.Error(t, err)
}
func TestDefaultInstalledManifestAndUnavailableRUMSetup(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	assets := filepath.Join(t.TempDir(), "dem")
	assert.Equal(t, filepath.Join(assets, "runtime-"+goruntime.GOARCH+".json"), RuntimeManifestPath(cfg, assets))
	cache := filepath.Join(t.TempDir(), "cache")
	cfg.CachePath = cache
	handle := OpenRuntime(cfg, assets, "")
	require.Error(t, handle.SetupError)
	require.NotNil(t, handle.Source)
	require.NotNil(t, handle.Host)
	assert.Equal(t, "unavailable", handle.Host.Status().State)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	_, err = os.Stat(cache)
	require.True(t, os.IsNotExist(err), "missing installed trust must not initialize the cache")
	require.NoError(t, handle.Close())
	require.NoError(t, handle.Close())
}
func TestPreloadedRuntimeRequiresInstalledTrust(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.Mode = "preloaded"
	cfg.PreloadPath = filepath.Join(t.TempDir(), "not-created")
	cfg.ManifestPath = "/not-installed/manifest.json"
	handle := OpenRuntime(cfg, "/installed/dem", "")
	require.Error(t, handle.SetupError)
	require.Nil(t, handle.Host)
	require.Nil(t, handle.Store)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
	_, err = os.Stat(cfg.PreloadPath)
	require.True(t, os.IsNotExist(err))
}
