// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"net/netip"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestTopologyAcquisitionReplayMatchesLiveBuilder(t *testing.T) {
	collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	freshFor := 32 * time.Minute
	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:       "192.0.2.1",
		Community:      "must-not-appear-in-acquisition-evidence",
		V3AuthKey:      "must-not-appear-in-acquisition-evidence",
		V3PrivKey:      "must-not-appear-in-acquisition-evidence",
		ManualProfiles: []string{"/must/not/appear/profile.yaml"},
		SysObjectID:    "1.3.6.1.4.1.9.1.1",
		SysName:        "switch-a",
		SysDescr:       "test switch",
		Vendor:         "Cisco",
		Model:          "C9300",
		VnodeGUID:      "vnode-guid",
		VnodeLabels:    map[string]string{"site": "lab"},
	}
	deviceInput := topologySemanticDeviceInputFromConnection(dev)
	targets := []netip.Addr{netip.MustParseAddr("192.0.2.1")}
	builder := newTopologyBuilderFromSemanticInput(deviceInput, targets, collectedAt, freshFor)
	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		deviceInput,
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionLiteral, addresses: targets},
		defaultTopologyAcquisitionLimits,
	)
	mainObserver := recorder.beginContext(0, "", "")

	pms := []*ddsnmp.ProfileMetrics{{
		Source: "/must/not/appear/profile.yaml",
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagLldpLocChassisID:        {Value: "001122334455", IsExactMatch: true},
			tagLldpLocChassisIDSubtype: {Value: "4", IsExactMatch: true},
			"unused_metadata":          {Value: "must-not-appear-unused-metadata"},
		},
		Tags: map[string]string{
			tagLldpLocSysName: "switch-a",
			"vendor":          "must-not-appear-profile-tag-vendor",
			"unused_profile":  "must-not-appear-unused-profile-tag",
		},
		TopologyMetrics: []ddsnmp.Metric{
			{
				Name:         "/must/not/appear/metric",
				TopologyKind: ddsnmp.KindIfName,
				Tags: map[string]string{
					tagTopoIfIndex:  "7",
					tagTopoIfName:   "Gi1/0/7",
					tagTopoIfOper:   "up",
					"unused_metric": "must-not-appear-unused-metric-tag",
				},
			},
			{
				TopologyKind: ddsnmp.KindLldpLocPort,
				Tags: map[string]string{
					tagLldpLocPortNum:       "7",
					tagLldpLocPortID:        "Gi1/0/7",
					tagLldpLocPortIDSubtype: "5",
				},
			},
			{
				TopologyKind: ddsnmp.KindLldpRemManAddr,
				Tags: map[string]string{
					tagLldpLocPortNum:         "7",
					tagLldpRemIndex:           "1",
					tagLldpRemMgmtAddrSubtype: "1",
					tagLldpRemMgmtAddr:        "c0000202",
					tagLldpRemMgmtAddrLen:     "4",
					"unused_lldp":             "must-not-appear-unused-lldp-tag",
				},
			},
		},
		BGPRows: []ddsnmp.BGPRow{{
			OriginProfileID: "_std-bgp4-mib.yaml",
			Table:           "bgpPeerTable",
			RowKey:          "192.0.2.2",
			StructuralID:    "peer-1",
			Kind:            ddprofiledefinition.BGPRowKindPeer,
			Identity: ddsnmp.BGPIdentity{
				RoutingInstance: "default",
				Neighbor:        "192.0.2.2",
				RemoteAS:        "65002",
			},
			Descriptors: ddsnmp.BGPDescriptors{
				LocalAddress: "192.0.2.1",
				LocalAS:      "65001",
			},
			Admin: ddsnmp.BGPAdmin{Enabled: ddsnmp.BGPBool{Has: true, Value: true}},
			State: ddsnmp.BGPState{Has: true, State: ddprofiledefinition.BGPPeerStateEstablished},
			Tags: map[string]string{
				"neighbor":         "192.0.2.2",
				"remote_as":        "65002",
				"routing_instance": "default",
				"unused_bgp":       "must-not-appear-unused-bgp-tag",
			},
		}},
	}}

	mainObserver.ObserveProfile(acquisitionReportForMetrics(
		0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, pms[0],
	), pms[0])
	recorder.completeContext(0, successfulAcquisitionPhase())
	recorder.setCollectedShape(collectedAt, freshFor, 1234)
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventSysUptime, sysUptime: 1234})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventProfileTags, profiles: pms})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventTopologyMetrics, profiles: pms})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: pms})
	vlanPMS := []*ddsnmp.ProfileMetrics{{
		Source: "/must/not/appear/vlan-profile.yaml",
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagOSPFRouterID: {Value: "192.0.2.254", IsExactMatch: true},
		},
		Tags: map[string]string{
			tagBridgeBaseAddress: "0011.2233.4455",
		},
		TopologyMetrics: []ddsnmp.Metric{
			{
				Name:         "/must/not/appear/vlan-metric",
				TopologyKind: ddsnmp.KindIfName,
				Tags: map[string]string{
					tagTopoIfIndex: "7",
					tagTopoIfName:  "Gi1/0/7",
				},
			},
			{
				TopologyKind: ddsnmp.KindIpIfIndex,
				Tags: map[string]string{
					tagTopoIPAddr: "must-not-appear-non-vlan-context-row",
				},
			},
		},
	}}
	vlanObserver := recorder.beginContext(1, "100", "users")
	vlanObserver.ObserveProfile(acquisitionReportForMetrics(
		0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, vlanPMS[0],
	), vlanPMS[0])
	recorder.completeContext(1, successfulAcquisitionPhase())
	applyTopologySemanticEvent(builder, topologySemanticEvent{
		kind:     topologySemanticEventVLANContext,
		vlanID:   "100",
		vlanName: "users",
		profiles: vlanPMS,
	})

	live, _ := freezeTopologyBuilder(builder)
	capture := recorder.finish()
	require.Equal(t, diagnosticCaptureAvailable, capture.state)
	require.NotNil(t, capture.evidence)
	require.NotZero(t, capture.recordCount)
	require.NotZero(t, capture.logicalBytes)

	dev.VnodeLabels["site"] = "changed"
	pms[0].DeviceMetadata[tagLldpLocChassisID] = ddsnmp.MetaTag{Value: "ffffffffffff"}
	pms[0].Tags[tagLldpLocSysName] = "changed"
	pms[0].TopologyMetrics[0].Tags[tagTopoIfName] = "changed"
	pms[0].BGPRows[0].Identity.Neighbor = "198.51.100.2"

	beforeReplay := cloneTopologyAcquisitionEvidence(capture.evidence)
	replayed, err := replayTopologyAcquisitionEvidence(capture.evidence)
	require.NoError(t, err)
	require.Equal(t, beforeReplay, capture.evidence)
	require.Equal(t, live.observation, replayed.observation)
	require.Equal(t, live.hasObservation, replayed.hasObservation)
	require.Equal(t, live.collectedAt, replayed.collectedAt)
	require.Equal(t, live.freshFor, replayed.freshFor)

	requireRetainedStringsExclude(t, capture.evidence.device, dev.Community, dev.V3AuthKey, dev.V3PrivKey,
		dev.ManualProfiles[0])
	requireRetainedStringsExclude(t, capture.evidence.collectionContexts,
		pms[0].Source,
		pms[0].TopologyMetrics[0].Name,
		"must-not-appear-unused-metadata",
		"must-not-appear-unused-profile-tag",
		"must-not-appear-profile-tag-vendor",
		"must-not-appear-unused-metric-tag",
		"must-not-appear-unused-bgp-tag",
		"must-not-appear-non-vlan-context-row",
		"must-not-appear-invalid-octet-tag",
	)
	requireRetainedStringsContain(t, capture.evidence.collectionContexts, "Gi1/0/7")
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.metadata, tagLldpLocChassisID)
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.tags, tagLldpLocSysName)
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.metrics[0].tags, tagTopoIfIndex)
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.metrics[2].tags, tagLldpRemMgmtAddrLen)
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.bgpRows[0].tags, "neighbor")
	require.Contains(t, capture.evidence.collectionContexts[1].profiles[0].values.metadata, tagOSPFRouterID)
	require.Contains(t, capture.evidence.collectionContexts[1].profiles[0].values.tags, tagBridgeBaseAddress)
}

