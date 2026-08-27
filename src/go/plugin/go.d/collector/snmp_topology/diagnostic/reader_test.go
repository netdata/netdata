// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type changingMemberSource struct {
	mu          sync.Mutex
	first       MemorySource
	replacement MemorySource
	opens       map[string]uint64
}

func (s *changingMemberSource) Open(ref ContentRef) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opens == nil {
		s.opens = make(map[string]uint64)
	}
	key := ref.Key()
	s.opens[key]++
	data, ok := s.first[key]
	if s.opens[key] > 1 {
		if replacement, exists := s.replacement[key]; exists {
			data, ok = replacement, true
		}
	}
	if !ok {
		return nil, errors.New("member not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

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

func TestRegistryValidateCapability_VerifiesEveryMemberOpen(t *testing.T) {
	t.Parallel()

	capability := CapabilityKey{Name: "semantic_replay", Revision: 1}
	leafType := MemberType{Kind: "semantic_leaf", Schema: SchemaV1}
	leafRef, leafData, err := Seal(leafType, testLeaf{ID: "leaf-a"})
	require.NoError(t, err)
	_, changedLeafData, err := Seal(leafType, testLeaf{ID: "leaf-b"})
	require.NoError(t, err)
	require.Len(t, changedLeafData, len(leafData))
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
	source := &changingMemberSource{
		first:       MemorySource{rootRef.Key(): rootData, leafRef.Key(): leafData},
		replacement: MemorySource{leafRef.Key(): changedLeafData},
	}

	_, err = registry.ValidateCapability(manifest, source, capability, testReaderLimits())
	require.ErrorContains(t, err, "content digest mismatch")
}

func TestRegistryValidateCapability_NotApplicableIsNotReplayable(t *testing.T) {
	t.Parallel()

	capability := CapabilityKey{Name: "semantic_replay", Revision: 1}
	root := CapabilityRootV1{
		Capability: capability,
		State:      StateNotApplicable,
		Sections: []SectionInventoryV1{{
			Name:  "semantic_inputs",
			State: StateNotApplicable,
		}},
	}
	rootRef, rootData, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
	require.NoError(t, err)
	manifest := ManifestV1{
		Format:           FormatV1,
		Canonicalization: CanonicalJSONV1,
		Sensitivity:      ExactRestrictedSensitivity(),
		Roots:            []CapabilityRefV1{{CapabilityKey: capability, State: StateNotApplicable, Root: rootRef}},
		Members:          []ContentRef{rootRef},
	}
	registry := NewRegistry()
	require.NoError(t, registry.Register(capability, Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
		},
	}))

	report, err := registry.ValidateCapability(
		manifest,
		MemorySource{rootRef.Key(): rootData},
		capability,
		testReaderLimits(),
	)
	require.NoError(t, err)
	assert.True(t, report.Completeness)
	assert.False(t, report.Replayable)
}
