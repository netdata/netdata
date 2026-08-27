// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

//go:embed schema-v1.json
var schemaV1 []byte

func TestSchemaV1_CompilesAndValidatesKnownDocuments(t *testing.T) {
	t.Parallel()

	var schemaDocument any
	require.NoError(t, json.Unmarshal(schemaV1, &schemaDocument))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema-v1.json", schemaDocument))
	schema, err := compiler.Compile("schema-v1.json")
	require.NoError(t, err)

	capability := CapabilityKey{Name: "semantic_replay", Revision: 1}
	leafRef, _, err := Seal(MemberType{Kind: "semantic_leaf", Schema: "1"}, testLeaf{ID: "leaf-a"})
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
	rootRef, _, err := Seal(MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1}, root)
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
	evidence := ProfileDefinitionEvidenceV1{
		Encoding: "netdata.ddsnmp-topology-profile-evidence-yaml/v1", EffectiveDefinition: "topology: []\n",
	}
	evidenceDigest, err := ProfileDefinitionDigest(evidence)
	require.NoError(t, err)
	timestamp := "2026-08-27T12:00:00Z"
	observation := ObservationV1{
		CaptureID: 1, Registration: 9, LocalDeviceID: "device-a", AgentID: "agent-a",
		CollectedAt: timestamp,
	}
	observationRef, _, err := Seal(MemberType{Kind: KindObservation, Schema: SchemaV1}, observation)
	require.NoError(t, err)
	generation := GenerationV1{
		Sequence: 1, PublishedAt: timestamp, ProducerScopeID: "producer-a",
		Kernel: GraphKernelV1{
			Name: "snmp_topology_graph", Revision: 1, ModelSchema: "2.0", OutputSchema: "netdata.topology.v1",
		},
		DeviceCount: 1, RenderableDevices: 1, Observations: []ContentRef{observationRef},
	}
	generationRef, _, err := Seal(MemberType{Kind: KindGeneration, Schema: SchemaV1}, generation)
	require.NoError(t, err)

	documents := map[string]any{
		"manifest":        manifest,
		"capability root": root,
		"capture gap": CaptureGapV1{
			CapabilityClass: string(CaptureClassSemantic),
			FirstAttempt:    1, LastAttempt: 1, Count: 1, Reason: "recorder_saturated",
		},
		"semantic device": SemanticDeviceV1{
			CaptureID: 1, Registration: 9, CollectedAt: timestamp, FreshForNanoseconds: 1,
			AgentID: "agent-a", TargetManagementIPs: []string{"192.0.2.1"},
		},
		"semantic profile": SemanticProfileV1{
			CaptureID: 1, Registration: 9, Role: "main", Origin: "profile.yaml", Projection: "topology_bgp",
			Definition: evidence, DefinitionSHA256: evidenceDigest,
		},
		"semantic shard": SemanticShardV1{
			Geometry: ShardGeometryV1{
				CaptureID: 1, Registration: 9, Section: SemanticSectionEvents, Phase: SemanticPhaseProfileTags,
				ShardCount: 1, RecordCount: 1,
			},
			Records: []SemanticRecordV1{{Kind: "profile_tags"}},
		},
		"observation": observation,
		"observation checkpoint": ObservationCheckpointV1{
			CaptureID: 1, Registration: 9, Canonicalization: "netdata.snmp-topology-observation/v1",
			LogicalLength: 1, SHA256: strings.Repeat("a", 64),
		},
		"generation": generation,
		"graph query": GraphQueryV1{
			CaptureID: 2, GenerationSequence: 1, Generation: generationRef,
			Options: GraphQueryOptionsV1{
				CollapseActorsByIP: true, EliminateNonIPInferred: true, MapType: "managed_fabric",
				InferenceStrategy: "fdb_minimum_knowledge", ManagedDeviceFocus: "all_devices", Depth: -1,
			},
		},
		"DNS trace": DNSTraceV1{
			CaptureID: 2, Records: []DNSRecordV1{{IP: "192.0.2.1", State: DNSStateMiss}},
		},
		"OUI trace": OUITraceV1{
			CaptureID: 2, Records: []OUIRecordV1{{MAC: "00:50:56:ab:cd:ef", Vendor: "VMware, Inc.", Prefix: "005056"}},
		},
	}

	for name, value := range documents {
		t.Run(name, func(t *testing.T) {
			data, err := CanonicalBytes(value)
			require.NoError(t, err)
			var document any
			require.NoError(t, json.Unmarshal(data, &document))
			require.NoError(t, schema.Validate(document), fmt.Sprintf("schema rejected %s", data))
		})
	}
}