func TestTopologyAcquisitionFailedProfileDoesNotRetainValues(t *testing.T) {
	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		topologySemanticDeviceInput{hostname: "192.0.2.1"},
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
		defaultTopologyAcquisitionLimits,
	)
	observer := recorder.beginContext(0, "", "")
	metrics := &ddsnmp.ProfileMetrics{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			tagOSPFRouterID: {Value: "192.0.2.254", IsExactMatch: true},
		},
		Tags: map[string]string{
			tagBridgeBaseAddress: "0011.2233.4455",
		},
		TopologyMetrics: []ddsnmp.Metric{{
			TopologyKind: ddsnmp.KindIfName,
			Tags: map[string]string{
				tagTopoIfIndex: "7",
				tagTopoIfName:  "Gi1/0/7",
			},
		}},
		BGPRows: []ddsnmp.BGPRow{{
			OriginProfileID: "_std-bgp4-mib.yaml",
			StructuralID:    "peer-1",
			Kind:            ddprofiledefinition.BGPRowKindPeer,
			Identity:        ddsnmp.BGPIdentity{Neighbor: "192.0.2.2", RemoteAS: "65002"},
		}},
	}

	observer.ObserveProfile(acquisitionReportForMetrics(
		0, ddsnmpcollector.AcquisitionProfileOutcomeFailed, metrics,
	), metrics)
	capture := recorder.finish()

	require.Equal(t, diagnosticCaptureAvailable, capture.state)
	require.Len(t, capture.evidence.collectionContexts, 1)
	require.Len(t, capture.evidence.collectionContexts[0].profiles, 1)
	profile := capture.evidence.collectionContexts[0].profiles[0]
	require.Equal(t, ddsnmpcollector.AcquisitionProfileOutcomeFailed, profile.outcome)
	require.NotEmpty(t, profile.routes)
	require.Empty(t, profile.values)
}

