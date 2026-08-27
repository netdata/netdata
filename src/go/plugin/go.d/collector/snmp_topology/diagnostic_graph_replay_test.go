// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func TestDisabledTopologyDiagnosticsAllocateNothing(t *testing.T) {
	collector := &Collector{}
	generation := &topologyGeneration{sequence: 1}
	options := topologyoptions.DefaultQueryOptions()

	deviceCaptureAllocs := testing.AllocsPerRun(1_000, func() {
		if capture := collector.beginTopologyDiagnosticCapture(1); capture != nil {
			panic("disabled device diagnostics returned a capture")
		}
	})
	graphCaptureAllocs := testing.AllocsPerRun(1_000, func() {
		if capture := beginTopologyGraphDiagnosticCapture(nil, generation, options); capture != nil {
			panic("disabled graph diagnostics returned a capture")
		}
	})

	assert.Zero(t, deviceCaptureAllocs)
	assert.Zero(t, graphCaptureAllocs)
}

func TestGraphDNSReplay_PreservesThreeStateOrder(t *testing.T) {
	replay := graphDNSReplay{records: []diagnostic.DNSRecordV1{
		{Ordinal: 0, IP: "192.0.2.1", State: diagnostic.DNSStateMiss},
		{Ordinal: 1, IP: "192.0.2.2", State: diagnostic.DNSStatePositive, Name: "switch.example.test"},
		{Ordinal: 2, IP: "192.0.2.3", State: diagnostic.DNSStateCachedNegative},
	}}

	assert.Empty(t, replay.lookup("192.0.2.1"))
	assert.Equal(t, "switch.example.test", replay.lookup("192.0.2.2"))
	assert.Empty(t, replay.lookup("192.0.2.3"))
	require.NoError(t, replay.err)
	assert.Equal(t, len(replay.records), replay.position)
}

func TestGraphOUIReplay_PreservesNegativeAttemptBeforeWinner(t *testing.T) {
	replay := graphOUIReplay{records: []diagnostic.OUIRecordV1{
		{Ordinal: 0, MAC: "00:00:00:00:00:01"},
		{Ordinal: 1, MAC: "00:50:56:ab:cd:ef", Vendor: "VMware, Inc.", Prefix: "005056"},
	}}

	vendor, prefix := replay.lookup("00:00:00:00:00:01")
	assert.Empty(t, vendor)
	assert.Empty(t, prefix)
	vendor, prefix = replay.lookup("00:50:56:ab:cd:ef")
	assert.Equal(t, "VMware, Inc.", vendor)
	assert.Equal(t, "005056", prefix)
	require.NoError(t, replay.err)
	assert.Equal(t, len(replay.records), replay.position)
}

func TestGraphTraceReplay_RejectsChangedDecisionOrder(t *testing.T) {
	dnsReplay := graphDNSReplay{records: []diagnostic.DNSRecordV1{
		{Ordinal: 0, IP: "192.0.2.1", State: diagnostic.DNSStateMiss},
	}}
	assert.Empty(t, dnsReplay.lookup("192.0.2.2"))
	require.ErrorContains(t, dnsReplay.err, "replay requested")

	ouiReplay := graphOUIReplay{records: []diagnostic.OUIRecordV1{
		{Ordinal: 0, MAC: "00:50:56:ab:cd:ef", Vendor: "VMware, Inc.", Prefix: "005056"},
	}}
	vendor, prefix := ouiReplay.lookup("00:00:00:00:00:01")
	assert.Empty(t, vendor)
	assert.Empty(t, prefix)
	require.ErrorContains(t, ouiReplay.err, "replay requested")
}

func TestGraphReplayWorkBudget_CoversNestedObservationStructure(t *testing.T) {
	observation := diagnostic.ObservationV1{
		LocalDevice: diagnostic.SemanticDeviceDTO{
			Labels: map[string]string{"site": "lab"},
			InterfaceCharts: map[string]diagnostic.SemanticChartRefV1{
				"1": {AvailableMetrics: []string{"traffic", "errors"}},
			},
		},
		L2: []diagnostic.L2ObservationV1{{ManagementAliases: []string{"192.0.2.2"}}},
	}
	work := replayWorkBudget{limit: 6}
	require.ErrorContains(t, addGraphObservationWork(&work, observation), "replay work exceeds limit 6")
}
