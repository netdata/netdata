// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func TestDiagnosticProfileEvidence_UsesExplicitTopologyProjection(t *testing.T) {
	base := &ddsnmp.Profile{
		SourceFile: "/stock/profiles/profile-a.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddsnmp.KindIfName,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.2", Name: "ifTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr"}},
				},
			}},
		},
	}
	want, err := diagnosticProfileEvidence(1, 1, "main", "", 0, base)
	require.NoError(t, err)
	assert.Equal(t, "profile-a.yaml", want.Origin)
	assert.NotContains(t, want.Definition.EffectiveDefinition, "/stock/profiles")

	unrelated := &ddsnmp.Profile{SourceFile: base.SourceFile, Definition: base.Definition.Clone()}
	unrelated.Definition.Metrics = []ddprofiledefinition.MetricsConfig{{
		Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.999.1", Name: "regularMetric"},
	}}
	got, err := diagnosticProfileEvidence(1, 1, "main", "", 0, unrelated)
	require.NoError(t, err)
	assert.Equal(t, want.DefinitionSHA256, got.DefinitionSHA256)
	assert.False(t, strings.Contains(got.Definition.EffectiveDefinition, "regularMetric"))

	related := &ddsnmp.Profile{SourceFile: base.SourceFile, Definition: base.Definition.Clone()}
	related.Definition.Topology[0].Symbols[0].Name = "ifName"
	changed, err := diagnosticProfileEvidence(1, 1, "main", "", 0, related)
	require.NoError(t, err)
	assert.NotEqual(t, want.DefinitionSHA256, changed.DefinitionSHA256)
}