func TestTopologyAcquisitionRetainedStringsOwnTheirBacking(t *testing.T) {
	backing := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 1<<15)
	value := func(offset int) string { return backing[offset : offset+8] }
	input := topologySemanticDeviceInput{
		hostname:    value(0),
		sysObjectID: value(16),
		sysName:     value(32),
		sysDescr:    value(48),
		sysContact:  value(64),
		sysLocation: value(80),
		vendor:      value(96),
		model:       value(112),
		vnodeGUID:   value(128),
		vnodeLabels: map[string]string{value(144): value(160)},
	}
	pms := &ddsnmp.ProfileMetrics{
		DeviceMetadata: map[string]ddsnmp.MetaTag{
			"vendor": {Value: value(176)},
		},
		Tags: map[string]string{
			tagLldpLocSysName: value(192),
		},
		TopologyMetrics: []ddsnmp.Metric{{
			TopologyKind: ddsnmp.KindIfName,
			Tags: map[string]string{
				tagTopoIfIndex: value(208),
				tagTopoIfName:  value(224),
			},
		}},
		BGPRows: []ddsnmp.BGPRow{{
			OriginProfileID: value(240),
			Table:           value(256),
			RowKey:          value(272),
			StructuralID:    value(288),
			Kind:            ddprofiledefinition.BGPRowKindPeer,
			Identity: ddsnmp.BGPIdentity{
				RoutingInstance: value(304),
				Neighbor:        value(320),
				RemoteAS:        value(336),
			},
			Descriptors: ddsnmp.BGPDescriptors{
				LocalAddress:    value(352),
				LocalAS:         value(368),
				LocalIdentifier: value(384),
				PeerIdentifier:  value(400),
				PeerType:        value(416),
				BGPVersion:      value(432),
				Description:     value(448),
			},
			State: ddsnmp.BGPState{
				Has:   true,
				State: ddprofiledefinition.BGPPeerState(value(464)),
				Raw:   value(480),
			},
			Tags: map[string]string{"neighbor": value(496)},
		}},
	}
	report := acquisitionReportForMetrics(
		0,
		ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
		pms,
	)
	report.Routes[0].RootOID = value(512)

	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		input,
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
		defaultTopologyAcquisitionLimits,
	)
	observer := recorder.beginContext(0, value(528), value(544))
	observer.ObserveProfile(report, pms)
	recorder.completeContext(0, successfulAcquisitionPhase())
	capture := recorder.finish()
	require.Equal(t, diagnosticCaptureAvailable, capture.state)
	require.NotNil(t, capture.evidence)

	context := capture.evidence.collectionContexts[0]
	profile := context.profiles[0]
	requireStringOutsideBacking(t, backing, capture.evidence.device.hostname)
	requireStringOutsideBacking(t, backing, capture.evidence.device.sysName)
	for key, retained := range capture.evidence.device.vnodeLabels {
		requireStringOutsideBacking(t, backing, key)
		requireStringOutsideBacking(t, backing, retained)
	}
	requireStringOutsideBacking(t, backing, context.vlanID)
	requireStringOutsideBacking(t, backing, context.vlanName)
	requireStringOutsideBacking(t, backing, profile.routes[0].RootOID)
	requireStringOutsideBacking(t, backing, profile.values.metadata["vendor"].Value)
	requireStringOutsideBacking(t, backing, profile.values.tags[tagLldpLocSysName])
	requireStringOutsideBacking(t, backing, profile.values.metrics[0].tags[tagTopoIfIndex])
	requireStringOutsideBacking(t, backing, profile.values.metrics[0].tags[tagTopoIfName])
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].originProfileID)
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].table)
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].rowKey)
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].structuralID)
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].neighbor)
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].remoteAS)
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].description)
	requireStringOutsideBacking(t, backing, string(profile.values.bgpRows[0].state))
	requireStringOutsideBacking(t, backing, profile.values.bgpRows[0].tags["neighbor"])
	requireRetainedStringsOutsideBacking(t, backing, capture.evidence.device)
	requireRetainedStringsOutsideBacking(t, backing, context)
	runtime.KeepAlive(backing)
}

