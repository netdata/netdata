// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseValidatesAgainstSchemaAndSemanticChecks(t *testing.T) {
	payload := NewResponse(Data{
		Producer: Producer{
			Source:   "test-topology",
			Instance: "test-instance",
			Plugin:   "go-test",
		},
		CollectedAt: time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
		Dictionaries: Dictionaries{
			"strings": StringValues("node-a"),
		},
		Types: TypeRegistry{
			ActorTypes: map[string]ActorType{
				"node": {
					Layer:    "node",
					Identity: []string{"id"},
					Presentation: &ActorPresentation{
						Label:     "Node",
						Role:      "actor",
						Icon:      "server",
						ColorSlot: "primary",
						Annotation: &AnnotationPresentation{
							ColorSlot: "warning",
							Style:     "ring",
						},
						Border: &BorderPresentation{
							Enabled: Bool(true),
							Style:   "solid",
						},
						LabelPolicy: &LabelPolicy{
							Columns:   []string{"name"},
							Fallback:  "type_label",
							MaxLength: 80,
							Array:     "reject",
						},
						Ports: &ActorPortsPresentation{
							ShowBullets: true,
							Sources: []PortSourcePresentation{
								{
									Source:      "links",
									ActorColumn: "src",
									NameColumn:  "port_name",
									DefaultType: "topology",
								},
							},
						},
						Hover: &HoverPresentation{
							Fields: []PresentationField{{Key: "name", Label: "Name"}},
						},
					},
				},
			},
			LinkTypes: map[string]LinkType{
				"dependency": {
					Orientation:   "directed",
					DirectionRole: "dependency",
					Aggregation: LinkAggregation{
						Direction: "preserve",
					},
					Presentation: &LinkPresentation{
						Label:     "Dependency",
						ColorSlot: "primary",
						LineStyle: "solid",
						Width:     "normal",
						Curve:     "auto",
						Arrow:     "forward",
						Variable: &LinkVariablePresentation{
							Channel:     "width",
							ScaleKey:    "requests",
							ValueColumn: "weight",
							Min:         "normal",
							Max:         "emphasis",
						},
						Hover: &HoverPresentation{
							Fields: []PresentationField{{Key: "weight", Label: "Requests"}},
						},
					},
				},
			},
			PortTypes: map[string]PortType{
				"topology": {
					Presentation: &PortPresentation{
						Label:     "Topology",
						ColorSlot: "primary",
					},
				},
			},
			TableTypes: map[string]TableType{
				"port_detail": {
					Role:        "actor_detail",
					Owner:       "actor",
					Aggregation: "append",
					Columns: []Column{
						NewColumn("actor", "actor_ref"),
						NewColumn("neighbors", "json", WithNullable()),
					},
				},
				"path_detail": {
					Role:        "actor_detail",
					Owner:       "actor",
					Aggregation: "append",
					Columns: []Column{
						NewColumn("actor", "actor_ref"),
						NewColumn("path_actor", "actor_ref"),
						NewColumn("path_index", "uint"),
					},
				},
			},
		},
		Presentation: &Presentation{
			ProfileVersion: "test",
			Selection: &SelectionPresentation{
				ActorClick: &ActorClickPresentation{
					Mode:            "highlight_path",
					PathTable:       "path",
					PathOwnerColumn: "actor",
					PathActorColumn: "path_actor",
					PathOrderColumn: "path_index",
				},
			},
			Legend: &PresentationLegend{
				Actors: []LegendEntry{{Type: "node", Label: "Node"}},
				Links:  []LegendEntry{{Type: "dependency", Label: "Dependency"}},
				Ports:  []LegendEntry{{Type: "topology", Label: "Topology"}},
			},
			PortFields: []PresentationField{{Key: "type", Label: "Type"}},
			ScaleKeys: map[string]ScaleKeyPresentation{
				"requests": {Label: "Requests", Unit: "count"},
			},
		},
		Actors: MustTable(1,
			[]Column{
				NewColumn("id", "string", WithRole("identity")),
				NewColumn("type", "string"),
				NewColumn("name", "string_ref", WithDictionary("strings")),
			},
			[]ColumnEncoding{
				Values("node-a"),
				Const("node"),
				Values(0),
			},
		),
		Links: MustTable(1,
			[]Column{
				NewColumn("src", "actor_ref"),
				NewColumn("dst", "actor_ref"),
				NewColumn("type", "string"),
				NewColumn("port_name", "string"),
				NewColumn("weight", "uint", WithRole("metric"), WithAggregation("sum")),
			},
			[]ColumnEncoding{
				Values(0),
				Values(0),
				Const("dependency"),
				Const("node-a"),
				Values(7),
			},
		),
		Tables: &DetailTables{
			Actor: map[string]DetailTable{
				"ports": {
					Type: "port_detail",
					Table: MustTable(1,
						[]Column{
							NewColumn("actor", "actor_ref"),
							NewColumn("neighbors", "json", WithNullable()),
						},
						[]ColumnEncoding{
							Values(0),
							Values(map[string]any{
								"neighbors": []any{
									map[string]any{
										"protocol":    "lldp",
										"remote_port": "eth0",
									},
								},
							}),
						},
					),
				},
				"path": {
					Type: "path_detail",
					Table: MustTable(1,
						[]Column{
							NewColumn("actor", "actor_ref"),
							NewColumn("path_actor", "actor_ref"),
							NewColumn("path_index", "uint"),
						},
						[]ColumnEncoding{
							Values(0),
							Values(0),
							Values(0),
						},
					),
				},
			},
		},
	})

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.Contains(t, string(payloadBytes), `"schema_version":"netdata.topology.v1"`)

	validateAgainstTopologySchema(t, payloadBytes)

	var decoded any
	require.NoError(t, json.Unmarshal(payloadBytes, &decoded))
	require.NoError(t, ValidateDecodedResponse(decoded))
}

