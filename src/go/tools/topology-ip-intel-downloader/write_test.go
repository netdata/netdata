// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/stretchr/testify/require"
)

func TestWriteOutputsAndClassifications(t *testing.T) {
	cfg := defaultConfig()
	cfg.output.directory = t.TempDir()
	cfg.output.asnFile = "asn.mmdb"
	cfg.output.countryFile = "country.mmdb"
	cfg.output.metadataFile = "meta.json"
	cfg.policy.localhostCIDRs = []string{"127.0.0.0/8"}
	cfg.policy.privateCIDRs = []string{"10.0.0.0/8"}
	cfg.policy.interestingCIDRs = []string{"203.0.113.0/24"}

	asnRanges := []asnRange{{
		start: mustParseAddr(t, "1.0.0.0"),
		end:   mustParseAddr(t, "1.0.0.255"),
		asn:   13335,
		org:   "Cloudflare",
	}}
	countryRanges := []countryRange{{
		start:   mustParseAddr(t, "1.0.0.0"),
		end:     mustParseAddr(t, "1.0.0.255"),
		country: "US",
	}}

	require.NoError(t, writeOutputs(cfg, asnRanges, countryRanges))

	loadOpts := mmdbwriter.Options{IncludeReservedNetworks: true, DisableIPv4Aliasing: true}
	asnDB, err := mmdbwriter.Load(filepath.Join(cfg.output.directory, cfg.output.asnFile), loadOpts)
	require.NoError(t, err)

	countryDB, err := mmdbwriter.Load(filepath.Join(cfg.output.directory, cfg.output.countryFile), loadOpts)
	require.NoError(t, err)

	publicRec := mustLookupMap(t, asnDB, "1.0.0.1")
	require.Equal(t, uint32(13335), uint32(publicRec["autonomous_system_number"].(mmdbtype.Uint32)))
	require.Equal(t, "Cloudflare", string(publicRec["autonomous_system_organization"].(mmdbtype.String)))

	localhostRec := mustLookupMap(t, asnDB, "127.0.0.1")
	require.Equal(t, "localhost", netdataClass(localhostRec))
	require.True(t, netdataTrackIndividual(localhostRec))

	privateRec := mustLookupMap(t, asnDB, "10.1.2.3")
	require.Equal(t, "private", netdataClass(privateRec))
	require.True(t, netdataTrackIndividual(privateRec))

	interestingRec := mustLookupMap(t, asnDB, "203.0.113.55")
	require.Equal(t, "interesting", netdataClass(interestingRec))
	require.True(t, netdataTrackIndividual(interestingRec))

	countryRec := mustLookupMap(t, countryDB, "1.0.0.8")
	countryMap, ok := countryRec["country"].(mmdbtype.Map)
	require.True(t, ok)
	require.Equal(t, "US", string(countryMap["iso_code"].(mmdbtype.String)))

	metaPath := filepath.Join(cfg.output.directory, cfg.output.metadataFile)
	_, err = os.Stat(metaPath)
	require.NoError(t, err)
}

func TestAtomicReplaceExistingFile(t *testing.T) {
	cfg := defaultConfig()
	cfg.output.directory = t.TempDir()
	cfg.output.asnFile = "asn.mmdb"
	cfg.output.countryFile = "country.mmdb"
	cfg.output.metadataFile = "meta.json"

	asnPath := filepath.Join(cfg.output.directory, cfg.output.asnFile)
	countryPath := filepath.Join(cfg.output.directory, cfg.output.countryFile)
	require.NoError(t, os.WriteFile(asnPath, []byte("stale"), 0o644))
	require.NoError(t, os.WriteFile(countryPath, []byte("stale"), 0o644))

	require.NoError(t, writeOutputs(cfg, nil, nil))

	loadOpts := mmdbwriter.Options{IncludeReservedNetworks: true, DisableIPv4Aliasing: true}
	asnDB, err := mmdbwriter.Load(asnPath, loadOpts)
	require.NoError(t, err)

	countryDB, err := mmdbwriter.Load(countryPath, loadOpts)
	require.NoError(t, err)
	_, _ = asnDB, countryDB
}

func mustParseAddr(t *testing.T, value string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(value)
	require.NoError(t, err)
	return addr
}

func mustLookupMap(t *testing.T, tree *mmdbwriter.Tree, ip string) mmdbtype.Map {
	t.Helper()
	_, v := tree.Get(net.ParseIP(ip))
	m, ok := v.(mmdbtype.Map)
	require.True(t, ok)
	return m
}

func netdataClass(record mmdbtype.Map) string {
	netdata, ok := record["netdata"].(mmdbtype.Map)
	if !ok {
		return ""
	}
	classValue, ok := netdata["ip_class"].(mmdbtype.String)
	if !ok {
		return ""
	}
	return string(classValue)
}

func netdataTrackIndividual(record mmdbtype.Map) bool {
	netdata, ok := record["netdata"].(mmdbtype.Map)
	if !ok {
		return false
	}
	flag, ok := netdata["track_individual"].(mmdbtype.Bool)
	if !ok {
		return false
	}
	return bool(flag)
}
