// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSourceToken(t *testing.T) {
	source, err := parseSourceToken(sourceFamilyGeo, "dbip:city-lite@csv")
	require.NoError(t, err)
	require.Equal(t, sourceFamilyGeo, source.family)
	require.Equal(t, providerDBIP, source.provider)
	require.Equal(t, artifactDBIPCityLite, source.artifact)
	require.Equal(t, formatCSV, source.format)
	require.Equal(t, "dbip-city-lite", source.name)
}

func TestParseSourceTokenUsesBuiltInDefaultFormat(t *testing.T) {
	source, err := parseSourceToken(sourceFamilyASN, "dbip:asn-lite")
	require.NoError(t, err)
	require.Equal(t, formatMMDB, source.format)
}

func TestParseSourceTokenSupportsAllBuiltInProviderFamilies(t *testing.T) {
	tests := map[string]struct {
		family   string
		token    string
		provider string
		artifact string
		format   string
	}{
		"caida asn": {
			family:   sourceFamilyASN,
			token:    "caida:prefix2as",
			provider: providerCAIDA,
			artifact: artifactCAIDAPrefix2AS,
			format:   formatTSV,
		},
		"maxmind asn": {
			family:   sourceFamilyASN,
			token:    "maxmind:geolite2-asn",
			provider: providerMaxMind,
			artifact: artifactMaxMindGeoLite2ASN,
			format:   formatMMDB,
		},
		"maxmind country": {
			family:   sourceFamilyGeo,
			token:    "maxmind:geolite2-country",
			provider: providerMaxMind,
			artifact: artifactMaxMindGeoLite2Country,
			format:   formatCSV,
		},
		"ip2location country": {
			family:   sourceFamilyGeo,
			token:    "ip2location:country-lite",
			provider: providerIP2Location,
			artifact: artifactIP2LocationCountryLite,
			format:   formatCSV,
		},
		"ipdeny country": {
			family:   sourceFamilyGeo,
			token:    "ipdeny:country-zones",
			provider: providerIPDeny,
			artifact: artifactIPDenyCountryZones,
			format:   formatCIDR,
		},
		"ipip country": {
			family:   sourceFamilyGeo,
			token:    "ipip:country",
			provider: providerIPIP,
			artifact: artifactIPIPCountry,
			format:   formatTXT,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			source, err := parseSourceToken(tc.family, tc.token)
			require.NoError(t, err)
			require.Equal(t, tc.provider, source.provider)
			require.Equal(t, tc.artifact, source.artifact)
			require.Equal(t, tc.format, source.format)
		})
	}
}

func TestParseSourceTokenRejectsInvalidFamilyArtifactCombination(t *testing.T) {
	_, err := parseSourceToken(sourceFamilyGeo, "dbip:asn-lite")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not compatible with family")
}

func TestApplyFamilyOverrideReplacesOnlyTargetFamily(t *testing.T) {
	current := []sourceEntry{
		{family: sourceFamilyASN, provider: providerDBIP, artifact: artifactDBIPASNLite, format: formatMMDB},
		{family: sourceFamilyGeo, provider: providerDBIP, artifact: artifactDBIPCityLite, format: formatMMDB},
	}
	override := []sourceEntry{
		{family: sourceFamilyASN, provider: providerIPToASN, artifact: artifactIPToASNCombined, format: formatTSV},
	}

	next := applyFamilyOverride(current, sourceFamilyASN, override, false)
	require.Equal(t, []sourceEntry{
		{family: sourceFamilyGeo, provider: providerDBIP, artifact: artifactDBIPCityLite, format: formatMMDB},
		{family: sourceFamilyASN, provider: providerIPToASN, artifact: artifactIPToASNCombined, format: formatTSV},
	}, next)
}

func TestApplyFamilyOverrideDisablesFamily(t *testing.T) {
	current := []sourceEntry{
		{family: sourceFamilyASN, provider: providerDBIP, artifact: artifactDBIPASNLite, format: formatMMDB},
		{family: sourceFamilyGeo, provider: providerDBIP, artifact: artifactDBIPCityLite, format: formatMMDB},
	}

	next := applyFamilyOverride(current, sourceFamilyGeo, nil, true)
	require.Equal(t, []sourceEntry{
		{family: sourceFamilyASN, provider: providerDBIP, artifact: artifactDBIPASNLite, format: formatMMDB},
	}, next)
}
