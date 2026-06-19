// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildJSONPayload(t *testing.T) {
	tests := map[string]struct {
		code           int
		message        string
		expectMsgKey   string
		expectErrorKey string
	}{
		"success payload uses message key": {
			code:         200,
			message:      "ok",
			expectMsgKey: "ok",
		},
		"error payload uses errorMessage key": {
			code:           499,
			message:        "request canceled",
			expectErrorKey: "request canceled",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			payload := BuildJSONPayload(tc.code, tc.message)
			require.NotEmpty(t, payload)

			var decoded map[string]any
			require.NoError(t, json.Unmarshal(payload, &decoded))

			status, ok := decoded["status"].(float64)
			require.True(t, ok)
			assert.Equal(t, float64(tc.code), status)

			if tc.expectMsgKey != "" {
				assert.Equal(t, tc.expectMsgKey, decoded["message"])
				_, hasErrorMessage := decoded["errorMessage"]
				assert.False(t, hasErrorMessage)
			}
			if tc.expectErrorKey != "" {
				assert.Equal(t, tc.expectErrorKey, decoded["errorMessage"])
				_, hasMessage := decoded["message"]
				assert.False(t, hasMessage)
			}
		})
	}
}
