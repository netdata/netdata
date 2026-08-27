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

func semanticTestGroup(phase, context, profile uint32, records ...SemanticRecordV1) semanticShardGroupV1 {
	return semanticShardGroupV1{
		key:     semanticShardGroupKeyV1{phase: phase, context: context, profile: profile},
		records: records,
	}
}