func requireRetainedStringsOutsideBacking(t *testing.T, backing string, retained any) {
	t.Helper()
	var visit func(reflect.Value)
	visit = func(value reflect.Value) {
		if !value.IsValid() {
			return
		}
		switch value.Kind() {
		case reflect.String:
			if value.Len() > 0 {
				requireStringOutsideBacking(t, backing, value.String())
			}
		case reflect.Pointer, reflect.Interface:
			if !value.IsNil() {
				visit(value.Elem())
			}
		case reflect.Struct:
			for _, field := range value.Fields() {
				visit(field)
			}
		case reflect.Slice, reflect.Array:
			for i := range value.Len() {
				visit(value.Index(i))
			}
		case reflect.Map:
			iter := value.MapRange()
			for iter.Next() {
				visit(iter.Key())
				visit(iter.Value())
			}
		}
	}
	visit(reflect.ValueOf(retained))
}

func requireRetainedStringsExclude(t *testing.T, retained any, excluded ...string) {
	t.Helper()
	visitRetainedStrings(reflect.ValueOf(retained), func(value string) {
		for _, forbidden := range excluded {
			if forbidden != "" {
				require.NotContains(t, value, forbidden)
			}
		}
	})
}

func requireRetainedStringsContain(t *testing.T, retained any, expected string) {
	t.Helper()
	var found bool
	visitRetainedStrings(reflect.ValueOf(retained), func(value string) {
		found = found || value == expected
	})
	require.True(t, found, "retained values do not contain %q", expected)
}

