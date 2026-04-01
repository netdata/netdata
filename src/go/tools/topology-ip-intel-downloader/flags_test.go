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
