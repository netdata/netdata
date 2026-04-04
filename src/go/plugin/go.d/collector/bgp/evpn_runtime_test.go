// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

type evpnVNIErrorClient struct {
	*mockClient
	err error
}

func (c *evpnVNIErrorClient) EVPNVNI() ([]byte, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.mockClient.EVPNVNI()
}

func TestCollector_CollectKeepsBGPMetricsWhenEVPNVNIQueryFails(t *testing.T) {
	collr := New()
	collr.newClient = func(Config) (bgpClient, error) {
		return &evpnVNIErrorClient{
			mockClient: &mockClient{
				responses: map[string][]byte{
					"ipv4":       dataFRREmptySummary,
					"ipv6":       dataFRREmptySummary,
					"l2vpn/evpn": dataFRREVPNSummary,
				},
				neighbors: dataFRRNeighborsEnriched,
			},
			err: errors.New("mock zebra.vty error"),
		}, nil
	}
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 1, 0, 0, 0, 0, 0)

	peerID := makePeerID("default_l2vpn_evpn", "192.168.0.2")
	assert.Equal(t, int64(1), mx["family_default_l2vpn_evpn_peers_established"])
	assert.Equal(t, int64(peerStateUp), mx["peer_"+peerID+"_state"])
	assert.NotContains(t, mx, "vni_"+makeVNIID("default", 172192, "l2")+"_macs")
	assert.Nil(t, collr.Charts().Get("vni_"+makeVNIID("default", 172192, "l2")+"_entries"))
}

func TestCollector_CollectKeepsBGPMetricsWhenEVPNVNIParseFails(t *testing.T) {
	collr := New()
	collr.newClient = func(Config) (bgpClient, error) {
		return &mockClient{
			responses: map[string][]byte{
				"ipv4":       dataFRREmptySummary,
				"ipv6":       dataFRREmptySummary,
				"l2vpn/evpn": dataFRREVPNSummary,
			},
			neighbors: dataFRRNeighborsEnriched,
			evpnVNI:   []byte("{"),
		}, nil
	}
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 1, 0, 0, 0, 0)

	assert.Equal(t, int64(1), mx["family_default_l2vpn_evpn_peers_established"])
	assert.NotContains(t, mx, "vni_"+makeVNIID("default", 172192, "l2")+"_macs")
}

func TestCollector_SkipsEVPNVNIQueryWhenEVPNFamilyNotSelected(t *testing.T) {
	mock := &mockClient{
		responses: map[string][]byte{
			"ipv4":       dataFRRIPv4Summary,
			"ipv6":       dataFRREmptySummary,
			"l2vpn/evpn": dataFRREVPNSummary,
		},
		neighbors: dataFRRNeighborsEnriched,
		evpnVNI:   dataFRREVPNVNI,
	}

	collr := New()
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	collr.MaxFamilies = 1
	collr.SelectFamilies = matcher.SimpleExpr{Includes: []string{"=default/ipv4/unicast"}}
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	_ = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t, 0, mock.evpnVNICalls)
	assert.NotContains(t, mx, "vni_"+makeVNIID("default", 172192, "l2")+"_macs")
}

func TestCollector_ObsoleteEVPNVNIChartsAfterAbsence(t *testing.T) {
	mock := &mockClient{
		responses: map[string][]byte{
			"ipv4":       dataFRREmptySummary,
			"ipv6":       dataFRREmptySummary,
			"l2vpn/evpn": dataFRREVPNSummary,
		},
		neighbors: dataFRRNeighborsEnriched,
		evpnVNI:   dataFRREVPNVNI,
	}

	collr := New()
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	collr.cleanupEvery = 0
	collr.obsoleteAfter = time.Minute
	require.NoError(t, collr.Init(context.Background()))

	require.NotNil(t, collr.Collect(context.Background()))
	vniID := makeVNIID("default", 172192, "l2")
	require.NotNil(t, collr.Charts().Get("vni_"+vniID+"_entries"))

	for id := range collr.vniSeen {
		collr.vniSeen[id] = time.Now().Add(-2 * time.Minute)
	}
	mock.responses["l2vpn/evpn"] = dataFRREmptySummary
	mock.evpnVNI = dataFRREmptySummary

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	_ = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	require.NotNil(t, collr.Charts().Get("vni_"+vniID+"_entries"))
	assert.True(t, collr.Charts().Get("vni_"+vniID+"_entries").Obsolete)
	assert.True(t, collr.Charts().Get("vni_"+vniID+"_remote_vteps").Obsolete)
}
