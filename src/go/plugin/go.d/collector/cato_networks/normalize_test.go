// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"testing"

	catosdk "github.com/catonetworks/cato-go-sdk"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBGP(t *testing.T) {
	tests := map[string]struct {
		peers      []*catosdk.SiteBgpStatusResult
		wantIssues []string
		check      func(*testing.T, []bgpPeerState)
	}{
		"drops peers without remote identity": {
			peers: []*catosdk.SiteBgpStatusResult{
				{IncomingConnection: catosdk.IncomingConnection{State: "Established"}},
			},
			wantIssues: []string{normalizationIssueEmptyPeer},
			check: func(t *testing.T, peers []bgpPeerState) {
				require.Empty(t, peers)
			},
		},
		"deduplicates peer metric labels": {
			peers: []*catosdk.SiteBgpStatusResult{
				{RemoteIP: "192.0.2.10", RemoteASN: "64512", RoutesCount: "1"},
				{RemoteIP: "192.0.2.10", RemoteASN: "64512", RoutesCount: "2"},
			},
			check: func(t *testing.T, peers []bgpPeerState) {
				require.Len(t, peers, 1)
				require.Equal(t, int64(2), peers[0].RoutesCount)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			peers, issues := normalizeBGP(tc.peers)

			require.Equal(t, tc.wantIssues, issues)
			tc.check(t, peers)
		})
	}
}

func TestNormalizeSnapshot(t *testing.T) {
	tests := map[string]struct {
		snapshot *catosdk.AccountSnapshot
		names    map[string]string
		check    func(*testing.T, map[string]*siteState, []string)
	}{
		"defaults nil info and statuses": {
			snapshot: &catosdk.AccountSnapshot{AccountSnapshot: &catosdk.AccountSnapshot_AccountSnapshot{
				Sites: []*catosdk.AccountSnapshot_AccountSnapshot_Sites{
					{ID: new("1001")},
				},
			}},
			names: map[string]string{"1001": "Site One"},
			check: func(t *testing.T, sites map[string]*siteState, order []string) {
				require.Equal(t, []string{"1001"}, order)
				require.Equal(t, "Site One", sites["1001"].Name)
				require.Empty(t, sites["1001"].Description)
				require.Empty(t, sites["1001"].CountryCode)
				require.Empty(t, sites["1001"].CountryName)
				require.Empty(t, sites["1001"].Region)
				require.Empty(t, sites["1001"].SiteType)
				require.Empty(t, sites["1001"].ConnectionType)
				require.Equal(t, "unknown", sites["1001"].ConnectivityStatus)
				require.Equal(t, "unknown", sites["1001"].OperationalStatus)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sites, order := normalizeSnapshot(tc.snapshot, tc.names)
			tc.check(t, sites, order)
		})
	}
}

func TestMergeMetrics(t *testing.T) {
	tests := map[string]struct {
		metrics *catosdk.AccountMetrics
		sites   map[string]*siteState
		check   func(*testing.T, map[string]*siteState, []string)
	}{
		"merges all-interface metrics into site metrics": {
			metrics: func() *catosdk.AccountMetrics {
				siteBytesUpstream := float64(100)
				siteRTT := int64(42)
				return &catosdk.AccountMetrics{AccountMetrics: &catosdk.AccountMetrics_AccountMetrics{
					Sites: []*catosdk.AccountMetrics_AccountMetrics_Sites{
						{
							ID: new("1001"),
							Metrics: &catosdk.AccountMetrics_AccountMetrics_Sites_Metrics{
								BytesUpstream: &siteBytesUpstream,
								Rtt:           &siteRTT,
							},
							Interfaces: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces{
								{
									Name: new("all"),
									Timeseries: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces_Timeseries{
										{Label: "bytesDownstreamMax", Data: [][]float64{{1, 200}}},
									},
								},
							},
						},
					},
				}}
			}(),
			sites: map[string]*siteState{
				"1001": {ID: "1001", Interfaces: make(map[string]*interfaceState)},
			},
			check: func(t *testing.T, sites map[string]*siteState, issues []string) {
				require.Empty(t, issues)
				require.Equal(t, float64(100), sites["1001"].Metrics.BytesUpstreamMax)
				require.Equal(t, float64(200), sites["1001"].Metrics.BytesDownstreamMax)
				require.Equal(t, float64(42), sites["1001"].Metrics.RTTMS)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			issues := mergeMetrics(tc.metrics, tc.sites)
			tc.check(t, tc.sites, issues)
		})
	}
}

func TestBGPSessionState(t *testing.T) {
	tests := map[string]struct {
		status string
		want   string
	}{
		"established":     {status: "Established", want: "up"},
		"up":              {status: "up", want: "up"},
		"not established": {status: "not_established", want: "down"},
		"idle":            {status: "idle", want: "down"},
		"empty":           {status: "", want: "unknown"},
		"unknown":         {status: "unknown", want: "unknown"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, bgpSessionState(tc.status))
		})
	}
}
