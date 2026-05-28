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

var testCollectedAt = time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)

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
							Enabled: new(true),
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
						Modal: &ModalPresentation{
							Enabled: new(true),
							Labels: &ModalLabelsPresentation{
								Table:       "actor_labels",
								ActorColumn: "actor",
								KeyColumn:   "key",
								ValueColumn: "value",
							},
							MiniTopology: &ModalMiniTopologyPresentation{
								Enabled: new(true),
								Depth:   1,
							},
							Sections: []ModalSection{
								{
									ID:    "ports",
									Label: "Ports",
									Source: ModalSource{
										Kind:  "actor_table",
										Table: "ports",
									},
									OwnerFilter: &ModalOwnerFilter{
										Mode:        "actor_column",
										ActorColumn: "actor",
									},
									Columns: []ModalColumn{
										{
											ID:    "neighbors",
											Label: "Neighbors",
											Projection: ModalProjection{
												Kind:   "direct",
												Column: "neighbors",
											},
											Cell:       "debug_json",
											Visibility: "debug",
										},
									},
								},
								{
									ID:    "paths",
									Label: "Paths",
									Source: ModalSource{
										Kind:  "actor_table",
										Table: "path",
									},
									OwnerFilter: &ModalOwnerFilter{
										Mode:        "actor_column",
										ActorColumn: "actor",
									},
									Columns: []ModalColumn{
										{
											ID:    "path_actor",
											Label: "Path actor",
											Projection: ModalProjection{
												Kind:        "actor_ref_label",
												ActorColumn: "path_actor",
											},
											Cell: "actor_link",
										},
										{
											ID:    "path_index",
											Label: "Hop",
											Projection: ModalProjection{
												Kind:   "direct",
												Column: "path_index",
											},
											Cell: "number",
										},
										{
											ID:    "source",
											Label: "Source",
											Projection: ModalProjection{
												Kind:  "const",
												Value: "test",
											},
											Cell:       "text",
											Visibility: "expanded",
										},
									},
									Sort: &ModalSort{Column: "path_index", Direction: "asc"},
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
						Layout: &LinkLayoutPresentation{
							Strength: "weaker",
							Distance: "farther",
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
				"actor_labels": {
					Role:        "actor_inventory",
					Owner:       "actor",
					Aggregation: "set",
					Columns: []Column{
						NewColumn("actor", "actor_ref"),
						NewColumn("key", "string"),
						NewColumn("value", "string"),
						NewColumn("source", "string", WithNullable()),
						NewColumn("kind", "string", WithNullable()),
						NewColumn("value_index", "uint", WithNullable()),
					},
				},
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
		Correlation: &Correlation{
			Rules: map[string]CorrelationRule{
				"node_name": {
					Action:          "link",
					Priority:        10,
					KeySpace:        "node_name",
					Key:             []CorrelationKeyPart{{Column: "name"}},
					PointActorTypes: []string{"node"},
					ClaimActorTypes: []string{"node"},
					OutputLinkType:  "dependency",
				},
			},
			Points: &Table{
				Rows: 1,
				Columns: []Column{
					NewColumn("actor", "actor_ref"),
					NewColumn("rule", "string"),
					NewColumn("name", "string_ref", WithDictionary("strings")),
				},
				Values: []ColumnEncoding{
					Values(0),
					Const("node_name"),
					Values(0),
				},
			},
			Claims: &Table{
				Rows: 1,
				Columns: []Column{
					NewColumn("actor", "actor_ref"),
					NewColumn("rule", "string"),
					NewColumn("name", "string_ref", WithDictionary("strings")),
				},
				Values: []ColumnEncoding{
					Values(0),
					Const("node_name"),
					Values(0),
				},
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
				"actor_labels": {
					Type: "actor_labels",
					Table: MustTable(1,
						[]Column{
							NewColumn("actor", "actor_ref"),
							NewColumn("key", "string"),
							NewColumn("value", "string"),
							NewColumn("source", "string", WithNullable()),
							NewColumn("kind", "string", WithNullable()),
							NewColumn("value_index", "uint", WithNullable()),
						},
						[]ColumnEncoding{
							Values(0),
							Const("hostname"),
							Values("node-a"),
							Const("producer"),
							Const("identity"),
							Values(nil),
						},
					),
				},
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
	err := validateResponseData(t, minimalValidationData(&ActorPresentation{
		LabelPolicy: &LabelPolicy{Columns: []string{"missing"}},
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "label_policy.columns[0] references unknown actor column")
}

func TestValidateDecodedResponseRejectsNonDisplayLabelColumn(t *testing.T) {
	actors := minimalActorTable(
		[]Column{NewColumn("metadata", "json", WithNullable())},
		[]ColumnEncoding{Values(map[string]any{"labels": []any{"not-a-label"}})},
	)
	err := validateResponseData(t, minimalValidationData(
		&ActorPresentation{LabelPolicy: &LabelPolicy{Columns: []string{"metadata"}}},
		withActors(actors),
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "references non-display actor column")
}

func TestValidateDecodedResponseRejectsMissingPortSourceTable(t *testing.T) {
	err := validateResponseData(t, minimalValidationData(
		&ActorPresentation{
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
		withPortTypes(map[string]PortType{
			"topology": {Presentation: &PortPresentation{Label: "Topology"}},
		}),
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "references unknown actor table")
}

func TestValidateDecodedResponseRejectsMissingModalActorTable(t *testing.T) {
	err := validateResponseData(t, minimalValidationData(
		&ActorPresentation{
			Modal: &ModalPresentation{
				Sections: []ModalSection{
					{
						ID:    "missing",
						Label: "Missing",
						Source: ModalSource{
							Kind:  "actor_table",
							Table: "missing_table",
						},
						Columns: []ModalColumn{
							{
								ID:    "name",
								Label: "Name",
								Projection: ModalProjection{
									Kind:   "direct",
									Column: "name",
								},
							},
						},
					},
				},
			},
		},
		withActors(minimalActorTable(
			[]Column{NewColumn("name", "string_ref", WithDictionary("strings"))},
			[]ColumnEncoding{Values(0)},
		)),
		withLinks(dependencyLinkTable(0)),
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "modal.sections[0].source.table references unknown actor table")
}

func TestValidateDecodedResponseRejectsInvalidModalProjectionShapes(t *testing.T) {
	cases := map[string]struct {
		projection ModalProjection
		want       string
	}{
		"direct missing column": {
			projection: ModalProjection{Kind: "direct"},
			want:       "column is required",
		},
		"actor label missing actor column": {
			projection: ModalProjection{Kind: "actor_ref_label"},
			want:       "actor_column is required",
		},
		"opposite actor missing actor columns": {
			projection: ModalProjection{Kind: "opposite_actor"},
			want:       "src_actor_column is required",
		},
		"const missing value": {
			projection: ModalProjection{Kind: "const"},
			want:       "value is required when kind is const",
		},
		"selected endpoint missing local side": {
			projection: ModalProjection{Kind: "selected_side_endpoint", SrcActorColumn: "src", DstActorColumn: "dst", RemoteIPColumn: "remote_ip"},
			want:       "requires local_ip_column or local_port_column",
		},
		"selected endpoint missing remote side": {
			projection: ModalProjection{Kind: "selected_side_endpoint", SrcActorColumn: "src", DstActorColumn: "dst", LocalIPColumn: "local_ip"},
			want:       "requires remote_ip_column or remote_port_column",
		},
		"selected endpoint empty local side": {
			projection: ModalProjection{Kind: "selected_side_endpoint", SrcActorColumn: "src", DstActorColumn: "dst", LocalIPColumn: "", RemoteIPColumn: "remote_ip"},
			want:       "requires local_ip_column or local_port_column",
		},
		"selected endpoint missing side actor columns": {
			projection: ModalProjection{Kind: "selected_side_endpoint", LocalIPColumn: "local_ip", RemoteIPColumn: "remote_ip"},
			want:       "src_actor_column is required",
		},
		"label lookup missing key": {
			projection: ModalProjection{Kind: "label_lookup", ActorColumn: "src"},
			want:       "label_key is required when kind is label_lookup",
		},
		"json path missing path": {
			projection: ModalProjection{Kind: "json_path", Column: "metadata"},
			want:       "path is required when kind is json_path",
		},
		"json path missing column": {
			projection: ModalProjection{Kind: "json_path", Path: "$.state"},
			want:       "column is required",
		},
		"coalesce missing columns": {
			projection: ModalProjection{Kind: "coalesce"},
			want:       "columns is required when kind is coalesce",
		},
		"formatted endpoint missing endpoint columns": {
			projection: ModalProjection{Kind: "formatted_endpoint", ProtocolColumn: "type"},
			want:       "requires ip_column or port_column",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			data := minimalValidationData(
				&ActorPresentation{
					Modal: &ModalPresentation{
						Sections: []ModalSection{
							{
								ID:     "links",
								Label:  "Links",
								Source: ModalSource{Kind: "links"},
								Columns: []ModalColumn{
									{
										ID:         "endpoint",
										Label:      "Endpoint",
										Projection: tc.projection,
									},
								},
							},
						},
					},
				},
				withLinks(dependencyLinkTableWith(1,
					[]Column{
						NewColumn("local_ip", "string", WithNullable()),
						NewColumn("remote_ip", "string", WithNullable()),
						NewColumn("metadata", "json", WithNullable()),
					},
					[]ColumnEncoding{
						Values("10.0.0.1"),
						Values("10.0.0.2"),
						Values(map[string]any{"state": "open"}),
					},
				)),
			)

			payloadBytes, err := json.Marshal(NewResponse(data))
			require.NoError(t, err)
			require.Error(t, topologySchemaValidationError(t, payloadBytes))

			var decoded any
			require.NoError(t, json.Unmarshal(payloadBytes, &decoded))

			err = ValidateDecodedResponse(decoded)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateDecodedResponseRejectsInvalidModalPresentationSemantics(t *testing.T) {
	basePayload := func(actorPresentation map[string]any) map[string]any {
		return map[string]any{
			"status": float64(200),
			"type":   "topology",
			"data": map[string]any{
				"schema_version": SchemaVersion,
				"dictionaries":   map[string]any{},
				"types": map[string]any{
					"actor_types": map[string]any{
						"node": map[string]any{
							"layer":        "node",
							"identity":     []any{"id"},
							"presentation": actorPresentation,
						},
					},
					"link_types": map[string]any{
						"dependency": map[string]any{
							"orientation":    "directed",
							"direction_role": "dependency",
							"aggregation":    map[string]any{"direction": "preserve"},
						},
					},
					"port_types":  map[string]any{},
					"table_types": map[string]any{},
				},
				"actors": map[string]any{
					"rows": float64(1),
					"columns": []any{
						map[string]any{"id": "id", "type": "string", "role": "identity"},
						map[string]any{"id": "type", "type": "string"},
					},
					"values": []any{
						map[string]any{"codec": "const", "value": "node-a"},
						map[string]any{"codec": "const", "value": "node"},
					},
				},
				"links": map[string]any{
					"rows": float64(1),
					"columns": []any{
						map[string]any{"id": "src", "type": "actor_ref"},
						map[string]any{"id": "dst", "type": "actor_ref"},
						map[string]any{"id": "type", "type": "string"},
					},
					"values": []any{
						map[string]any{"codec": "const", "value": float64(0)},
						map[string]any{"codec": "const", "value": float64(0)},
						map[string]any{"codec": "const", "value": "dependency"},
					},
				},
			},
		}
	}
	validSection := func() map[string]any {
		return map[string]any{
			"id":     "links",
			"label":  "Links",
			"source": map[string]any{"kind": "links"},
			"columns": []any{
				map[string]any{
					"id":         "type",
					"label":      "Type",
					"projection": map[string]any{"kind": "direct", "column": "type"},
				},
			},
		}
	}

	cases := map[string]struct {
		presentation map[string]any
		want         string
	}{
		"empty actor type label": {
			presentation: map[string]any{"label": ""},
			want:         "presentation.label is required",
		},
		"mini topology non-integer depth": {
			presentation: map[string]any{
				"modal": map[string]any{
					"mini_topology": map[string]any{"depth": "1"},
				},
			},
			want: "mini_topology.depth is not an integer",
		},
		"modal is not an object": {
			presentation: map[string]any{
				"modal": "invalid",
			},
			want: "modal is not an object",
		},
		"missing modal columns": {
			presentation: map[string]any{
				"modal": map[string]any{
					"sections": []any{
						map[string]any{
							"id":     "links",
							"label":  "Links",
							"source": map[string]any{"kind": "links"},
						},
					},
				},
			},
			want: "columns is not an array",
		},
		"empty modal columns": {
			presentation: map[string]any{
				"modal": map[string]any{
					"sections": []any{
						map[string]any{
							"id":      "links",
							"label":   "Links",
							"source":  map[string]any{"kind": "links"},
							"columns": []any{},
						},
					},
				},
			},
			want: "columns must not be empty",
		},
		"duplicate modal section id": {
			presentation: map[string]any{
				"modal": map[string]any{
					"sections": []any{
						validSection(),
						validSection(),
					},
				},
			},
			want: "duplicates modal section id",
		},
		"duplicate modal column id": {
			presentation: map[string]any{
				"modal": map[string]any{
					"sections": []any{
						func() map[string]any {
							section := validSection()
							section["columns"] = []any{
								map[string]any{
									"id":         "type",
									"label":      "Type",
									"projection": map[string]any{"kind": "direct", "column": "type"},
								},
								map[string]any{
									"id":         "type",
									"label":      "Type again",
									"projection": map[string]any{"kind": "direct", "column": "type"},
								},
							}
							return section
						}(),
					},
				},
			},
			want: "duplicates modal column id",
		},
		"sort references unknown modal column": {
			presentation: map[string]any{
				"modal": map[string]any{
					"sections": []any{
						func() map[string]any {
							section := validSection()
							section["sort"] = map[string]any{"column": "missing"}
							return section
						}(),
					},
				},
			},
			want: "sort.column references unknown modal column",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateDecodedResponse(basePayload(tc.presentation))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateDecodedResponseRejectsInvalidZeroHeuristicContract(t *testing.T) {
	basePayload := func() map[string]any {
		return map[string]any{
			"status": float64(200),
			"type":   "topology",
			"data": map[string]any{
				"schema_version": SchemaVersion,
				"dictionaries":   map[string]any{},
				"types": map[string]any{
					"actor_types": map[string]any{
						"node": map[string]any{
							"layer":    "node",
							"identity": []any{"id"},
							"search": map[string]any{
								"enabled": true,
								"columns": []any{"name"},
							},
							"presentation": map[string]any{
								"label":  "Node",
								"size":   map[string]any{"mode": "fixed", "scale": "normal"},
								"layout": map[string]any{"repulsion": "normal"},
							},
						},
					},
					"link_types": map[string]any{
						"dependency": map[string]any{
							"orientation":    "directed",
							"direction_role": "dependency",
							"semantic_role":  "traffic",
							"aggregation":    map[string]any{"direction": "preserve"},
						},
					},
				},
				"actors": map[string]any{
					"rows": float64(1),
					"columns": []any{
						map[string]any{"id": "id", "type": "string", "role": "identity"},
						map[string]any{"id": "type", "type": "string"},
						map[string]any{"id": "name", "type": "string"},
					},
					"values": []any{
						map[string]any{"codec": "const", "value": "node-a"},
						map[string]any{"codec": "const", "value": "node"},
						map[string]any{"codec": "const", "value": "Node A"},
					},
				},
				"links": map[string]any{
					"rows": float64(1),
					"columns": []any{
						map[string]any{"id": "src", "type": "actor_ref"},
						map[string]any{"id": "dst", "type": "actor_ref"},
						map[string]any{"id": "type", "type": "string"},
					},
					"values": []any{
						map[string]any{"codec": "const", "value": float64(0)},
						map[string]any{"codec": "const", "value": float64(0)},
						map[string]any{"codec": "const", "value": "dependency"},
					},
				},
			},
		}
	}

	cases := map[string]struct {
		mutate func(map[string]any)
		want   string
	}{
		"invalid search column": {
			mutate: func(payload map[string]any) {
				actorType := payload["data"].(map[string]any)["types"].(map[string]any)["actor_types"].(map[string]any)["node"].(map[string]any)
				actorType["search"].(map[string]any)["columns"] = []any{"missing"}
			},
			want: "search.columns[0] references unknown actor column",
		},
		"invalid size scale": {
			mutate: func(payload map[string]any) {
				actorType := payload["data"].(map[string]any)["types"].(map[string]any)["actor_types"].(map[string]any)["node"].(map[string]any)
				presentation := actorType["presentation"].(map[string]any)
				presentation["size"].(map[string]any)["scale"] = "huge"
			},
			want: "presentation.size.scale has unsupported value",
		},
		"invalid layout repulsion": {
			mutate: func(payload map[string]any) {
				actorType := payload["data"].(map[string]any)["types"].(map[string]any)["actor_types"].(map[string]any)["node"].(map[string]any)
				presentation := actorType["presentation"].(map[string]any)
				presentation["layout"].(map[string]any)["repulsion"] = "huge"
			},
			want: "presentation.layout.repulsion has unsupported value",
		},
		"invalid semantic role": {
			mutate: func(payload map[string]any) {
				linkType := payload["data"].(map[string]any)["types"].(map[string]any)["link_types"].(map[string]any)["dependency"].(map[string]any)
				linkType["semantic_role"] = "protocol"
			},
			want: "semantic_role has unsupported value",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			payload := basePayload()
			tc.mutate(payload)
			err := ValidateDecodedResponse(payload)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateDecodedResponseRejectsEmptyPresentationLabels(t *testing.T) {
	basePayload := func(types map[string]any) map[string]any {
		return map[string]any{
			"status": float64(200),
			"type":   "topology",
			"data": map[string]any{
				"schema_version": SchemaVersion,
				"dictionaries":   map[string]any{},
				"types":          types,
				"actors": map[string]any{
					"rows": float64(1),
					"columns": []any{
						map[string]any{"id": "id", "type": "string", "role": "identity"},
						map[string]any{"id": "type", "type": "string"},
					},
					"values": []any{
						map[string]any{"codec": "const", "value": "node-a"},
						map[string]any{"codec": "const", "value": "node"},
					},
				},
				"links": map[string]any{
					"rows": float64(0),
					"columns": []any{
						map[string]any{"id": "type", "type": "string"},
					},
					"values": []any{
						map[string]any{"codec": "const", "value": "dependency"},
					},
				},
			},
		}
	}
	defaultTypes := func() map[string]any {
		return map[string]any{
			"actor_types": map[string]any{
				"node": map[string]any{"layer": "node", "identity": []any{"id"}},
			},
			"link_types": map[string]any{
				"dependency": map[string]any{
					"orientation":    "directed",
					"direction_role": "dependency",
					"aggregation":    map[string]any{"direction": "preserve"},
				},
			},
			"port_types": map[string]any{},
			"table_types": map[string]any{
				"actor_labels": map[string]any{
					"role":        "actor_inventory",
					"owner":       "actor",
					"aggregation": "set",
					"columns": []any{
						map[string]any{"id": "actor", "type": "actor_ref"},
					},
				},
			},
		}
	}

	cases := map[string]func(map[string]any){
		"actor type label": func(types map[string]any) {
			types["actor_types"].(map[string]any)["node"].(map[string]any)["presentation"] = map[string]any{"label": ""}
		},
		"link type label": func(types map[string]any) {
			types["link_types"].(map[string]any)["dependency"].(map[string]any)["presentation"] = map[string]any{"label": ""}
		},
		"port type label": func(types map[string]any) {
			types["port_types"].(map[string]any)["port"] = map[string]any{"presentation": map[string]any{"label": ""}}
		},
		"table type label": func(types map[string]any) {
			types["table_types"].(map[string]any)["actor_labels"].(map[string]any)["presentation"] = map[string]any{"label": ""}
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			types := defaultTypes()
			mutate(types)

			err := ValidateDecodedResponse(basePayload(types))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "label is required")
		})
	}
}

func TestValidateDecodedResponseRejectsMalformedResponseEnvelope(t *testing.T) {
	cases := map[string]struct {
		payload any
		want    string
	}{
		"not object": {
			payload: []any{},
			want:    "response is not an object",
		},
		"missing data": {
			payload: map[string]any{"status": float64(200), "type": "topology"},
			want:    "response.data is not an object",
		},
		"wrong schema": {
			payload: map[string]any{
				"status": float64(200),
				"type":   "topology",
				"data":   map[string]any{"schema_version": "old"},
			},
			want: "response.data.schema_version is not",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateDecodedResponse(tc.payload)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateDecodedResponseRejectsInvalidCompactTableColumns(t *testing.T) {
	basePayload := func(columns []any, values []any) map[string]any {
		return map[string]any{
			"status": float64(200),
			"type":   "topology",
			"data": map[string]any{
				"schema_version": SchemaVersion,
				"dictionaries":   map[string]any{},
				"types": map[string]any{
					"actor_types": map[string]any{
						"node": map[string]any{"layer": "node", "identity": []any{"id"}},
					},
					"link_types": map[string]any{
						"dependency": map[string]any{
							"orientation":    "directed",
							"direction_role": "dependency",
							"aggregation":    map[string]any{"direction": "preserve"},
						},
					},
					"port_types":  map[string]any{},
					"table_types": map[string]any{},
				},
				"actors": map[string]any{
					"rows": float64(1),
					"columns": []any{
						map[string]any{"id": "id", "type": "string", "role": "identity"},
						map[string]any{"id": "type", "type": "string"},
					},
					"values": []any{
						map[string]any{"codec": "const", "value": "node-a"},
						map[string]any{"codec": "const", "value": "node"},
					},
				},
				"links": map[string]any{
					"rows":    float64(1),
					"columns": columns,
					"values":  values,
				},
			},
		}
	}

	cases := map[string]struct {
		columns []any
		values  []any
		want    string
	}{
		"empty id": {
			columns: []any{
				map[string]any{"id": "", "type": "string"},
			},
			values: []any{
				map[string]any{"codec": "const", "value": "dependency"},
			},
			want: "columns[0].id is required",
		},
		"duplicate id": {
			columns: []any{
				map[string]any{"id": "type", "type": "string"},
				map[string]any{"id": "type", "type": "string"},
			},
			values: []any{
				map[string]any{"codec": "const", "value": "dependency"},
				map[string]any{"codec": "const", "value": "dependency"},
			},
			want: "duplicates column",
		},
		"wrong uint value type": {
			columns: []any{
				map[string]any{"id": "type", "type": "string"},
				map[string]any{"id": "socket_count", "type": "uint"},
			},
			values: []any{
				map[string]any{"codec": "const", "value": "dependency"},
				map[string]any{"codec": "const", "value": "not-a-number"},
			},
			want: "is not a non-negative integer",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateDecodedResponse(basePayload(tc.columns, tc.values))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateDecodedResponseValidatesExplicitModalLabelColumns(t *testing.T) {
	baseData := func(labels *ModalLabelsPresentation) Data {
		return Data{
			Producer:     Producer{Source: "test-topology", Instance: "test-instance"},
			CollectedAt:  time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC),
			Dictionaries: Dictionaries{"strings": StringValues("node-a")},
			Types: TypeRegistry{
				ActorTypes: map[string]ActorType{
					"node": {
						Layer:    "node",
						Identity: []string{"id"},
						Presentation: &ActorPresentation{
							Modal: &ModalPresentation{
								Labels: labels,
							},
						},
					},
				},
				LinkTypes: map[string]LinkType{},
				PortTypes: map[string]PortType{},
				TableTypes: map[string]TableType{
					"actor_labels": {
						Role:        "actor_inventory",
						Owner:       "actor",
						Aggregation: "set",
						Columns: []Column{
							NewColumn("actor", "actor_ref"),
							NewColumn("key", "string"),
							NewColumn("value", "string"),
						},
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
			Links: MustTable(0,
				[]Column{NewColumn("type", "string")},
				[]ColumnEncoding{Const("none")},
			),
		}
	}

	t.Run("omitted optional columns are allowed", func(t *testing.T) {
		payload := NewResponse(baseData(&ModalLabelsPresentation{Table: "actor_labels"}))
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, topologySchemaValidationError(t, payloadBytes))

		var decoded any
		require.NoError(t, json.Unmarshal(payloadBytes, &decoded))
		require.NoError(t, ValidateDecodedResponse(decoded))
	})

	t.Run("identification fields are accepted", func(t *testing.T) {
		payload := NewResponse(baseData(&ModalLabelsPresentation{
			Table: "actor_labels",
			Identification: &ModalLabelIdentificationPresentation{
				Fields: []ModalLabelIdentificationField{
					{Key: "display_name", Label: "Name", MaxValues: 1},
					{Key: "role", Label: "Role", MaxValues: 2},
				},
			},
		}))
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, topologySchemaValidationError(t, payloadBytes))

		var decoded any
		require.NoError(t, json.Unmarshal(payloadBytes, &decoded))
		require.NoError(t, ValidateDecodedResponse(decoded))
	})

	t.Run("invalid identification max values is rejected", func(t *testing.T) {
		payload := NewResponse(baseData(&ModalLabelsPresentation{Table: "actor_labels"}))
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err)

		var decoded any
		require.NoError(t, json.Unmarshal(payloadBytes, &decoded))
		data := decoded.(map[string]any)["data"].(map[string]any)
		actorType := data["types"].(map[string]any)["actor_types"].(map[string]any)["node"].(map[string]any)
		modal := actorType["presentation"].(map[string]any)["modal"].(map[string]any)
		labels := modal["labels"].(map[string]any)
		labels["identification"] = map[string]any{
			"fields": []any{
				map[string]any{"key": "display_name", "label": "Name", "max_values": float64(0)},
			},
		}

		err = ValidateDecodedResponse(decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "labels.identification.fields[0].max_values must be a positive integer")
	})

	t.Run("explicit missing optional column is rejected", func(t *testing.T) {
		payload := NewResponse(baseData(&ModalLabelsPresentation{Table: "actor_labels", SourceColumn: "missing"}))
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, topologySchemaValidationError(t, payloadBytes))

		var decoded any
		require.NoError(t, json.Unmarshal(payloadBytes, &decoded))

		err = ValidateDecodedResponse(decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "labels.source_column references unknown column \"missing\"")
	})
}

func TestValidateDecodedResponseRejectsInvalidModalSectionsShape(t *testing.T) {
	payload := map[string]any{
		"status": float64(200),
		"type":   "topology",
		"data": map[string]any{
			"schema_version": SchemaVersion,
			"dictionaries":   map[string]any{},
			"types": map[string]any{
				"actor_types": map[string]any{
					"node": map[string]any{
						"layer":    "node",
						"identity": []any{"id"},
						"presentation": map[string]any{
							"modal": map[string]any{
								"sections": map[string]any{"not": "an-array"},
							},
						},
					},
				},
				"link_types":  map[string]any{},
				"port_types":  map[string]any{},
				"table_types": map[string]any{},
			},
			"actors": map[string]any{
				"rows": float64(1),
				"columns": []any{
					map[string]any{"id": "id", "type": "string", "role": "identity"},
					map[string]any{"id": "type", "type": "string"},
				},
				"values": []any{
					map[string]any{"codec": "values", "values": []any{"node-a"}},
					map[string]any{"codec": "const", "value": "node"},
				},
			},
			"links": map[string]any{
				"rows": float64(0),
				"columns": []any{
					map[string]any{"id": "type", "type": "string"},
				},
				"values": []any{
					map[string]any{"codec": "const", "value": "none"},
				},
			},
		},
	}

	err := ValidateDecodedResponse(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modal.sections is not an array")
}

func TestValidateDecodedResponseRejectsInvalidModalRowFilters(t *testing.T) {
	cases := map[string]struct {
		filter ModalRowFilter
		want   string
	}{
		"eq missing value": {
			filter: ModalRowFilter{Column: "type", Op: "eq"},
			want:   "value is required when op is eq",
		},
		"in missing values": {
			filter: ModalRowFilter{Column: "type", Op: "in"},
			want:   "values is required when op is in",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
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
								Modal: &ModalPresentation{
									Sections: []ModalSection{
										{
											ID:         "links",
											Label:      "Links",
											Source:     ModalSource{Kind: "links"},
											RowFilters: []ModalRowFilter{tc.filter},
											Columns: []ModalColumn{
												{
													ID:    "type",
													Label: "Type",
													Projection: ModalProjection{
														Kind:   "direct",
														Column: "type",
													},
												},
											},
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
						Const(0),
						Const(0),
						Const("dependency"),
					},
				),
			})

			payloadBytes, err := json.Marshal(payload)
			require.NoError(t, err)
			require.Error(t, topologySchemaValidationError(t, payloadBytes))

			var decoded any
			require.NoError(t, json.Unmarshal(payloadBytes, &decoded))

			err = ValidateDecodedResponse(decoded)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateDecodedResponseRejectsCorrelationMissingKeyColumn(t *testing.T) {
	correlation := &Correlation{
		Rules: map[string]CorrelationRule{
			"node_name": {
				Action:          "link",
				Priority:        10,
				KeySpace:        "node_name",
				Key:             []CorrelationKeyPart{{Column: "name"}},
				PointActorTypes: []string{"node"},
				OutputLinkType:  "dependency",
			},
			"node_owner": {
				Action:          "link",
				Priority:        20,
				KeySpace:        "node_owner",
				Key:             []CorrelationKeyPart{{Column: "owner"}},
				PointActorTypes: []string{"node"},
				OutputLinkType:  "dependency",
			},
		},
		Points: &Table{
			Rows: 1,
			Columns: []Column{
				NewColumn("actor", "actor_ref"),
				NewColumn("rule", "string"),
			},
			Values: []ColumnEncoding{
				Values(0),
				Const("node_name"),
			},
		},
	}
	err := validateResponseData(t, minimalValidationData(nil,
		withCorrelation(correlation),
		withLinks(dependencyLinkTable(0)),
	))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing correlation key column")
}

func TestValidateDecodedResponseAllowsUnusedCorrelationRuleColumns(t *testing.T) {
	correlation := &Correlation{
		Rules: map[string]CorrelationRule{
			"node_name": {
				Action:          "link",
				Priority:        10,
				KeySpace:        "node_name",
				Key:             []CorrelationKeyPart{{Column: "name"}},
				PointActorTypes: []string{"node"},
				OutputLinkType:  "dependency",
			},
		},
		Points: &Table{
			Rows: 1,
			Columns: []Column{
				NewColumn("actor", "actor_ref"),
				NewColumn("rule", "string"),
				NewColumn("name", "string_ref", WithDictionary("strings")),
			},
			Values: []ColumnEncoding{
				Values(0),
				Const("node_name"),
				Values(0),
			},
		},
	}
	err := validateResponseData(t, minimalValidationData(nil,
		withCorrelation(correlation),
		withLinks(dependencyLinkTable(0)),
	))

	require.NoError(t, err)
}

func TestPresentationTokenEnumsMatchSchema(t *testing.T) {
	schemaDoc := loadTopologySchema(t)

	assert.ElementsMatch(t, colorSlotTokens, schemaEnum(t, schemaDoc, "color_slot"))
	assert.ElementsMatch(t, opacityTokens, schemaEnum(t, schemaDoc, "opacity_token"))
	assert.ElementsMatch(t, widthTokens, schemaEnum(t, schemaDoc, "width_token"))
	assert.ElementsMatch(t, layoutStrengthTokens, schemaEnum(t, schemaDoc, "layout_strength_token"))
	assert.ElementsMatch(t, layoutDistanceTokens, schemaEnum(t, schemaDoc, "layout_distance_token"))
	assert.ElementsMatch(t, actorSizeScaleTokens, schemaEnum(t, schemaDoc, "actor_size_scale_token"))
	assert.ElementsMatch(t, linkSemanticRoleTokens, schemaEnum(t, schemaDoc, "link_semantic_role"))
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

func TestValidateDecodedResponseRejectsInvalidEvidenceSectionShape(t *testing.T) {
	payload := map[string]any{
		"status": float64(200),
		"type":   "topology",
		"data": map[string]any{
			"schema_version": SchemaVersion,
			"dictionaries":   map[string]any{},
			"types":          map[string]any{},
			"actors": map[string]any{
				"rows":    float64(0),
				"columns": []any{},
				"values":  []any{},
			},
			"links": map[string]any{
				"rows":    float64(0),
				"columns": []any{},
				"values":  []any{},
			},
			"evidence": map[string]any{
				"socket": "not-an-object",
			},
		},
	}

	err := ValidateDecodedResponse(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data.evidence.socket is not an object")
}

func validateResponseData(t *testing.T, data Data) error {
	t.Helper()

	payloadBytes, err := json.Marshal(NewResponse(data))
	require.NoError(t, err)

	var decoded any
	require.NoError(t, json.Unmarshal(payloadBytes, &decoded))
	return ValidateDecodedResponse(decoded)
}

func minimalValidationData(actorPresentation *ActorPresentation, opts ...func(*Data)) Data {
	data := Data{
		Producer:     Producer{Source: "test-topology", Instance: "test-instance"},
		CollectedAt:  testCollectedAt,
		Dictionaries: Dictionaries{"strings": StringValues("node-a")},
		Types: TypeRegistry{
			ActorTypes: map[string]ActorType{
				"node": {
					Layer:        "node",
					Identity:     []string{"id"},
					Presentation: actorPresentation,
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
		Actors: minimalActorTable(nil, nil),
		Links:  dependencyLinkTable(1),
	}

	for _, opt := range opts {
		opt(&data)
	}
	return data
}

func minimalActorTable(extraColumns []Column, extraValues []ColumnEncoding) Table {
	columns := []Column{
		NewColumn("id", "string", WithRole("identity")),
		NewColumn("type", "string"),
	}
	values := []ColumnEncoding{
		Values("node-a"),
		Const("node"),
	}
	columns = append(columns, extraColumns...)
	values = append(values, extraValues...)
	return MustTable(1, columns, values)
}

func dependencyLinkTable(rows int) Table {
	return dependencyLinkTableWith(rows, nil, nil)
}

func dependencyLinkTableWith(rows int, extraColumns []Column, extraValues []ColumnEncoding) Table {
	columns := []Column{
		NewColumn("src", "actor_ref"),
		NewColumn("dst", "actor_ref"),
		NewColumn("type", "string"),
	}
	values := []ColumnEncoding{
		Const(0),
		Const(0),
		Const("dependency"),
	}
	columns = append(columns, extraColumns...)
	values = append(values, extraValues...)
	return MustTable(rows, columns, values)
}

func withActors(actors Table) func(*Data) {
	return func(data *Data) {
		data.Actors = actors
	}
}

func withLinks(links Table) func(*Data) {
	return func(data *Data) {
		data.Links = links
	}
}

func withPortTypes(portTypes map[string]PortType) func(*Data) {
	return func(data *Data) {
		data.Types.PortTypes = portTypes
	}
}

func withCorrelation(correlation *Correlation) func(*Data) {
	return func(data *Data) {
		data.Correlation = correlation
	}
}

func validateAgainstTopologySchema(t *testing.T, payload []byte) {
	t.Helper()

	require.NoError(t, topologySchemaValidationError(t, payload))
}

func topologySchemaValidationError(t *testing.T, payload []byte) error {
	t.Helper()

	schemaDoc := loadTopologySchema(t)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", schemaDoc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)

	var decoded any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	return schema.Validate(decoded)
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
