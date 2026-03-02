// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigCustomFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "topology-ip-intel.yaml")
	err := os.WriteFile(cfgPath, []byte(`
source:
  provider: custom
  asn:
    path: /tmp/asn.csv
    format: dbip_asn_csv
  country:
    path: /tmp/country.csv
    format: dbip_country_csv
output:
  directory: /tmp/netdata-ip-intel
  asn_file: asn.mmdb
  country_file: country.mmdb
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
	require.Equal(t, providerCustom, cfg.source.provider)
	require.Equal(t, "", cfg.source.combined.url)
	require.Equal(t, "/tmp/asn.csv", cfg.source.asn.path)
	require.Equal(t, formatDBIPAsnCSV, cfg.source.asn.format)
	require.Equal(t, "/tmp/country.csv", cfg.source.country.path)
	require.Equal(t, "/tmp/netdata-ip-intel", cfg.output.directory)
	require.Equal(t, "asn.mmdb", cfg.output.asnFile)
	require.Equal(t, "country.mmdb", cfg.output.countryFile)
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
}

func TestInvalidOutputNamesRejected(t *testing.T) {
	cfg := defaultConfig()
	cfg.output.asnFile = "nested/asn.mmdb"
	err := cfg.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "output.asn_file")
}
