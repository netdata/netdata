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

func Test_CiscoBGPProfileMergedIntoCiscoASR(t *testing.T) {
	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	matched := FindProfiles("1.3.6.1.4.1.9.1.923", "", nil) // ciscoASR1002
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "cisco-asr.yaml")
	})
	require.NotEqual(t, -1, index, "expected cisco-asr profile to match")

	profile := matched[index]

	metricIndex := slices.IndexFunc(profile.Definition.Metrics, func(m ddprofiledefinition.MetricsConfig) bool {
		return m.Table.Name == "cbgpPeer3Table" && m.Table.OID == "1.3.6.1.4.1.9.9.187.1.2.9"
	})
	require.NotEqual(t, -1, metricIndex, "expected merged Cisco profile to include cbgpPeer3Table")

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
	assert.Contains(t, symbolNames, "bgpPeerInUpdates")
	assert.Contains(t, symbolNames, "bgpPeerOutTotalMessages")
	assert.Contains(t, symbolNames, "bgpPeerLastErrorCode")
	assert.Contains(t, symbolNames, "bgpPeerLastErrorSubcode")
	assert.Contains(t, symbolNames, "bgpPeerFsmEstablishedTransitions")
	assert.Contains(t, symbolNames, "bgpPeerFsmEstablishedTime")
	assert.Contains(t, symbolNames, "bgpPeerInUpdateElapsedTime")
	assert.Contains(t, symbolNames, "bgpPeerPreviousState")

	assert.Contains(t, tagNames, "routing_instance")
	assert.Contains(t, tagNames, "routing_instance_id")
	assert.Contains(t, tagNames, "neighbor_address_type")
	assert.Contains(t, tagNames, "neighbor")
	assert.Contains(t, tagNames, "local_address")
	assert.Contains(t, tagNames, "local_as")
	assert.Contains(t, tagNames, "remote_as")
	assert.Contains(t, tagNames, "local_identifier")
	assert.Contains(t, tagNames, "peer_identifier")
	assert.Contains(t, tagNames, "bgp_version")
}

func Test_CiscoBgpPrefixProfileMergedIntoCiscoASR(t *testing.T) {
	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	matched := FindProfiles("1.3.6.1.4.1.9.1.923", "", nil) // ciscoASR1002
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "cisco-asr.yaml")
	})
	require.NotEqual(t, -1, index, "expected cisco-asr profile to match")

	profile := matched[index]

	metricIndex := slices.IndexFunc(profile.Definition.Metrics, func(m ddprofiledefinition.MetricsConfig) bool {
		return m.Table.Name == "cbgpPeer2AddrFamilyPrefixTable" && m.Table.OID == "1.3.6.1.4.1.9.9.187.1.2.8"
	})
	require.NotEqual(t, -1, metricIndex, "expected merged Cisco profile to include cbgpPeer2AddrFamilyPrefixTable")

	metric := profile.Definition.Metrics[metricIndex]

	var symbolNames []string
	for _, sym := range metric.Symbols {
		symbolNames = append(symbolNames, sym.Name)
	}

	var tagNames []string
	for _, tag := range metric.MetricTags {
		tagNames = append(tagNames, tag.Tag)
	}

	assert.Contains(t, symbolNames, "bgpPeerPrefixesAccepted")
	assert.Contains(t, symbolNames, "bgpPeerPrefixesRejected")
	assert.Contains(t, symbolNames, "bgpPeerPrefixAdminLimit")
	assert.Contains(t, symbolNames, "bgpPeerPrefixThreshold")
	assert.Contains(t, symbolNames, "bgpPeerPrefixClearThreshold")
	assert.Contains(t, symbolNames, "bgpPeerPrefixesAdvertised")
	assert.Contains(t, symbolNames, "bgpPeerPrefixesSuppressed")
	assert.Contains(t, symbolNames, "bgpPeerPrefixesWithdrawn")

	assert.Contains(t, tagNames, "neighbor_address_type")
	assert.Contains(t, tagNames, "neighbor")
	assert.Contains(t, tagNames, "local_address")
	assert.Contains(t, tagNames, "local_as")
	assert.Contains(t, tagNames, "remote_as")
	assert.Contains(t, tagNames, "local_identifier")
	assert.Contains(t, tagNames, "peer_identifier")
	assert.Contains(t, tagNames, "bgp_version")
	assert.Contains(t, tagNames, "address_family_name")
	assert.Contains(t, tagNames, "address_family")
	assert.Contains(t, tagNames, "subsequent_address_family")
}