func TestValidateDecodedResponseRejectsInvalidPresentationReferences(t *testing.T) {
	payload := NewResponse(Data{
		Producer:     Producer{Source: "test-topology", Instance: "test-instance"},
		CollectedAt:  time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
		Dictionaries: Dictionaries{"strings": StringValues("node-a")},
		Types: TypeRegistry{
			ActorTypes: map[string]ActorType{
				"node": {
					Layer:    "node",
					Identity: []string{"id"},
					Presentation: &ActorPresentation{
						LabelPolicy: &LabelPolicy{Columns: []string{"missing"}},
					},
				},
			},
			LinkTypes: map[string]LinkType{
				"dependency": {
					Orientation:   "directed",
					DirectionRole: "dependency",
					Aggregation:   LinkAggregation{Direction: "preserve"},
				},
			},
		},
		Actors: MustTable(1,
			[]Column{
				NewColumn("id", "string", WithRole("identity")),
				NewColumn("type", "string"),
			},
			[]ColumnEncoding{
				Values("node-a"),
				Const("node"),
			},
		),
		Links: MustTable(1,
			[]Column{
				NewColumn("src", "actor_ref"),
				NewColumn("dst", "actor_ref"),
				NewColumn("type", "string"),
			},
			[]ColumnEncoding{
				Values(0),
				Values(0),
				Const("dependency"),
			},
		),
	})

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(payloadBytes, &decoded))

	err = ValidateDecodedResponse(decoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "label_policy.columns[0] references unknown actor column")
}

func TestValidateDecodedResponseRejectsNonDisplayLabelColumn(t *testing.T) {
	payload := NewResponse(Data{
		Producer:     Producer{Source: "test-topology", Instance: "test-instance"},
		CollectedAt:  time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
		Dictionaries: Dictionaries{"strings": StringValues("node-a")},
		Types: TypeRegistry{
			ActorTypes: map[string]ActorType{
				"node": {
					Layer:    "node",
					Identity: []string{"id"},
					Presentation: &ActorPresentation{
						LabelPolicy: &LabelPolicy{Columns: []string{"metadata"}},
					},
				},
			},
			LinkTypes: map[string]LinkType{
				"dependency": {
					Orientation:   "directed",
					DirectionRole: "dependency",
					Aggregation:   LinkAggregation{Direction: "preserve"},
				},
			},
		},
		Actors: MustTable(1,
			[]Column{
				NewColumn("id", "string", WithRole("identity")),
				NewColumn("type", "string"),
				NewColumn("metadata", "json", WithNullable()),
			},
			[]ColumnEncoding{
				Values("node-a"),
				Const("node"),
				Values(map[string]any{"labels": []any{"not-a-label"}}),
			},
		),
		Links: MustTable(1,
			[]Column{
				NewColumn("src", "actor_ref"),
				NewColumn("dst", "actor_ref"),
				NewColumn("type", "string"),
			},
			[]ColumnEncoding{
				Values(0),
				Values(0),
				Const("dependency"),
			},
		),
	})

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(payloadBytes, &decoded))

	err = ValidateDecodedResponse(decoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references non-display actor column")
}