func TestCollectorDiagnosticCapture_ReplaysProductionSemanticPathAndExcludesCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname: "192.0.2.10", Port: 161, Community: "COMMUNITY_CANARY",
		V3User: "V3_USER_CANARY", V3AuthKey: "V3_AUTH_CANARY", V3PrivKey: "V3_PRIV_CANARY",
		V3ContextName: "V3_CONTEXT_CANARY", SysName: "switch-a", SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	profile := &ddsnmp.Profile{
		SourceFile: "/stock/path/profile-a.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddsnmp.KindFdbEntry,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.17.4.3", Name: "dot1dTpFdbTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.17.4.3.1.1", Name: "dot1dTpFdbAddress"}},
				},
			}},
		},
	}
	pms := []*ddsnmp.ProfileMetrics{{
		Source: profile.SourceFile,
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			"device_vendor": {Value: "Cisco", IsExactMatch: true},
		},
		Tags: map[string]string{"site": "lab"},
		TopologyMetrics: []ddsnmp.Metric{
			{TopologyKind: ddsnmp.KindBridgePortIfIndex, Tags: map[string]string{tagBridgeBasePort: "5", tagBridgeIfIndex: "5"}},
			{TopologyKind: ddsnmp.KindFdbEntry, Tags: map[string]string{tagFdbMac: "005056abcdef", tagFdbBridgePort: "5", tagFdbStatus: "3"}},
		},
		BGPRows: []ddsnmp.BGPRow{{
			OriginProfileID: "_cisco-bgp.yaml", Kind: ddprofiledefinition.BGPRowKindPeer,
			Table: "bgpPeerTable", RowKey: "192.0.2.20", StructuralID: "peer-1",
			Identity:    ddsnmp.BGPIdentity{Neighbor: "192.0.2.20", RemoteAS: "64501"},
			Descriptors: ddsnmp.BGPDescriptors{LocalAddress: "192.0.2.10", LocalAS: "64500"},
			State:       ddsnmp.BGPState{Has: true, State: ddprofiledefinition.BGPPeerStateEstablished},
			Connection:  ddsnmp.BGPConnection{EstablishedUptime: ddsnmp.BGPInt64{Has: true, Value: 600}},
		}},
	}}
	for i := range 300 {
		pms[0].TopologyMetrics = append(pms[0].TopologyMetrics, ddsnmp.Metric{
			TopologyKind: ddsnmp.KindIfName,
			Tags:         map[string]string{tagTopoIfIndex: fmt.Sprint(i + 1), tagTopoIfName: fmt.Sprintf("Ethernet%d", i+1)},
		})
	}

	sink := &diagnostic.MemorySink{}
	recorder, err := diagnostic.NewRecorder(diagnostic.RecorderConfig{
		QueueCapacity: 4, MaxMembers: 128, MaxRetainedBytes: 32 << 20, Sink: sink,
	})
	require.NoError(t, err)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	registerTestDeviceState(store, dev)
	coll.now = func() time.Time { return time.Date(2026, time.August, 27, 14, 0, 0, 0, time.UTC) }
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{profile} }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return pms, nil })
	}
	coll.diagnosticRecorder = recorder

	stats := coll.refreshTopology(context.Background())
	require.Zero(t, stats.errors)
	livePayload, ok, err := (funcDepsAdapter{
		registry: coll.topologyRegistry, diagnosticRecorder: recorder,
	}).Snapshot(topologyoptions.DefaultQueryOptions())
	require.NoError(t, err)
	require.True(t, ok)
	recorder.Close()

	results := sink.Results()
	require.Len(t, results, 3)
	result := diagnosticResultForCapability(t, results, diagnostic.SemanticCapabilityV1())
	generationResult := diagnosticResultForCapability(t, results, diagnostic.GenerationCapabilityV1())
	graphResult := diagnosticResultForCapability(t, results, diagnostic.GraphCapabilityV1())
	for _, ref := range graphResult.Manifest.Members {
		assert.NotEqual(t, "graph_checkpoint", ref.Kind,
			"Stage 2 must not serialize or retain live graph-output identity")
	}
	require.NoError(t, result.Err)
	require.NoError(t, result.Manifest.Validate())
	registry := diagnostic.NewRegistry()
	require.NoError(t, registry.Register(diagnostic.SemanticCapabilityV1(), diagnostic.SemanticClosureV1()))
	report, err := registry.ValidateCapability(
		result.Manifest, result.Members, diagnostic.SemanticCapabilityV1(), topologyDiagnosticReaderLimits(),
	)
	require.NoError(t, err)
	assert.True(t, report.Replayable)
	require.NoError(t, generationResult.Err)
	require.NoError(t, registry.Register(diagnostic.GenerationCapabilityV1(), diagnostic.GenerationClosureV1()))
	generationReport, err := registry.ValidateCapability(
		generationResult.Manifest, sink.Source(), diagnostic.GenerationCapabilityV1(), topologyDiagnosticReaderLimits(),
	)
	require.NoError(t, err)
	assert.True(t, generationReport.Replayable)
	require.NoError(t, registry.Register(diagnostic.GraphCapabilityV1(), diagnostic.GraphClosureV1()))
	graphReport, err := registry.ValidateCapability(
		graphResult.Manifest, sink.Source(), diagnostic.GraphCapabilityV1(), topologyDiagnosticReaderLimits(),
	)
	require.NoError(t, err)
	assert.True(t, graphReport.Replayable)
	var graphRoot diagnostic.CapabilityRootV1
	require.NoError(t, diagnostic.DecodeReferenced(
		sink.Source(), graphResult.Manifest.Roots[0].Root, topologyDiagnosticReaderLimits(), &graphRoot,
	))
	dnsSection := diagnosticSection(graphRoot, diagnostic.GraphSectionDNSTrace)
	require.Equal(t, diagnostic.StateSuccess, dnsSection.State)
	var dnsTrace diagnostic.DNSTraceV1
	require.NoError(t, diagnostic.DecodeReferenced(sink.Source(), dnsSection.Members[0], topologyDiagnosticReaderLimits(), &dnsTrace))
	require.NotEmpty(t, dnsTrace.Records)
	assert.Equal(t, diagnostic.DNSStateMiss, dnsTrace.Records[0].State)
	ouiSection := diagnosticSection(graphRoot, diagnostic.GraphSectionOUITrace)
	require.Equal(t, diagnostic.StateSuccess, ouiSection.State)
	var ouiTrace diagnostic.OUITraceV1
	require.NoError(t, diagnostic.DecodeReferenced(sink.Source(), ouiSection.Members[0], topologyDiagnosticReaderLimits(), &ouiTrace))
	require.NotEmpty(t, ouiTrace.Records)
	replayedPayload, replayedOK, err := replayTopologyGraphV1(
		graphResult.Manifest, sink.Source(), topologyDiagnosticReaderLimits(),
	)
	require.NoError(t, err)
	require.True(t, replayedOK)
	assert.Equal(t, livePayload, replayedPayload)

	replayed, err := replayTopologySemanticV1(result.Manifest, result.Members, topologyDiagnosticReaderLimits())
	require.NoError(t, err)
	require.NotNil(t, replayed)
	require.True(t, replayed.hasObservation)

	entry := store.Entries()[0]
	live := coll.deviceStates[entry.RegistrationID].generation
	require.NotNil(t, live)
	assert.Equal(t, live.observation, replayed.observation)
	_, err, state := live.diagnosticObservation.Resolve()
	require.NoError(t, err)
	assert.Equal(t, diagnostic.HandleSealed, state)

	for _, member := range result.Members {
		for _, canary := range [][]byte{
			[]byte(dev.Community), []byte(dev.V3User), []byte(dev.V3AuthKey), []byte(dev.V3PrivKey),
			[]byte(dev.V3ContextName), []byte("/stock/path"),
		} {
			assert.False(t, bytes.Contains(member, canary), "credential/path canary crossed the diagnostic boundary")
		}
	}
}

