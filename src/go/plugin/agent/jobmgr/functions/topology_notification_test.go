// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMethodGenerationTopologyNotifications(t *testing.T) {
	topology := topologyv1.Data{
		SchemaVersion: topologyv1.SchemaVersion,
		Producer: topologyv1.Producer{
			Source:   "test-topology",
			Instance: "sample-node",
			NodeID:   "node-a",
			Plugin:   "go-test",
		},
		CollectedAt:  time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC),
		Dictionaries: topologyv1.Dictionaries{},
		Types: topologyv1.TypeRegistry{
			ActorTypes: map[string]topologyv1.ActorType{
				"node": {
					Layer:    "node",
					Identity: []string{"id"},
				},
			},
			LinkTypes: map[string]topologyv1.LinkType{
				"dependency": {
					Orientation:   "directed",
					DirectionRole: "dependency",
					Aggregation:   topologyv1.LinkAggregation{Direction: "preserve"},
				},
			},
		},
		Actors: topologyv1.MustTable(2, []topologyv1.Column{
			topologyv1.NewColumn("id", "string", topologyv1.WithRole("identity")),
			topologyv1.NewColumn("type", "string"),
		}, []topologyv1.ColumnEncoding{
			topologyv1.Values("node-a", "node-b"),
			topologyv1.Const("node"),
		}),
		Links: topologyv1.MustTable(1, []topologyv1.Column{
			topologyv1.NewColumn("src", "actor_ref"),
			topologyv1.NewColumn("dst", "actor_ref"),
			topologyv1.NewColumn("type", "string"),
		}, []topologyv1.ColumnEncoding{
			topologyv1.Const(0),
			topologyv1.Const(1),
			topologyv1.Const("dependency"),
		}),
	}
	graphJSON, err := json.Marshal(topology)
	require.NoError(t, err)

	origin := &topologyv1.Producer{
		Source:       "test-topology",
		Instance:     "test-instance",
		NodeID:       "node-b",
		MachineGUID:  "machine-b",
		AgentVersion: "test",
		Plugin:       "go-test",
		Capabilities: []string{"topology-v1", "aggregation"},
	}
	tests := map[string]struct {
		notifications []topologyv1.Notification
		wantJSON      string
	}{
		"omitted notifications": {},
		"empty notifications": {
			notifications: []topologyv1.Notification{},
		},
		"info with inherited origin": {
			notifications: []topologyv1.Notification{{
				Severity: topologyv1.NotificationSeverityInfo,
				Code:     "test_context",
				Message:  "Source \"sample-node\" returned <graph> data.\nDetails are plain text.",
			}},
			wantJSON: `[{"severity":"info","code":"test_context","message":"Source \"sample-node\" returned <graph> data.\nDetails are plain text."}]`,
		},
		"warning with inherited origin": {
			notifications: []topologyv1.Notification{{
				Severity:       topologyv1.NotificationSeverityWarning,
				Code:           "test_ambiguity",
				Message:        "The test graph contains an unresolved relationship.",
				AffectedNodeID: "node-a",
			}},
			wantJSON: `[{"severity":"warning","code":"test_ambiguity","message":"The test graph contains an unresolved relationship.","affected_node_id":"node-a"}]`,
		},
		"warning preserves full explicit origin": {
			notifications: []topologyv1.Notification{{
				Severity:       topologyv1.NotificationSeverityWarning,
				Code:           "test_ambiguity",
				Message:        "The test graph contains an unresolved relationship.",
				Origin:         origin,
				AffectedNodeID: "node-a",
			}},
			wantJSON: `[{"severity":"warning","code":"test_ambiguity","message":"The test graph contains an unresolved relationship.",
				"origin":{"source":"test-topology","instance":"test-instance","node_id":"node-b","machine_guid":"machine-b","agent_version":"test","plugin":"go-test","capabilities":["topology-v1","aggregation"]},
				"affected_node_id":"node-a"}]`,
		},
		"error notification retains successful graph response": {
			notifications: []topologyv1.Notification{{
				Severity:       topologyv1.NotificationSeverityError,
				Code:           "test_source_failure",
				Message:        "One test source failed; other graph data remains available.",
				Origin:         origin,
				AffectedNodeID: "node-a",
			}},
			wantJSON: `[{"severity":"error","code":"test_source_failure","message":"One test source failed; other graph data remains available.",
				"origin":{"source":"test-topology","instance":"test-instance","node_id":"node-b","machine_guid":"machine-b","agent_version":"test","plugin":"go-test","capabilities":["topology-v1","aggregation"]},
				"affected_node_id":"node-a"}]`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			data := topology
			data.Notifications = test.notifications
			generation := methodGeneration{kind: methodGenerationShared}
			result, err := generation.responseResult(funcapi.FunctionConfig{
				ID:           "topology",
				ResponseType: topologyv1.ResponseType,
			}, nil, &funcapi.FunctionResponse{
				Status: 200,
				Data:   data,
			})
			require.NoError(t, err)

			payload := topologyNotificationResultPayload(t, result, 200)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(payload, &decoded))
			require.NoError(t, topologyv1.ValidateDecodedResponse(decoded))
			var envelope map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(payload, &envelope))
			assert.JSONEq(t, `200`, string(envelope["status"]))
			assert.JSONEq(t, `"topology"`, string(envelope["type"]))
			assert.NotContains(t, envelope, "errorMessage")
			assert.NotContains(t, envelope, "notifications")

			var gotData map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(envelope["data"], &gotData))
			if test.wantJSON == "" {
				assert.NotContains(t, gotData, "notifications")
			} else {
				assert.JSONEq(t, test.wantJSON, string(gotData["notifications"]))
			}
			delete(gotData, "notifications")
			gotGraphJSON, err := json.Marshal(gotData)
			require.NoError(t, err)
			assert.JSONEq(t, string(graphJSON), string(gotGraphJSON))
		})
	}
}

func TestMethodGenerationTopologyFatalResponse(t *testing.T) {
	for name, status := range map[string]int{"bad request": 400, "unavailable": 503} {
		t.Run(name, func(t *testing.T) {
			generation := methodGeneration{kind: methodGenerationShared}
			result, err := generation.responseResult(funcapi.FunctionConfig{
				ID:           "topology",
				ResponseType: topologyv1.ResponseType,
			}, nil, &funcapi.FunctionResponse{
				Status:  status,
				Message: "The topology request failed.",
				Data: topologyv1.Data{
					Notifications: []topologyv1.Notification{{
						Severity: topologyv1.NotificationSeverityError,
						Code:     "test_source_failure",
						Message:  "The test source is unavailable.",
					}},
				},
			})
			require.NoError(t, err)

			payload := topologyNotificationResultPayload(t, result, status)
			assert.JSONEq(t, fmt.Sprintf(`{"status":%d,"errorMessage":"The topology request failed."}`, status),
				string(payload))
		})
	}
}

func topologyNotificationResultPayload(t *testing.T, result lifecycle.SealedResult, status int) []byte {
	t.Helper()

	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	frame, err := lifecycle.PrepareFrame("topology-test", result, 1)
	require.NoError(t, err)
	require.NoError(t, owner.Commit(frame))
	header, body, ok := strings.Cut(output.String(), "\n")
	require.True(t, ok)
	assert.Equal(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN topology-test %d application/json 1", status), header)
	payload, ok := strings.CutSuffix(body, "\nFUNCTION_RESULT_END\n\n")
	require.True(t, ok)
	return []byte(payload)
}
