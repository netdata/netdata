// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSemanticGroupOrderV1_BindsExecutedProfileEvidence(t *testing.T) {
	profiles := []SemanticProfileV1{
		{Role: "main", Ordinal: 0},
		{Role: "vlan", VLANID: "100", Ordinal: 0},
	}
	groups := []semanticShardGroupV1{
		semanticTestGroup(SemanticPhaseProfileTags, 0, 0, SemanticRecordV1{Kind: "profile_tags", Profile: 0}),
		semanticTestGroup(SemanticPhaseBGPPeers, 0, 0, SemanticRecordV1{
			Kind: "bgp_outcome", Profile: 0, State: StateSuccess,
		}),
		semanticTestGroup(SemanticPhaseVLANOutcome, 0, 0, SemanticRecordV1{
			Kind: "vlan_outcome", VLANID: "100", VLANName: "users", State: StateSuccess,
		}),
		semanticTestGroup(SemanticPhaseVLANProfileTags, 0, 0, SemanticRecordV1{
			Kind: "profile_tags", Profile: 0, VLANID: "100",
		}),
	}
	require.NoError(t, validateSemanticGroupOrderV1(groups, profiles, 1))

	require.ErrorContains(t, validateSemanticGroupOrderV1(groups, profiles[:1], 1), "no matching ordered evidence")

	extra := append([]SemanticProfileV1(nil), profiles...)
	extra = append(extra, SemanticProfileV1{Role: "vlan", VLANID: "200", Ordinal: 0})
	require.ErrorContains(t, validateSemanticGroupOrderV1(groups, extra, 1), "not consumed")
}

func TestValidateSemanticGroupOrderV1_RejectsChangedExecutionOrder(t *testing.T) {
	groups := []semanticShardGroupV1{
		semanticTestGroup(SemanticPhaseBGPPeers, 0, 0, SemanticRecordV1{
			Kind: "bgp_outcome", Profile: 0, State: StateSuccess,
		}),
		semanticTestGroup(SemanticPhaseProfileTags, 0, 0, SemanticRecordV1{Kind: "profile_tags", Profile: 0}),
	}
	require.ErrorContains(t, validateSemanticGroupOrderV1(
		groups, []SemanticProfileV1{{Role: "main", Ordinal: 0}}, 1,
	), "missing its ordered profile_tags group")
}

func TestSemanticBGPPeerV1_RejectsValuesHiddenBehindAbsentOptionals(t *testing.T) {
	peer := SemanticBGPPeerV1{Kind: "peer"}
	require.NoError(t, peer.Validate())

	peer.State.Raw = "established"
	require.ErrorContains(t, peer.Validate(), "absent BGP state")
}

func TestSemanticBGPPeerV1_ValidatesStructuralOrigin(t *testing.T) {
	require.NoError(t, (SemanticBGPPeerV1{Kind: "peer", OriginProfileID: "vendor-a/_bgp.yaml"}).Validate())
	for _, origin := range []string{"/stock/vendor-a/_bgp.yaml", "../vendor-a/_bgp.yaml", `vendor-a\_bgp.yaml`} {
		require.Error(t, (SemanticBGPPeerV1{Kind: "peer", OriginProfileID: origin}).Validate())
	}
}

func TestSemanticRecordV1_RejectsUnreplayableVLANOutcomeStates(t *testing.T) {
	for _, state := range []TerminalState{StateEmpty, StateNotApplicable} {
		record := SemanticRecordV1{
			Kind: "vlan_outcome", VLANID: "100", State: state, Reason: "collection_failed",
		}
		require.ErrorContains(t, record.Validate(), "invalid vlan_outcome")
	}
}

func TestSemanticClosureV1_EnforcesDeviceStructuralLimits(t *testing.T) {
	device := SemanticDeviceV1{
		CaptureID: 1, Registration: 1, CollectedAt: "2026-08-27T12:00:00Z",
		FreshForNanoseconds: 1, AgentID: "agent-a",
		TargetManagementIPs: []string{"192.0.2.1", "192.0.2.2"},
	}
	deviceRef, deviceData, err := Seal(MemberType{Kind: KindSemanticDevice, Schema: SchemaV1}, device)
	require.NoError(t, err)
	root := CapabilityRootV1{
		Capability: SemanticCapabilityV1(), State: StateEmpty,
		Sections: []SectionInventoryV1{
			{Name: SemanticSectionDevice, State: StateSuccess, ExpectedRecords: 1, Members: []ContentRef{deviceRef}},
			{Name: SemanticSectionObservation, State: StateEmpty},
			{Name: SemanticSectionProfiles, State: StateEmpty},
			{Name: SemanticSectionEvents, State: StateEmpty},
		},
	}
	limits := testReaderLimits()
	limits.MaxRows = 1
	require.ErrorContains(t, validateSemanticGraphV1(
		root, MemorySource{deviceRef.Key(): deviceData}, limits,
	), "semantic device row count exceeds limit 1")
}

func semanticTestGroup(phase, context, profile uint32, records ...SemanticRecordV1) semanticShardGroupV1 {
	return semanticShardGroupV1{
		key:     semanticShardGroupKeyV1{phase: phase, context: context, profile: profile},
		records: records,
	}
}
