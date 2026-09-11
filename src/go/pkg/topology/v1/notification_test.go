// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationGoModelRoundTripAndOmission(t *testing.T) {
	withoutNotifications := NewResponse(minimalValidationData(nil))
	payloadBytes, err := json.Marshal(withoutNotifications)
	require.NoError(t, err)
	assert.NotContains(t, string(payloadBytes), `"notifications"`)

	data := minimalValidationData(nil)
	data.Notifications = []Notification{
		{
			Severity: NotificationSeverityInfo,
			Code:     "discovery.complete",
			Message:  "Discovery completed.",
		},
		{
			Severity: NotificationSeverityWarning,
			Code:     "correlation.ambiguous",
			Message:  "Multiple correlation candidates were found.",
			Origin: &Producer{
				Source:       "network-connections",
				Instance:     "network-viewer",
				NodeID:       "node-a",
				MachineGUID:  "machine-a",
				AgentVersion: "1.99.0",
				Plugin:       "network-viewer.plugin",
				Capabilities: []string{"topology", "correlation"},
			},
		},
		{
			Severity:       NotificationSeverityError,
			Code:           "source.unavailable",
			Message:        "A requested source did not return topology data.",
			AffectedNodeID: "node-b",
		},
	}

	payload := NewResponse(data)
	payloadBytes, err = json.Marshal(payload)
	require.NoError(t, err)
	assert.Contains(t, string(payloadBytes), `"notifications"`)
	validateAgainstTopologySchema(t, payloadBytes)

	var decoded struct {
		Status int `json:"status"`
		Data   struct {
			Notifications []Notification `json:"notifications"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(payloadBytes, &decoded))
	assert.Equal(t, 200, decoded.Status)
	assert.Equal(t, data.Notifications, decoded.Data.Notifications)

	var decodedAny any
	require.NoError(t, json.Unmarshal(payloadBytes, &decodedAny))
	require.NoError(t, ValidateDecodedResponse(decodedAny))
}

func TestNotificationSchemaAndSemanticValidationAgree(t *testing.T) {
	tests := map[string]struct {
		present       bool
		notifications any
		valid         bool
		want          string
	}{
		"omitted": {
			valid: true,
		},
		"empty array": {
			present:       true,
			notifications: []any{},
			valid:         true,
		},
		"all severities": {
			present: true,
			notifications: []any{
				testNotification(map[string]any{"severity": "info"}),
				testNotification(map[string]any{"severity": "warning"}),
				testNotification(map[string]any{"severity": "error"}),
			},
			valid: true,
		},
		"identifier punctuation": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"code": "A:b.c-d_0"})},
			valid:         true,
		},
		"whitespace message": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"message": " \n"})},
			valid:         true,
		},
		"empty affected node": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"affected_node_id": ""})},
			valid:         true,
		},
		"null notifications": {
			present:       true,
			notifications: nil,
			want:          "data.notifications is not an array",
		},
		"notifications object": {
			present:       true,
			notifications: map[string]any{},
			want:          "data.notifications is not an array",
		},
		"null notification": {
			present:       true,
			notifications: []any{nil},
			want:          "data.notifications[0] is not an object",
		},
		"unknown notification field": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"extra": true})},
			want:          "data.notifications[0].extra is unknown",
		},
		"missing severity": {
			present:       true,
			notifications: []any{map[string]any{"code": "source.unavailable", "message": "Unavailable."}},
			want:          "data.notifications[0].severity is required",
		},
		"non-string severity": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"severity": 7})},
			want:          "data.notifications[0].severity is not a non-empty string",
		},
		"unsupported severity": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"severity": "fatal"})},
			want:          "data.notifications[0].severity has unsupported value",
		},
		"missing code": {
			present:       true,
			notifications: []any{map[string]any{"severity": "warning", "message": "Unavailable."}},
			want:          "data.notifications[0].code is required",
		},
		"non-string code": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"code": true})},
			want:          "data.notifications[0].code is not a valid identifier",
		},
		"code starts with number": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"code": "1source.unavailable"})},
			want:          "data.notifications[0].code is not a valid identifier",
		},
		"code contains space": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"code": "source unavailable"})},
			want:          "data.notifications[0].code is not a valid identifier",
		},
		"code contains newline": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"code": "source.unavailable\n"})},
			want:          "data.notifications[0].code is not a valid identifier",
		},
		"missing message": {
			present:       true,
			notifications: []any{map[string]any{"severity": "warning", "code": "source.unavailable"}},
			want:          "data.notifications[0].message is required",
		},
		"empty message": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"message": ""})},
			want:          "data.notifications[0].message is required",
		},
		"non-string message": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"message": 7})},
			want:          "data.notifications[0].message is required",
		},
		"null affected node": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"affected_node_id": nil})},
			want:          "data.notifications[0].affected_node_id is not a string",
		},
		"non-string affected node": {
			present:       true,
			notifications: []any{testNotification(map[string]any{"affected_node_id": 7})},
			want:          "data.notifications[0].affected_node_id is not a string",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			payload := decodedNotificationTestPayload(t)
			if tc.present {
				payload["data"].(map[string]any)["notifications"] = tc.notifications
			}

			assertNotificationValidation(t, payload, tc.valid, tc.want)
		})
	}
}

func TestNotificationOriginSchemaAndSemanticValidationAgree(t *testing.T) {
	tests := map[string]struct {
		origin any
		valid  bool
		want   string
	}{
		"explicit complete origin": {
			origin: testNotificationOrigin(map[string]any{
				"node_id":       "node-a",
				"machine_guid":  "machine-a",
				"agent_version": "1.99.0",
				"plugin":        "network-viewer.plugin",
				"capabilities":  []any{"topology", "correlation"},
			}),
			valid: true,
		},
		"empty producer strings and capabilities": {
			origin: testNotificationOrigin(map[string]any{
				"instance":      "",
				"node_id":       "",
				"machine_guid":  "",
				"agent_version": "",
				"plugin":        "",
				"capabilities":  []any{},
			}),
			valid: true,
		},
		"null origin": {
			origin: nil,
			want:   "data.notifications[0].origin is not an object",
		},
		"non-object origin": {
			origin: []any{},
			want:   "data.notifications[0].origin is not an object",
		},
		"origin missing source": {
			origin: map[string]any{"instance": "job-a"},
			want:   "data.notifications[0].origin.source is required",
		},
		"origin non-string source": {
			origin: testNotificationOrigin(map[string]any{"source": 7}),
			want:   "data.notifications[0].origin.source is not a valid identifier",
		},
		"origin source contains newline": {
			origin: testNotificationOrigin(map[string]any{"source": "network-connections\n"}),
			want:   "data.notifications[0].origin.source is not a valid identifier",
		},
		"origin missing instance": {
			origin: map[string]any{"source": "snmp"},
			want:   "data.notifications[0].origin.instance is required",
		},
		"origin non-string instance": {
			origin: testNotificationOrigin(map[string]any{"instance": 7}),
			want:   "data.notifications[0].origin.instance is not a string",
		},
		"unknown origin field": {
			origin: testNotificationOrigin(map[string]any{"extra": true}),
			want:   "data.notifications[0].origin.extra is unknown",
		},
		"origin null node id": {
			origin: testNotificationOrigin(map[string]any{"node_id": nil}),
			want:   "data.notifications[0].origin.node_id is not a string",
		},
		"origin non-string machine guid": {
			origin: testNotificationOrigin(map[string]any{"machine_guid": 7}),
			want:   "data.notifications[0].origin.machine_guid is not a string",
		},
		"origin null agent version": {
			origin: testNotificationOrigin(map[string]any{"agent_version": nil}),
			want:   "data.notifications[0].origin.agent_version is not a string",
		},
		"origin non-string plugin": {
			origin: testNotificationOrigin(map[string]any{"plugin": 7}),
			want:   "data.notifications[0].origin.plugin is not a string",
		},
		"origin capabilities not array": {
			origin: testNotificationOrigin(map[string]any{"capabilities": "topology"}),
			want:   "data.notifications[0].origin.capabilities is not an array",
		},
		"origin capabilities null": {
			origin: testNotificationOrigin(map[string]any{"capabilities": nil}),
			want:   "data.notifications[0].origin.capabilities is not an array",
		},
		"duplicate origin capability": {
			origin: testNotificationOrigin(map[string]any{"capabilities": []any{"topology", "topology"}}),
			want:   "data.notifications[0].origin.capabilities[1] duplicates value",
		},
		"invalid origin capability": {
			origin: testNotificationOrigin(map[string]any{"capabilities": []any{"bad capability"}}),
			want:   "data.notifications[0].origin.capabilities[0] is not a valid identifier",
		},
		"origin capability contains newline": {
			origin: testNotificationOrigin(map[string]any{"capabilities": []any{"topology\n"}}),
			want:   "data.notifications[0].origin.capabilities[0] is not a valid identifier",
		},
		"origin capability is not string": {
			origin: testNotificationOrigin(map[string]any{"capabilities": []any{7}}),
			want:   "data.notifications[0].origin.capabilities[0] is not a non-empty string",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			payload := decodedNotificationTestPayload(t)
			payload["data"].(map[string]any)["notifications"] = []any{
				testNotification(map[string]any{"origin": tc.origin}),
			}
			assertNotificationValidation(t, payload, tc.valid, tc.want)
		})
	}
}

func assertNotificationValidation(t *testing.T, payload map[string]any, valid bool, want string) {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	schemaErr := topologySchemaValidationError(t, payloadBytes)
	semanticErr := ValidateDecodedResponse(payload)
	assert.Equal(t, schemaErr == nil, semanticErr == nil, "schema and semantic validators disagree")
	if valid {
		assert.NoError(t, schemaErr)
		assert.NoError(t, semanticErr)
		return
	}
	assert.Error(t, schemaErr)
	if assert.Error(t, semanticErr) {
		assert.Contains(t, semanticErr.Error(), want)
	}
}

func testNotification(overrides map[string]any) map[string]any {
	notification := map[string]any{
		"severity": "warning",
		"code":     "source.unavailable",
		"message":  "Unavailable.",
	}
	for key, value := range overrides {
		notification[key] = value
	}
	return notification
}

func testNotificationOrigin(overrides map[string]any) map[string]any {
	origin := map[string]any{
		"source":   "network-connections",
		"instance": "network-viewer",
	}
	for key, value := range overrides {
		origin[key] = value
	}
	return origin
}

func decodedNotificationTestPayload(t *testing.T) map[string]any {
	t.Helper()

	payloadBytes, err := json.Marshal(NewResponse(minimalValidationData(nil)))
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	return payload
}
