// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryValidateCapability_SkipsUnknownUnrelatedMember(t *testing.T) {
	t.Parallel()

	capability := CapabilityKey{Name: "semantic_replay", Revision: 1}
	leafType := MemberType{Kind: "semantic_leaf", Schema: "1"}
	leafRef, leafData, err := Seal(leafType, testLeaf{ID: "leaf-a", Tags: map[string]string{"role": "switch"}})
	require.NoError(t, err)
	root := CapabilityRootV1{
		Capability: capability,
		State:      StateSuccess,
		Sections: []SectionInventoryV1{{
			Name:            "semantic_inputs",
			State:           StateSuccess,
			ExpectedRecords: 1,
			Members:         []ContentRef{leafRef},
		}},
	}
	rootRef, rootData, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
	require.NoError(t, err)
	unknownRef, unknownData, err := Seal(MemberType{Kind: "future_member", Schema: "7"}, testLeaf{ID: "future-a"})
	require.NoError(t, err)

	members := []ContentRef{rootRef, leafRef, unknownRef}
	SortContentRefs(members)
	manifest := ManifestV1{
		Format:           FormatV1,
		Canonicalization: CanonicalJSONV1,
		Sensitivity:      ExactRestrictedSensitivity(),
		Authenticity:     AuthenticityV1{State: TrustUnauthenticated},
		Roots: []CapabilityRefV1{{
			CapabilityKey: capability,
			State:         StateSuccess,
			Root:          rootRef,
		}},
		Members: members,
	}
	source := MemorySource{
		leafRef.Key():    leafData,
		rootRef.Key():    rootData,
		unknownRef.Key(): unknownData,
	}

	registry := NewRegistry()
	require.NoError(t, registry.Register(capability, Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
			leafType: DecodeLeaf[testLeaf](),
		},
	}))

	report, err := registry.ValidateCapability(manifest, source, capability, testReaderLimits())
	require.NoError(t, err)
	assert.True(t, report.Integrity)
	assert.True(t, report.Schema)
	assert.True(t, report.Completeness)
	assert.True(t, report.Replayable)
	assert.False(t, report.Preserved)
	assert.Equal(t, TrustUnauthenticated, report.Authenticity)
	assert.Equal(t, uint64(3), report.Members)
}

func TestRegistryValidateCapability_RejectsUnknownMemberInRequestedClosure(t *testing.T) {
	t.Parallel()

	capability := CapabilityKey{Name: "semantic_replay", Revision: 1}
	unknownRef, unknownData, err := Seal(MemberType{Kind: "future_member", Schema: "7"}, testLeaf{ID: "future-a"})
	require.NoError(t, err)
	root := CapabilityRootV1{
		Capability: capability,
		State:      StateSuccess,
		Sections: []SectionInventoryV1{{
			Name:            "semantic_inputs",
			State:           StateSuccess,
			ExpectedRecords: 1,
			Members:         []ContentRef{unknownRef},
		}},
	}
	rootRef, rootData, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
	require.NoError(t, err)
	members := []ContentRef{rootRef, unknownRef}
	SortContentRefs(members)
	manifest := ManifestV1{
		Format:           FormatV1,
		Canonicalization: CanonicalJSONV1,
		Sensitivity:      ExactRestrictedSensitivity(),
		Authenticity:     AuthenticityV1{State: TrustUnauthenticated},
		Roots:            []CapabilityRefV1{{CapabilityKey: capability, State: StateSuccess, Root: rootRef}},
		Members:          members,
	}
	registry := NewRegistry()
	require.NoError(t, registry.Register(capability, Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
		},
	}))

	_, err = registry.ValidateCapability(manifest, MemorySource{
		rootRef.Key():    rootData,
		unknownRef.Key(): unknownData,
	}, capability, testReaderLimits())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported member future_member@7 inside requested capability")
}

func TestRegistryValidateCapability_RejectsMissingAndCorruptMembers(t *testing.T) {
	t.Parallel()

	capability := CapabilityKey{Name: "semantic_replay", Revision: 1}
	leafType := MemberType{Kind: "semantic_leaf", Schema: "1"}
	leafRef, leafData, err := Seal(leafType, testLeaf{ID: "leaf-a"})
	require.NoError(t, err)
	root := CapabilityRootV1{
		Capability: capability,
		State:      StateSuccess,
		Sections: []SectionInventoryV1{{
			Name:            "semantic_inputs",
			State:           StateSuccess,
			ExpectedRecords: 1,
			Members:         []ContentRef{leafRef},
		}},
	}
	rootRef, rootData, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
	require.NoError(t, err)
	members := []ContentRef{rootRef, leafRef}
	SortContentRefs(members)
	manifest := ManifestV1{
		Format:           FormatV1,
		Canonicalization: CanonicalJSONV1,
		Sensitivity:      ExactRestrictedSensitivity(),
		Authenticity:     AuthenticityV1{State: TrustUnauthenticated},
		Roots:            []CapabilityRefV1{{CapabilityKey: capability, State: StateSuccess, Root: rootRef}},
		Members:          members,
	}
	registry := NewRegistry()
	require.NoError(t, registry.Register(capability, Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
			leafType: DecodeLeaf[testLeaf](),
		},
	}))

	tests := map[string]MemorySource{
		"missing": {rootRef.Key(): rootData},
		"corrupt": {rootRef.Key(): rootData, leafRef.Key(): append([]byte(nil), leafData[:len(leafData)-1]...)},
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := registry.ValidateCapability(manifest, source, capability, testReaderLimits())
			require.Error(t, err)
		})
	}
}

func TestRegistryValidateCapability_RejectsCapabilityStateMismatch(t *testing.T) {
	t.Parallel()

	capability := CapabilityKey{Name: "semantic_replay", Revision: 1}
	leafType := MemberType{Kind: "semantic_leaf", Schema: "1"}
	leafRef, leafData, err := Seal(leafType, testLeaf{ID: "leaf-a"})
	require.NoError(t, err)
	root := CapabilityRootV1{
		Capability: capability,
		State:      StateSuccess,
		Sections: []SectionInventoryV1{{
			Name:            "semantic_inputs",
			State:           StateSuccess,
			ExpectedRecords: 1,
			Members:         []ContentRef{leafRef},
		}},
	}
	rootRef, rootData, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
	require.NoError(t, err)
	members := []ContentRef{rootRef, leafRef}
	SortContentRefs(members)
	manifest := ManifestV1{
		Format:           FormatV1,
		Canonicalization: CanonicalJSONV1,
		Sensitivity:      ExactRestrictedSensitivity(),
		Authenticity:     AuthenticityV1{State: TrustUnauthenticated},
		Roots:            []CapabilityRefV1{{CapabilityKey: capability, State: StateIncomplete, Root: rootRef}},
		Members:          members,
	}
	registry := NewRegistry()
	require.NoError(t, registry.Register(capability, Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
			leafType: DecodeLeaf[testLeaf](),
		},
	}))

	_, err = registry.ValidateCapability(manifest, MemorySource{
		rootRef.Key(): rootData,
		leafRef.Key(): leafData,
	}, capability, testReaderLimits())
	require.ErrorContains(t, err, "capability state mismatch: manifest=incomplete root=success")
}
