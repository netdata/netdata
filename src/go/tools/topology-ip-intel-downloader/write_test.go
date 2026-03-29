// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
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
	cfg.output.geoFile = "geo.mmdb"
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
	geoRanges := []geoRange{{
		start:       mustParseAddr(t, "1.0.0.0"),
		end:         mustParseAddr(t, "1.0.0.255"),
		country:     "US",
		state:       "California",
		city:        "Los Angeles",
		latitude:    34.0522,
		longitude:   -118.2437,
		hasLocation: true,
	}}
	sources := []generationDatasetRef{
		{
			Name:        "dbip-asn",
			Family:      sourceFamilyASN,
			Provider:    providerDBIP,
			Artifact:    artifactDBIPASNLite,
			Source:      "builtin",
			Format:      formatMMDB,
			ResolvedURL: "https://download.db-ip.com/free/dbip-asn-lite-2026-03.mmdb.gz",
		},
		{
			Name:        "dbip-geo",
			Family:      sourceFamilyGeo,
			Provider:    providerDBIP,
			Artifact:    artifactDBIPCityLite,
			Source:      "builtin",
			Format:      formatMMDB,
			ResolvedURL: "https://download.db-ip.com/free/dbip-city-lite-2026-03.mmdb.gz",
		},
	}

	require.NoError(t, writeOutputs(cfg, asnRanges, geoRanges, sources))

	loadOpts := mmdbwriter.Options{IncludeReservedNetworks: true, DisableIPv4Aliasing: true}
	asnDB, err := mmdbwriter.Load(filepath.Join(cfg.output.directory, cfg.output.asnFile), loadOpts)
	require.NoError(t, err)

	geoDB, err := mmdbwriter.Load(filepath.Join(cfg.output.directory, cfg.output.geoFile), loadOpts)
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

	geoRec := mustLookupMap(t, geoDB, "1.0.0.8")
	countryMap, ok := geoRec["country"].(mmdbtype.Map)
	require.True(t, ok)
	require.Equal(t, "US", string(countryMap["iso_code"].(mmdbtype.String)))
	require.Equal(t, "California", string(geoRec["region"].(mmdbtype.String)))

	cityMap, ok := geoRec["city"].(mmdbtype.Map)
	require.True(t, ok)
	cityNames := cityMap["names"].(mmdbtype.Map)
	require.Equal(t, "Los Angeles", string(cityNames["en"].(mmdbtype.String)))

	locationMap, ok := geoRec["location"].(mmdbtype.Map)
	require.True(t, ok)
	require.Equal(t, float64(34.0522), float64(locationMap["latitude"].(mmdbtype.Float64)))
	require.Equal(t, float64(-118.2437), float64(locationMap["longitude"].(mmdbtype.Float64)))

	metaPath := filepath.Join(cfg.output.directory, cfg.output.metadataFile)
	metaBlob, err := os.ReadFile(metaPath)
	require.NoError(t, err)

	var meta generationMetadata
	require.NoError(t, json.Unmarshal(metaBlob, &meta))
	require.Equal(t, "topology-ip-intel-downloader", meta.GeneratedBy)
	require.Len(t, meta.Sources, 2)
	require.Equal(t, cfg.output.asnFile, meta.Output.AsnFile)
	require.Equal(t, cfg.output.geoFile, meta.Output.GeoFile)
	require.Equal(t, cfg.output.metadataFile, meta.Output.MetadataFile)
	require.Equal(t, 1, meta.Counts.AsnRanges)
	require.Equal(t, 1, meta.Counts.GeoRanges)
}

func TestWriteOutputsRemovesDisabledGeoFile(t *testing.T) {
	cfg := defaultConfig()
	cfg.output.directory = t.TempDir()
	cfg.output.asnFile = "asn.mmdb"
	cfg.output.geoFile = "geo.mmdb"
	cfg.output.metadataFile = "meta.json"
	cfg.sources = []sourceEntry{
		{
			family:   sourceFamilyASN,
			provider: providerDBIP,
			artifact: artifactDBIPASNLite,
			format:   formatMMDB,
		},
	}

	geoPath := filepath.Join(cfg.output.directory, cfg.output.geoFile)
	require.NoError(t, os.WriteFile(geoPath, []byte("stale"), 0o644))

	require.NoError(t, writeOutputs(cfg, nil, nil, nil))

	_, err := os.Stat(geoPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	metaBlob, err := os.ReadFile(filepath.Join(cfg.output.directory, cfg.output.metadataFile))
	require.NoError(t, err)
	var meta generationMetadata
	require.NoError(t, json.Unmarshal(metaBlob, &meta))
	require.Equal(t, cfg.output.asnFile, meta.Output.AsnFile)
	require.Empty(t, meta.Output.GeoFile)
}

func TestWriteOutputsDoesNotPublishHalfBuiltState(t *testing.T) {
	cfg := defaultConfig()
	cfg.output.directory = t.TempDir()
	cfg.output.asnFile = "asn.mmdb"
	cfg.output.geoFile = "geo.mmdb"
	cfg.output.metadataFile = "meta.json"

	asnPath := filepath.Join(cfg.output.directory, cfg.output.asnFile)
	geoPath := filepath.Join(cfg.output.directory, cfg.output.geoFile)
	require.NoError(t, writeOutputs(cfg, nil, nil, nil))

	asnBefore, err := os.ReadFile(asnPath)
	require.NoError(t, err)
	geoBefore, err := os.ReadFile(geoPath)
	require.NoError(t, err)

	invalidGeo := []geoRange{{
		start: mustParseAddr(t, "1.0.0.0"),
		end:   mustParseAddr(t, "2001:db8::1"),
	}}

	err = writeOutputs(cfg, nil, invalidGeo, nil)
	require.Error(t, err)

	asnAfter, err := os.ReadFile(asnPath)
	require.NoError(t, err)
	geoAfter, err := os.ReadFile(geoPath)
	require.NoError(t, err)
	require.Equal(t, asnBefore, asnAfter)
	require.Equal(t, geoBefore, geoAfter)
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
