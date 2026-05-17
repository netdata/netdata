// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func TestTopologyV1FixturesValidate(t *testing.T) {
	schemaBytes := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	fixtures, err := filepath.Glob(filepath.Join("..", "fixtures", "topology-v1", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(fixtures) != 4 {
		t.Fatalf("expected 4 topology fixtures, got %d: %v", len(fixtures), fixtures)
	}

	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			inputBytes := readTestFile(t, fixture)
			if _, err := validateJSON(schemaBytes, inputBytes); err != nil {
				t.Fatalf("validate fixture: %v", err)
			}
		})
	}
}

func TestTopologyV1EnvelopeVersionValidates(t *testing.T) {
	schemaBytes := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	fixture := readTestFile(t, filepath.Join("..", "fixtures", "topology-v1", "snmp-l2.json"))

	var payload map[string]any
	if err := json.Unmarshal(fixture, &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	payload["v"] = float64(3)

	inputBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if _, err := validateJSON(schemaBytes, inputBytes); err != nil {
		t.Fatalf("validate fixture with envelope version: %v", err)
	}
}

func TestFunctionUISchemaValidationSkipsTopologySemanticsForTableResponses(t *testing.T) {
	schemaBytes := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_UI_SCHEMA.json"))
	input := []byte(`{
		"status": 200,
		"type": "table",
		"columns": {
			"name": {"index": 0, "name": "Name", "type": "string", "visualization": "value"}
		},
		"data": [["row-a"]]
	}`)

	if _, err := validateJSON(schemaBytes, input); err != nil {
		t.Fatalf("validate table response: %v", err)
	}
}

func TestTopologySemanticChecksRejectColumnLengthMismatch(t *testing.T) {
	payload := decodeTestJSON(t, `{
		"status": 200,
		"type": "topology",
		"data": {
			"schema_version": "netdata.topology.v1",
			"dictionaries": {"strings": ["node"]},
			"actors": {
				"rows": 2,
				"columns": [{"id": "type", "type": "string_ref", "dictionary": "strings"}],
				"values": [{"codec": "values", "values": [0]}]
			},
			"links": {"rows": 0, "columns": [], "values": []}
		}
	}`)

	err := topologyv1.ValidateDecodedResponse(payload)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "decoded length mismatch") {
		t.Fatalf("expected decoded length mismatch, got %v", err)
	}
}

func TestTopologySemanticChecksRejectOutOfBoundsActorReference(t *testing.T) {
	payload := decodeTestJSON(t, `{
		"status": 200,
		"type": "topology",
		"data": {
			"schema_version": "netdata.topology.v1",
			"dictionaries": {"strings": ["node", "depends_on"]},
			"actors": {
				"rows": 1,
				"columns": [{"id": "type", "type": "string_ref", "dictionary": "strings"}],
				"values": [{"codec": "const", "value": 0}]
			},
			"links": {
				"rows": 1,
				"columns": [
					{"id": "src_actor", "type": "actor_ref"},
					{"id": "dst_actor", "type": "actor_ref"},
					{"id": "type", "type": "string_ref", "dictionary": "strings"}
				],
				"values": [
					{"codec": "const", "value": 0},
					{"codec": "const", "value": 2},
					{"codec": "const", "value": 1}
				]
			}
		}
	}`)

	err := topologyv1.ValidateDecodedResponse(payload)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "actor reference out of bounds") {
		t.Fatalf("expected actor reference bounds error, got %v", err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func decodeTestJSON(t *testing.T, input string) any {
	t.Helper()
	var payload any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		t.Fatalf("decode test JSON: %v", err)
	}
	return payload
}
