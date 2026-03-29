// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigCustomFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "topology-ip-intel.yaml")
	err := os.WriteFile(cfgPath, []byte(`
sources:
  - name: primary-asn
    family: asn
    provider: dbip
    artifact: asn-lite
    path: /tmp/asn.mmdb
  - name: fallback-geo
    family: geo
    provider: dbip
    artifact: city-lite
    url: https://example.test/city.mmdb.gz
    format: mmdb
output:
  directory: /tmp/netdata-ip-intel
  asn_file: asn.mmdb
  geo_file: geo.mmdb
  metadata_file: meta.json
policy:
  localhost_cidrs: ["127.0.0.0/8"]
  private_cidrs: ["10.0.0.0/8"]
  interesting_cidrs: ["203.0.113.0/24"]
http:
  timeout: 30s
  user_agent: test-agent
`), 0o644)
	require.NoError(t, err)

	cfg, loadedPath, err := loadConfig(cfgPath)
	require.NoError(t, err)
	require.Equal(t, cfgPath, loadedPath)
	require.Len(t, cfg.sources, 2)

	require.Equal(t, "primary-asn", cfg.sources[0].name)
	require.Equal(t, sourceFamilyASN, cfg.sources[0].family)
	require.Equal(t, providerDBIP, cfg.sources[0].provider)
	require.Equal(t, artifactDBIPASNLite, cfg.sources[0].artifact)
	require.Equal(t, formatMMDB, cfg.sources[0].format)
	require.Equal(t, "/tmp/asn.mmdb", cfg.sources[0].path)

	require.Equal(t, "fallback-geo", cfg.sources[1].name)
	require.Equal(t, sourceFamilyGeo, cfg.sources[1].family)
	require.Equal(t, providerDBIP, cfg.sources[1].provider)
	require.Equal(t, artifactDBIPCityLite, cfg.sources[1].artifact)
	require.Equal(t, formatMMDB, cfg.sources[1].format)
	require.Equal(t, "https://example.test/city.mmdb.gz", cfg.sources[1].url)

	require.Equal(t, "/tmp/netdata-ip-intel", cfg.output.directory)
	require.Equal(t, "asn.mmdb", cfg.output.asnFile)
	require.Equal(t, "geo.mmdb", cfg.output.geoFile)
	require.Equal(t, "meta.json", cfg.output.metadataFile)
	require.Equal(t, []string{"127.0.0.0/8"}, cfg.policy.localhostCIDRs)
	require.Equal(t, []string{"10.0.0.0/8"}, cfg.policy.privateCIDRs)
	require.Equal(t, []string{"203.0.113.0/24"}, cfg.policy.interestingCIDRs)
	require.Equal(t, 30*time.Second, cfg.http.timeout)
	require.Equal(t, "test-agent", cfg.http.userAgent)
}

func TestDefaultConfigValidation(t *testing.T) {
	cfg := defaultConfig()
	require.NoError(t, cfg.validate())
	require.Len(t, cfg.familySources(sourceFamilyASN), 1)
	require.Len(t, cfg.familySources(sourceFamilyGeo), 1)
	require.True(t, cfg.hasFamily(sourceFamilyASN))
	require.True(t, cfg.hasFamily(sourceFamilyGeo))
}

func TestConfigValidationAllowsAllFamiliesDisabled(t *testing.T) {
	cfg := defaultConfig()
	cfg.sources = nil
	require.NoError(t, cfg.validate())
	require.False(t, cfg.hasFamily(sourceFamilyASN))
	require.False(t, cfg.hasFamily(sourceFamilyGeo))
}

func TestDefaultConfigUsesBuildInfoCacheDir(t *testing.T) {
	oldCacheDir := buildinfo.CacheDir
	t.Cleanup(func() {
		buildinfo.CacheDir = oldCacheDir
	})

	buildinfo.CacheDir = "/opt/netdata/var/cache/netdata"
	cfg := defaultConfig()
	require.Equal(t, "/opt/netdata/var/cache/netdata/topology-ip-intel", cfg.output.directory)
}

func TestDiscoverDefaultConfigPathUsesBuildInfoDirs(t *testing.T) {
	oldUserConfigDir := buildinfo.UserConfigDir
	oldStockConfigDir := buildinfo.StockConfigDir
	t.Cleanup(func() {
		buildinfo.UserConfigDir = oldUserConfigDir
		buildinfo.StockConfigDir = oldStockConfigDir
	})

	root := t.TempDir()
	userDir := filepath.Join(root, "opt", "netdata", "etc", "netdata")
	stockDir := filepath.Join(root, "opt", "netdata", "usr", "lib", "netdata", "conf.d")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.MkdirAll(stockDir, 0o755))

	userPath := filepath.Join(userDir, "topology-ip-intel.yaml")
	stockPath := filepath.Join(stockDir, "topology-ip-intel.yaml")

	buildinfo.UserConfigDir = userDir
	buildinfo.StockConfigDir = stockDir

	require.NoError(t, os.WriteFile(stockPath, []byte("output: {}\n"), 0o644))
	require.Equal(t, stockPath, discoverDefaultConfigPath())

	require.NoError(t, os.WriteFile(userPath, []byte("output: {}\n"), 0o644))
	require.Equal(t, userPath, discoverDefaultConfigPath())
}

func TestInvalidOutputNamesRejected(t *testing.T) {
	cfg := defaultConfig()
	cfg.output.asnFile = "nested/asn.mmdb"
	err := cfg.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "output.asn_file")
}

func TestNormalizeSourceEntryInfersFormatFromPath(t *testing.T) {
	source := sourceEntry{
		family:   sourceFamilyGeo,
		provider: providerDBIP,
		artifact: artifactDBIPCityLite,
		path:     "/tmp/dbip-city-lite.mmdb.gz",
	}
	require.NoError(t, normalizeSourceEntry(&source))
	require.Equal(t, formatMMDB, source.format)
	require.Equal(t, "dbip-city-lite", source.name)
}
