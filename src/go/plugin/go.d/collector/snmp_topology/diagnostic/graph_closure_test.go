// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerationClosureV1_RejectsMissingObservationMember(t *testing.T) {
	observationRef, _, err := Seal(MemberType{Kind: KindObservation, Schema: SchemaV1}, ObservationV1{})
	require.NoError(t, err)
	generation := validTestGeneration()
	generation.DeviceCount = 1
	generation.RenderableDevices = 1
	generation.Observations = []ContentRef{observationRef}
	generationRef, generationData, err := Seal(MemberType{Kind: KindGeneration, Schema: SchemaV1}, generation)
	require.NoError(t, err)

	root := CapabilityRootV1{
		Capability: GenerationCapabilityV1(), State: StateSuccess,
		Sections: []SectionInventoryV1{{
			Name: GenerationSectionGeneration, State: StateSuccess, ExpectedRecords: 1,
			Members: []ContentRef{generationRef},
		}},
	}
	err = validateGenerationCapabilityGraphV1(root, MemorySource{generationRef.Key(): generationData}, testReaderLimits())
	require.ErrorContains(t, err, "member not found")
}

func TestGraphClosureV1_RejectsQueryGenerationMismatch(t *testing.T) {
	generation := validTestGeneration()
	generationRef, generationData, err := Seal(MemberType{Kind: KindGeneration, Schema: SchemaV1}, generation)
	require.NoError(t, err)
	query := GraphQueryV1{
		CaptureID: 1, GenerationSequence: generation.Sequence + 1, Generation: generationRef,
		Options: validTestGraphOptions(),
	}
	queryRef, queryData, err := Seal(MemberType{Kind: KindGraphQuery, Schema: SchemaV1}, query)
	require.NoError(t, err)

	root := CapabilityRootV1{
		Capability: GraphCapabilityV1(), State: StateEmpty,
		Sections: []SectionInventoryV1{
			{Name: GraphSectionDNSTrace, State: StateEmpty},
			{Name: GraphSectionGeneration, State: StateSuccess, ExpectedRecords: 1, Members: []ContentRef{generationRef}},
			{Name: GraphSectionOUITrace, State: StateEmpty},
			{Name: GraphSectionQuery, State: StateSuccess, ExpectedRecords: 1, Members: []ContentRef{queryRef}},
		},
	}
	source := MemorySource{generationRef.Key(): generationData, queryRef.Key(): queryData}
	require.ErrorContains(t, validateGraphCapabilityGraphV1(root, source, testReaderLimits()),
		"graph query does not identify the inventoried generation")
}

func TestGraphClosureV1_RejectsTraceBeyondReaderPolicy(t *testing.T) {
	trace := DNSTraceV1{CaptureID: 1, Records: []DNSRecordV1{
		{Ordinal: 0, IP: "192.0.2.1", State: DNSStateMiss},
		{Ordinal: 1, IP: "192.0.2.2", State: DNSStateCachedNegative},
	}}
	ref, data, err := Seal(MemberType{Kind: KindDNSTrace, Schema: SchemaV1}, trace)
	require.NoError(t, err)
	section := SectionInventoryV1{
		Name: GraphSectionDNSTrace, State: StateSuccess, ExpectedRecords: 2, Members: []ContentRef{ref},
	}
	limits := testReaderLimits()
	limits.MaxDNSRecords = 1
	_, err = validateDNSTraceSection(section, 1, MemorySource{ref.Key(): data}, limits)
	require.ErrorContains(t, err, "count exceeds limit 1")
}

func validTestGeneration() GenerationV1 {
	return GenerationV1{
		Sequence: 1, PublishedAt: "2026-08-27T12:00:00Z", ProducerScopeID: "agent-a",
		Kernel: GraphKernelV1{
			Name: "snmp_topology_graph", Revision: 1, ModelSchema: "2.0", OutputSchema: "netdata.topology.v1",
		},
		Observations: []ContentRef{},
	}
}

func validTestGraphOptions() GraphQueryOptionsV1 {
	return GraphQueryOptionsV1{
		MapType: "managed_fabric", InferenceStrategy: "fdb_minimum_knowledge",
		ManagedDeviceFocus: "all_devices", Depth: -1,
	}
}