func visitRetainedStrings(value reflect.Value, visit func(string)) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.String:
		visit(value.String())
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			visitRetainedStrings(value.Elem(), visit)
		}
	case reflect.Struct:
		for _, field := range value.Fields() {
			visitRetainedStrings(field, visit)
		}
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			visitRetainedStrings(value.Index(i), visit)
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			visitRetainedStrings(iter.Key(), visit)
			visitRetainedStrings(iter.Value(), visit)
		}
	}
}

func requireStringOutsideBacking(t *testing.T, backing, retained string) {
	t.Helper()
	require.NotEmpty(t, retained)
	start := uintptr(unsafe.Pointer(unsafe.StringData(backing)))
	end := start + uintptr(len(backing))
	pointer := uintptr(unsafe.Pointer(unsafe.StringData(retained)))
	require.False(t, pointer >= start && pointer < end, "retained string aliases the source backing allocation")
}

func TestTopologyAcquisitionCaptureIgnoresRejectedBGPRows(t *testing.T) {
	collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	input := topologySemanticDeviceInput{hostname: "192.0.2.1"}
	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		input,
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
		defaultTopologyAcquisitionLimits,
	)
	observer := recorder.beginContext(0, "", "")
	pms := []*ddsnmp.ProfileMetrics{{
		BGPCollectError: errors.New("collection failed"),
		BGPRows: []ddsnmp.BGPRow{{
			OriginProfileID: "/ignored/private/profile.yaml",
			Kind:            ddprofiledefinition.BGPRowKindPeer,
		}},
	}}

	observer.ObserveProfile(acquisitionReportForMetrics(
		0, ddsnmpcollector.AcquisitionProfileOutcomePartial, pms[0],
	), pms[0])
	recorder.completeContext(0, successfulAcquisitionPhase())
	recorder.setCollectedShape(collectedAt, time.Minute, 0)
	capture := recorder.finish()
	require.Equal(t, diagnosticCaptureAvailable, capture.state)
	require.Len(t, capture.evidence.collectionContexts, 1)
	require.True(t, capture.evidence.collectionContexts[0].profiles[0].values.bgpFailed)
	require.Empty(t, capture.evidence.collectionContexts[0].profiles[0].values.bgpRows)
	requireRetainedStringsExclude(t, capture.evidence.collectionContexts, pms[0].BGPRows[0].OriginProfileID)
}

func TestTopologyAcquisitionReplayRejectsMissingOrReorderedContexts(t *testing.T) {
	evidence := completeTestTopologyAcquisitionEvidence(t)

	missing := cloneTopologyAcquisitionEvidence(evidence)
	missing.collectionContexts = nil
	_, err := replayTopologyAcquisitionEvidence(missing)
	require.ErrorContains(t, err, "main acquisition context")

	reordered := cloneTopologyAcquisitionEvidence(evidence)
	reordered.collectionContexts = append(reordered.collectionContexts, topologyAcquisitionContextEvidence{ordinal: 0})
	_, err = replayTopologyAcquisitionEvidence(reordered)
	require.ErrorContains(t, err, "acquisition context order")
}

func TestTopologyAcquisitionReplayIgnoresFailedVLANContextValues(t *testing.T) {
	evidence := completeTestTopologyAcquisitionEvidence(t)
	want, err := replayTopologyAcquisitionEvidence(evidence)
	require.NoError(t, err)

	failed := cloneTopologyAcquisitionEvidence(evidence)
	failed.collectionContexts = append(failed.collectionContexts, topologyAcquisitionContextEvidence{
		ordinal:    1,
		vlanID:     "100",
		vlanName:   "users",
		client:     successfulAcquisitionPhase(),
		connect:    successfulAcquisitionPhase(),
		collection: failedAcquisitionPhase(topologyAcquisitionFailureCollection),
		profiles: []topologyAcquisitionProfileEvidence{{
			identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
			outcome:  ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
			values: topologyAcquisitionProfileValues{metrics: []topologyAcquisitionMetricValue{{
				kind: ddsnmp.KindFdbEntry,
				tags: map[string]string{
					tagFdbMac:        "00:11:22:33:44:55",
					tagFdbBridgePort: "7",
					tagFdbStatus:     "learned",
				},
			}}},
		}},
	})

	got, err := replayTopologyAcquisitionEvidence(failed)
	require.NoError(t, err)
	require.Equal(t, want.observation, got.observation)
}