func diagnosticResultForCapability(
	t *testing.T,
	results []diagnostic.CaptureResult,
	capability diagnostic.CapabilityKey,
) diagnostic.CaptureResult {
	t.Helper()
	for _, result := range results {
		if len(result.Manifest.Roots) == 1 && result.Manifest.Roots[0].CapabilityKey == capability {
			return result
		}
	}
	t.Fatalf("diagnostic result for %s@%d is absent", capability.Name, capability.Revision)
	return diagnostic.CaptureResult{}
}

func TestDiagnosticSemanticReplay_PreservesVLANContextResults(t *testing.T) {
	profile := &ddsnmp.Profile{
		SourceFile: "profile-a.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{{Kind: ddsnmp.KindVtpVlan}},
		},
	}
	mainMetrics := []*ddsnmp.ProfileMetrics{{
		Source: profile.SourceFile,
		TopologyMetrics: []ddsnmp.Metric{{
			TopologyKind: ddsnmp.KindVtpVlan,
			Tags:         map[string]string{tagVtpVlanIndex: "100", tagVtpVlanName: "users", tagVtpVlanState: "operational"},
		}},
	}}
	vlanMetrics := []*ddsnmp.ProfileMetrics{{
		Source: profile.SourceFile,
		TopologyMetrics: []ddsnmp.Metric{
			{TopologyKind: ddsnmp.KindBridgePortIfIndex, Tags: map[string]string{tagBridgeBasePort: "5", tagBridgeIfIndex: "5"}},
			{TopologyKind: ddsnmp.KindFdbEntry, Tags: map[string]string{tagFdbMac: "005056abcdef", tagFdbBridgePort: "5", tagFdbStatus: "learned"}},
		},
	}}

	sink := &diagnostic.MemorySink{}
	recorder, err := diagnostic.NewRecorder(diagnostic.RecorderConfig{
		QueueCapacity: 1, MaxMembers: 64, MaxRetainedBytes: 16 << 20, Sink: sink,
	})
	require.NoError(t, err)
	coll := newTestSNMPTopologyCollector()
	coll.diagnosticRecorder = recorder
	capture := coll.beginTopologyDiagnosticCapture(1)
	require.NotNil(t, capture)

	builder := newTopologyBuilder()
	builder.updateTime = time.Date(2026, time.August, 27, 15, 0, 0, 0, time.UTC)
	builder.staleAfter = time.Hour
	builder.agentID = "192.0.2.10"
	builder.localDevice = buildLocalTopologyDevice(ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-a"})
	builder.targetManagementIPs = []netip.Addr{netip.MustParseAddr("192.0.2.10")}
	capture.setDevice(builder, builder.targetManagementIPs, 1234, []*ddsnmp.Profile{profile})

	applyTopologySemanticStream(builder, newTopologyMainSemanticStream(1234, mainMetrics), capture.observe)
	contexts := builder.vtpVLANContexts()
	require.Equal(t, []topologyVLANContext{{vlanID: "100", vlanName: "users"}}, contexts)
	vlanResults := []topologyVLANContextResult{{
		ordinal: 0, vlanID: "100", vlanName: "users", state: topologyVLANContextSuccess,
		profiles: vlanMetrics, profileDefinitions: []*ddsnmp.Profile{profile},
	}}
	applyTopologySemanticStream(builder, newTopologyVLANSemanticStream(vlanResults), capture.observe)
	snapshot, _ := freezeTopologyBuilder(builder)
	capture.commit(snapshot)
	recorder.Close()

	result := sink.Results()[0]
	require.NoError(t, result.Err)
	replayed, err := replayTopologySemanticV1(result.Manifest, result.Members, topologyDiagnosticReaderLimits())
	require.NoError(t, err)
	assert.Equal(t, snapshot.observation, replayed.observation)
	require.Len(t, replayed.observation.L2Observations, 1)
	require.Len(t, replayed.observation.L2Observations[0].FDBEntries, 1)
	assert.Equal(t, "100", replayed.observation.L2Observations[0].FDBEntries[0].VLANID)
	assert.Equal(t, "users", replayed.observation.L2Observations[0].FDBEntries[0].VLANName)
}

func topologyDiagnosticReaderLimits() diagnostic.ReaderLimits {
	return diagnostic.ReaderLimits{
		MaxStoredBytes: 64 << 20, MaxLogicalBytes: 64 << 20, MaxMemberBytes: 16 << 20,
		MaxMembers: 4096, MaxDevices: 1024, MaxProfiles: 4096, MaxRows: 1 << 20,
		MaxTags: 1 << 20, MaxStringBytes: 32 << 20, MaxDNSRecords: 1 << 16, MaxOUIRecords: 1 << 16,
		MaxReferenceEdges: 1 << 20, MaxNestingDepth: 64, MaxJSONTokens: 1 << 24, MaxReplayWork: 1 << 24,
	}
}