func TestValidateDecodedResponseRejectsMissingPortSourceTable(t *testing.T) {
	payload := NewResponse(Data{
		Producer:     Producer{Source: "test-topology", Instance: "test-instance"},
		CollectedAt:  time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
		Dictionaries: Dictionaries{"strings": StringValues("node-a")},
		Types: TypeRegistry{
			ActorTypes: map[string]ActorType{
				"node": {
					Layer:    "node",
					Identity: []string{"id"},
					Presentation: &ActorPresentation{
						Ports: &ActorPortsPresentation{
							ShowBullets: true,
							Sources: []PortSourcePresentation{
								{
									Source:      "actor_table",
									Table:       "missing_ports",
									ActorColumn: "actor",
									NameColumn:  "name",
									DefaultType: "topology",
								},
							},
						},
					},
				},
			},
			LinkTypes: map[string]LinkType{
				"dependency": {
					Orientation:   "directed",
					DirectionRole: "dependency",
					Aggregation:   LinkAggregation{Direction: "preserve"},
				},
			},
			PortTypes: map[string]PortType{
				"topology": {Presentation: &PortPresentation{Label: "Topology"}},
			},
		},
		Actors: MustTable(1,
			[]Column{
				NewColumn("id", "string", WithRole("identity")),
				NewColumn("type", "string"),
			},
			[]ColumnEncoding{
				Values("node-a"),
				Const("node"),
			},
		),
		Links: MustTable(1,
			[]Column{
				NewColumn("src", "actor_ref"),
				NewColumn("dst", "actor_ref"),
				NewColumn("type", "string"),
			},
			[]ColumnEncoding{
				Values(0),
				Values(0),
				Const("dependency"),
			},
		),
	})

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(payloadBytes, &decoded))

	err = ValidateDecodedResponse(decoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references unknown actor table")
}

func TestPresentationTokenEnumsMatchSchema(t *testing.T) {
	schemaDoc := loadTopologySchema(t)

	assert.ElementsMatch(t, colorSlotTokens, schemaEnum(t, schemaDoc, "color_slot"))
	assert.ElementsMatch(t, opacityTokens, schemaEnum(t, schemaDoc, "opacity_token"))
	assert.ElementsMatch(t, widthTokens, schemaEnum(t, schemaDoc, "width_token"))
	assert.ElementsMatch(t, iconTokens, schemaEnum(t, schemaDoc, "icon_token"))
}

func TestValidateDecodedResponseRejectsInvalidActorReference(t *testing.T) {
	payload := map[string]any{
		"status": float64(200),
		"type":   "topology",
		"data": map[string]any{
			"schema_version": SchemaVersion,
			"dictionaries": map[string]any{
				"strings": []any{"node-a"},
			},
			"types": map[string]any{},
			"actors": map[string]any{
				"rows":    float64(1),
				"columns": []any{map[string]any{"id": "id", "type": "string"}},
				"values":  []any{map[string]any{"codec": "values", "values": []any{"node-a"}}},
			},
			"links": map[string]any{
				"rows": float64(1),
				"columns": []any{
					map[string]any{"id": "src", "type": "actor_ref"},
					map[string]any{"id": "dst", "type": "actor_ref"},
				},
				"values": []any{
					map[string]any{"codec": "values", "values": []any{float64(0)}},
					map[string]any{"codec": "values", "values": []any{float64(3)}},
				},
			},
		},
	}

	err := ValidateDecodedResponse(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "actor reference out of bounds")
}

func validateAgainstTopologySchema(t *testing.T, payload []byte) {
	t.Helper()

	schemaDoc := loadTopologySchema(t)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", schemaDoc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)

	var decoded any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.NoError(t, schema.Validate(decoded))
}

func loadTopologySchema(t *testing.T) map[string]any {
	t.Helper()

	schemaPath := filepath.Clean(filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	schemaBytes, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	var schemaDoc map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &schemaDoc))
	return schemaDoc
}

func schemaEnum(t *testing.T, schemaDoc map[string]any, defName string) []string {
	t.Helper()

	defs, ok := schemaDoc["$defs"].(map[string]any)
	require.True(t, ok)
	definition, ok := defs[defName].(map[string]any)
	require.True(t, ok)
	rawEnum, ok := definition["enum"].([]any)
	require.True(t, ok)

	values := make([]string, 0, len(rawEnum))
	for _, raw := range rawEnum {
		value, ok := raw.(string)
		require.True(t, ok)
		values = append(values, value)
	}
	return values
}