func TestTopologyAcquisitionCaptureLimitFailsOpen(t *testing.T) {
	collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	input := topologySemanticDeviceInput{hostname: "192.0.2.1"}
	builder := newTopologyBuilderFromSemanticInput(input, nil, collectedAt, time.Minute)
	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		input,
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
		topologyAcquisitionLimits{
			maxRecords:      1,
			maxLogicalBytes: 1024,
		})

	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventSysUptime, sysUptime: 1234})
	recorder.beginContext(0, "", "")
	capture := recorder.finish()
	require.Equal(t, diagnosticCaptureLimitExceeded, capture.state)
	require.Equal(t, diagnosticCaptureReasonRecordLimit, capture.reason)
	require.Nil(t, capture.evidence)
	require.EqualValues(t, 1234, builder.localDevice.SysUptime, "capture limits must not change live ingestion")
}

func TestTopologyAcquisitionVnodeLabelsCountAsRecords(t *testing.T) {
	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		topologySemanticDeviceInput{
			hostname:    "192.0.2.1",
			vnodeLabels: map[string]string{"site": "lab"},
		},
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
		topologyAcquisitionLimits{maxRecords: 1, maxLogicalBytes: 1024},
	)

	capture := recorder.finish()
	require.Equal(t, diagnosticCaptureLimitExceeded, capture.state)
	require.Equal(t, diagnosticCaptureReasonRecordLimit, capture.reason)
	require.Nil(t, capture.evidence)
}

func TestTopologyAcquisitionCaptureErrorAndPanicFailOpen(t *testing.T) {
	collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	input := topologySemanticDeviceInput{hostname: "192.0.2.1"}

	t.Run("projection error", func(t *testing.T) {
		builder := newTopologyBuilderFromSemanticInput(input, nil, collectedAt, time.Minute)
		recorder := newTopologyAcquisitionRecorder(
			topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
			input,
			topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
			defaultTopologyAcquisitionLimits,
		)
		observer := recorder.beginContext(0, "", "")
		pms := []*ddsnmp.ProfileMetrics{{BGPRows: []ddsnmp.BGPRow{{
			OriginProfileID: "/private/profile.yaml",
			StructuralID:    "peer-1",
			Kind:            ddprofiledefinition.BGPRowKindPeer,
			Identity:        ddsnmp.BGPIdentity{Neighbor: "192.0.2.2", RemoteAS: "65002"},
		}}}}

		applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: pms})
		observer.ObserveProfile(acquisitionReportForMetrics(
			0, ddsnmpcollector.AcquisitionProfileOutcomeSuccess, pms[0],
		), pms[0])
		capture := recorder.finish()
		require.Equal(t, diagnosticCaptureUnavailable, capture.state)
		require.Equal(t, diagnosticCaptureReasonProjectionError, capture.reason)
		require.Nil(t, capture.evidence)
		require.Len(t, builder.bgpPeersByKey, 1, "projection errors must not change live ingestion")
	})

	t.Run("projection panic", func(t *testing.T) {
		builder := newTopologyBuilderFromSemanticInput(input, nil, collectedAt, time.Minute)
		recorder := newTopologyAcquisitionRecorder(
			topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
			input,
			topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
			defaultTopologyAcquisitionLimits,
		)
		observer := recorder.beginContext(0, "", "")
		recorder.projectProfile = func(
			topologySemanticEventKind,
			ddsnmpcollector.AcquisitionProfileReport,
			*ddsnmp.ProfileMetrics,
		) topologyAcquisitionProfileValues {
			panic("projection panic")
		}

		require.NotPanics(t, func() {
			applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventSysUptime, sysUptime: 1234})
			observer.ObserveProfile(ddsnmpcollector.AcquisitionProfileReport{
				Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
				Outcome:  ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
			}, &ddsnmp.ProfileMetrics{})
		})
		capture := recorder.finish()
		require.Equal(t, diagnosticCaptureUnavailable, capture.state)
		require.Equal(t, diagnosticCaptureReasonProjectionPanic, capture.reason)
		require.EqualValues(t, 1234, builder.localDevice.SysUptime)
	})
}

