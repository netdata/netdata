// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationValidatorReportsMissingRequiredFields(t *testing.T) {
	tests := map[string]struct {
		remove string
		origin bool
		want   string
	}{
		"severity": {
			remove: "severity",
			want:   "data.notifications[0].severity is required",
		},
		"code": {
			remove: "code",
			want:   "data.notifications[0].code is required",
		},
		"message": {
			remove: "message",
			want:   "data.notifications[0].message is required",
		},
		"origin source": {
			remove: "source",
			origin: true,
			want:   "data.notifications[0].origin.source is required",
		},
		"origin instance": {
			remove: "instance",
			origin: true,
			want:   "data.notifications[0].origin.instance is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			payload := decodedRequiredFieldTestPayload(t)
			notification := payload["data"].(map[string]any)["notifications"].([]any)[0].(map[string]any)
			if tc.origin {
				delete(notification["origin"].(map[string]any), tc.remove)
			} else {
				delete(notification, tc.remove)
			}

			require.EqualError(t, ValidateDecodedResponse(payload), tc.want)
		})
	}
}

func TestTopologyIDPatternMatchesSchema(t *testing.T) {
	schemaDoc := loadTopologySchema(t)
	definitions, ok := schemaDoc["$defs"].(map[string]any)
	require.True(t, ok)
	idDefinition, ok := definitions["id"].(map[string]any)
	require.True(t, ok)
	schemaPattern, ok := idDefinition["pattern"].(string)
	require.True(t, ok)

	assert.Equal(t, schemaPattern, topologyIDPattern.String())
}

func decodedRequiredFieldTestPayload(t *testing.T) map[string]any {
	t.Helper()

	payload := decodedNotificationTestPayload(t)
	payload["data"].(map[string]any)["notifications"] = []any{
		testNotification(map[string]any{"origin": testNotificationOrigin(nil)}),
	}
	return payload
}
