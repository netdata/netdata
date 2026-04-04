// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func Test_NokiaBgpPeerProfileMergedIntoNokiaSROS(t *testing.T) {
	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	matched := FindProfiles("1.3.6.1.4.1.6527.1.3.17", "", nil)
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "nokia-service-router-os.yaml")
	})
	require.NotEqual(t, -1, index, "expected nokia-service-router-os profile to match")

	profile := matched[index]

	metricIndex := slices.IndexFunc(profile.Definition.Metrics, func(m ddprofiledefinition.MetricsConfig) bool {
		return m.Table.Name == "tBgpPeerNgTable" && m.Table.OID == "1.3.6.1.4.1.6527.3.1.2.14.4.7"
	})
	require.NotEqual(t, -1, metricIndex, "expected merged Nokia profile to include tBgpPeerNgTable")

	metric := profile.Definition.Metrics[metricIndex]

	var symbolNames []string
	for _, sym := range metric.Symbols {
		symbolNames = append(symbolNames, sym.Name)
	}

	var tagNames []string
	for _, tag := range metric.MetricTags {
		tagNames = append(tagNames, tag.Tag)
	}

	assert.Contains(t, symbolNames, "bgpPeerAdminStatus")
	assert.Contains(t, symbolNames, "bgpPeerState")
	assert.Contains(t, symbolNames, "bgpPeerConnectRetryInterval")
	assert.Contains(t, symbolNames, "bgpPeerHoldTimeConfigured")
	assert.Contains(t, symbolNames, "bgpPeerKeepAliveConfigured")
	assert.Contains(t, symbolNames, "bgpPeerMinRouteAdvertisementInterval")

	assert.Contains(t, tagNames, "routing_instance")
	assert.Contains(t, tagNames, "neighbor_address_type")
	assert.Contains(t, tagNames, "neighbor")
	assert.Contains(t, tagNames, "local_address_type")
	assert.Contains(t, tagNames, "local_address")
	assert.Contains(t, tagNames, "local_as")
	assert.Contains(t, tagNames, "remote_as")
	assert.Contains(t, tagNames, "peer_description")
}

func Test_NokiaBgpOperationalProfilesMergedIntoNokiaSROS(t *testing.T) {
	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	matched := FindProfiles("1.3.6.1.4.1.6527.1.3.17", "", nil)
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "nokia-service-router-os.yaml")
	})
	require.NotEqual(t, -1, index, "expected nokia-service-router-os profile to match")

	profile := matched[index]

	peerOperIndex := slices.IndexFunc(profile.Definition.Metrics, func(m ddprofiledefinition.MetricsConfig) bool {
		return m.Table.Name == "tBgpPeerNgOperTable" &&
			hasSymbol(m.Symbols, "bgpPeerInUpdates") &&
			hasSymbol(m.Symbols, "bgpPeerFsmEstablishedTime")
	})
	require.NotEqual(t, -1, peerOperIndex, "expected merged Nokia profile to include peer operational counters")

	peerOper := profile.Definition.Metrics[peerOperIndex]

	var peerOperSymbols []string
	for _, sym := range peerOper.Symbols {
		peerOperSymbols = append(peerOperSymbols, sym.Name)
	}

	assert.Contains(t, peerOperSymbols, "bgpPeerFlaps")
	assert.Contains(t, peerOperSymbols, "bgpPeerFsmEstablishedTime")
	assert.Contains(t, peerOperSymbols, "bgpPeerLastErrorCode")
	assert.Contains(t, peerOperSymbols, "bgpPeerLastErrorSubcode")
	assert.Contains(t, peerOperSymbols, "bgpPeerIdentifier")
	assert.Contains(t, peerOperSymbols, "bgpPeerInUpdates")
	assert.Contains(t, peerOperSymbols, "bgpPeerOutUpdates")
	assert.Contains(t, peerOperSymbols, "bgpPeerInTotalMessages")
	assert.Contains(t, peerOperSymbols, "bgpPeerOutTotalMessages")
	assert.Contains(t, peerOperSymbols, "bgpPeerInNotifications")
	assert.Contains(t, peerOperSymbols, "bgpPeerOutNotifications")
	assert.Contains(t, peerOperSymbols, "bgpPeerInUpdateElapsedTime")

	familyIndex := slices.IndexFunc(profile.Definition.Metrics, func(m ddprofiledefinition.MetricsConfig) bool {
		return m.Table.Name == "tBgpPeerNgOperTable" &&
			hasStaticTag(m.StaticTags, "address_family", "ipv4") &&
			hasStaticTag(m.StaticTags, "subsequent_address_family", "unicast")
	})
	require.NotEqual(t, -1, familyIndex, "expected merged Nokia profile to include TiMOS AFI/SAFI prefix counters")

	familyMetric := profile.Definition.Metrics[familyIndex]

	var familySymbols []string
	for _, sym := range familyMetric.Symbols {
		familySymbols = append(familySymbols, sym.Name)
	}

	assert.Contains(t, familySymbols, "bgpPeerPrefixesReceivedTotal")
	assert.Contains(t, familySymbols, "bgpPeerPrefixesAdvertisedTotal")
	assert.Contains(t, familySymbols, "bgpPeerPrefixesActive")
	assert.Contains(t, familySymbols, "bgpPeerPrefixesRejectedTotal")

	assert.True(t, hasStaticTag(familyMetric.StaticTags, "address_family_name", "ipv4 unicast"))
}

func hasSymbol(symbols []ddprofiledefinition.SymbolConfig, name string) bool {
	return slices.ContainsFunc(symbols, func(sym ddprofiledefinition.SymbolConfig) bool {
		return sym.Name == name
	})
}

func hasStaticTag(tags []ddprofiledefinition.StaticMetricTagConfig, name, value string) bool {
	return slices.ContainsFunc(tags, func(tag ddprofiledefinition.StaticMetricTagConfig) bool {
		return tag.Tag == name && tag.Value == value
	})
}
