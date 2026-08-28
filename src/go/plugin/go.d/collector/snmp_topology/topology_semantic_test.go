// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"testing"
	"time"

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
					tagLldpLocPortNum:                      "7",
					tagLldpRemIndex:                        "1",
					tagLldpRemMgmtAddrSubtype:              "1",
					tagLldpRemMgmtAddrLen:                  "4",
					tagLldpRemMgmtAddrOctetPref + "1":      "192",
					tagLldpRemMgmtAddrOctetPref + "2":      "0",
					tagLldpRemMgmtAddrOctetPref + "3":      "2",
					tagLldpRemMgmtAddrOctetPref + "4":      "2",
					tagLldpRemMgmtAddrOctetPref + "unused": "must-not-appear-invalid-octet-tag",
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

	mainObserver.ObserveProfile(ddsnmpcollector.AcquisitionProfileReport{
		Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
		Outcome:  ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
	}, pms[0])
	recorder.completeContext(0, successfulAcquisitionPhase())
	recorder.setCollectedShape(collectedAt, freshFor, 1234)
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventSysUptime, sysUptime: 1234})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventProfileTags, profiles: pms})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventTopologyMetrics, profiles: pms})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: pms})
	vlanPMS := []*ddsnmp.ProfileMetrics{{
		Source: "/must/not/appear/vlan-profile.yaml",
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
	vlanObserver.ObserveProfile(ddsnmpcollector.AcquisitionProfileReport{
		Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
		Outcome:  ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
	}, vlanPMS[0])
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

	text := fmt.Sprintf("%+v", capture)
	require.NotContains(t, text, dev.Community)
	require.NotContains(t, text, dev.V3AuthKey)
	require.NotContains(t, text, dev.V3PrivKey)
	require.NotContains(t, text, dev.ManualProfiles[0])
	require.NotContains(t, text, pms[0].Source)
	require.NotContains(t, text, pms[0].TopologyMetrics[0].Name)
	require.NotContains(t, text, "must-not-appear-unused-metadata")
	require.NotContains(t, text, "must-not-appear-unused-profile-tag")
	require.NotContains(t, text, "must-not-appear-profile-tag-vendor")
	require.NotContains(t, text, "must-not-appear-unused-metric-tag")
	require.NotContains(t, text, "must-not-appear-unused-bgp-tag")
	require.NotContains(t, text, "must-not-appear-non-vlan-context-row")
	require.NotContains(t, text, "must-not-appear-invalid-octet-tag")
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.metadata, tagLldpLocChassisID)
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.tags, tagLldpLocSysName)
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.metrics[0].tags, tagTopoIfIndex)
	require.Contains(t, capture.evidence.collectionContexts[0].profiles[0].values.bgpRows[0].tags, "neighbor")
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

	observer.ObserveProfile(ddsnmpcollector.AcquisitionProfileReport{
		Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
		Outcome:  ddsnmpcollector.AcquisitionProfileOutcomePartial,
	}, pms[0])
	recorder.completeContext(0, successfulAcquisitionPhase())
	recorder.setCollectedShape(collectedAt, time.Minute, 0)
	capture := recorder.finish()
	require.Equal(t, diagnosticCaptureAvailable, capture.state)
	require.Len(t, capture.evidence.collectionContexts, 1)
	require.True(t, capture.evidence.collectionContexts[0].profiles[0].values.bgpFailed)
	require.Empty(t, capture.evidence.collectionContexts[0].profiles[0].values.bgpRows)
	require.NotContains(t, fmt.Sprintf("%+v", capture), pms[0].BGPRows[0].OriginProfileID)
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
		observer.ObserveProfile(ddsnmpcollector.AcquisitionProfileReport{
			Identity: ddsnmpcollector.AcquisitionProfileIdentity{Ordinal: 0},
			Outcome:  ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
		}, pms[0])
		capture := recorder.finish()
		require.Equal(t, diagnosticCaptureUnavailable, capture.state)
		require.Equal(t, diagnosticCaptureReasonProjectionError, capture.reason)
		require.Nil(t, capture.evidence)
		require.Len(t, builder.bgpPeersByKey, 1, "projection errors must not change live ingestion")
		require.NotContains(t, fmt.Sprintf("%+v", capture), pms[0].BGPRows[0].OriginProfileID)
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
		recorder.projectProfile = func(*ddsnmp.ProfileMetrics) topologyAcquisitionProfileValues {
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

func cloneTopologyAcquisitionEvidence(value *topologyAcquisitionAttemptEvidence) *topologyAcquisitionAttemptEvidence {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.collectionContexts = slices.Clone(value.collectionContexts)
	return &cloned
}