func completeTestTopologyAcquisitionEvidence(t *testing.T) *topologyAcquisitionAttemptEvidence {
	t.Helper()
	collectedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	input := topologySemanticDeviceInput{hostname: "192.0.2.1"}
	recorder := newTopologyAcquisitionRecorder(
		topologyAcquisitionAttemptID{registrationID: 1, ordinal: 1},
		input,
		topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
		defaultTopologyAcquisitionLimits,
	)
	observer := recorder.beginContext(0, "", "")
	observer.ObserveProfile(ddsnmpcollector.AcquisitionProfileReport{
		Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
		Outcome:  ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
	}, &ddsnmp.ProfileMetrics{})
	recorder.completeContext(0, successfulAcquisitionPhase())
	recorder.setCollectedShape(collectedAt, time.Minute, 1)
	capture := recorder.finish()
	require.Equal(t, diagnosticCaptureAvailable, capture.state)
	return capture.evidence
}

func acquisitionReportForMetrics(
	ordinal uint32,
	outcome ddsnmpcollector.AcquisitionProfileOutcome,
	metrics *ddsnmp.ProfileMetrics,
) ddsnmpcollector.AcquisitionProfileReport {
	report := ddsnmpcollector.AcquisitionProfileReport{
		Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: ordinal},
		Outcome:  outcome,
	}
	if metrics == nil {
		return report
	}
	if len(metrics.TopologyMetrics) > 0 {
		routeOrdinal := uint32(len(report.Routes))
		report.Routes = append(report.Routes, ddsnmpcollector.AcquisitionRouteReport{
			Ordinal: routeOrdinal,
			Kind:    ddsnmpcollector.AcquisitionRouteKindTopologyTable,
			Outcome: ddsnmpcollector.AcquisitionRouteOutcomeValues,
			Rows:    uint64(len(metrics.TopologyMetrics)),
			Values:  uint64(len(metrics.TopologyMetrics)),
		})
		for rowOrdinal := range metrics.TopologyMetrics {
			report.TopologyValueReferences = append(report.TopologyValueReferences,
				ddsnmpcollector.AcquisitionValueReference{
					RouteOrdinal: routeOrdinal,
					RowOrdinal:   uint32(rowOrdinal),
				})
		}
	}
	if len(metrics.BGPRows) > 0 {
		routeOrdinal := uint32(len(report.Routes))
		report.Routes = append(report.Routes, ddsnmpcollector.AcquisitionRouteReport{
			Ordinal: routeOrdinal,
			Kind:    ddsnmpcollector.AcquisitionRouteKindBGPTable,
			Outcome: ddsnmpcollector.AcquisitionRouteOutcomeValues,
			Rows:    uint64(len(metrics.BGPRows)),
			Values:  uint64(len(metrics.BGPRows)),
		})
		for rowOrdinal := range metrics.BGPRows {
			report.BGPValueReferences = append(report.BGPValueReferences,
				ddsnmpcollector.AcquisitionValueReference{
					RouteOrdinal: routeOrdinal,
					RowOrdinal:   uint32(rowOrdinal),
				})
		}
	}
	return report
}

func cloneTopologyAcquisitionEvidence(value *topologyAcquisitionAttemptEvidence) *topologyAcquisitionAttemptEvidence {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.collectionContexts = slices.Clone(value.collectionContexts)
	return &cloned
}
