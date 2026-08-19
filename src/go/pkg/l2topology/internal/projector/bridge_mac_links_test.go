// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBridgeMacLinkRecords_PreservesAmbiguousRawDomains(t *testing.T) {
	records := []bridgeMacLinkRecord{
		{
			port:       bridgePortRef{deviceID: "switch-a", ifIndex: 1, fdbDomainID: "fdb:500", vlanID: "100"},
			endpointID: "mac:02:00:00:00:01:01",
			method:     "fdb",
		},
		{
			port:       bridgePortRef{deviceID: "switch-a", ifIndex: 1, fdbDomainID: "fdb:600", vlanID: "100"},
			endpointID: "mac:02:00:00:00:01:02",
			method:     "fdb",
		},
		{
			port:       bridgePortRef{deviceID: "switch-a", ifIndex: 1, vlanID: "100"},
			endpointID: "mac:02:00:00:00:01:03",
			method:     "fdb",
		},
	}

	got := normalizeBridgeMacLinkRecords(records)
	require.Len(t, got, 3)
	require.Equal(t, []string{"fdb:500", "fdb:600", "vlan:100"}, bridgeMacLinkScopes(got))

	reversed := slices.Clone(records)
	slices.Reverse(reversed)
	require.Equal(t, got, normalizeBridgeMacLinkRecords(reversed))
}

func TestNormalizeBridgeMacLinkRecords_KeepsDistinctRawDomainsForSameEndpoint(t *testing.T) {
	records := []bridgeMacLinkRecord{
		{
			port:       bridgePortRef{deviceID: "switch-a", ifIndex: 1, fdbDomainID: "fdb:500"},
			endpointID: "mac:02:00:00:00:01:01",
			method:     "fdb",
		},
		{
			port:       bridgePortRef{deviceID: "switch-a", ifIndex: 1, fdbDomainID: "fdb:600"},
			endpointID: "mac:02:00:00:00:01:01",
			method:     "fdb",
		},
	}

	got := normalizeBridgeMacLinkRecords(records)
	require.Len(t, got, 2)
	require.Equal(t, []string{"fdb:500", "fdb:600"}, bridgeMacLinkScopes(got))
}

func TestNormalizeBridgeMacLinkRecords_UniqueVLANAliasIsOrderIndependent(t *testing.T) {
	records := []bridgeMacLinkRecord{
		{
			port:       bridgePortRef{deviceID: "switch-a", ifIndex: 1, fdbDomainID: "fdb:500", vlanID: "100"},
			endpointID: "mac:02:00:00:00:01:01",
			method:     "fdb",
		},
		{
			port:       bridgePortRef{deviceID: "switch-a", ifIndex: 1, fdbDomainID: "vlan:100", vlanID: "100"},
			endpointID: "mac:02:00:00:00:01:02",
			method:     "fdb",
		},
	}

	got := normalizeBridgeMacLinkRecords(records)
	require.Len(t, got, 2)
	require.Equal(t, []string{"fdb:500", "fdb:500"}, bridgeMacLinkScopes(got))

	reversed := slices.Clone(records)
	slices.Reverse(reversed)
	require.Equal(t, got, normalizeBridgeMacLinkRecords(reversed))
}

func bridgeMacLinkScopes(records []bridgeMacLinkRecord) []string {
	scopes := make([]string, 0, len(records))
	for _, record := range records {
		scopes = append(scopes, bridgePortForwardingDomain(record.port))
	}
	slices.Sort(scopes)
	return scopes
}
