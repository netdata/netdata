// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlexibleStringSlice_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected FlexibleStringSlice
	}{
		{
			name:     "single string (old RabbitMQ format)",
			input:    `"administrator"`,
			expected: FlexibleStringSlice{"administrator"},
		},
		{
			name:     "array of strings (new RabbitMQ format)",
			input:    `["administrator"]`,
			expected: FlexibleStringSlice{"administrator"},
		},
		{
			name:     "array with multiple strings",
			input:    `["administrator", "monitoring"]`,
			expected: FlexibleStringSlice{"administrator", "monitoring"},
		},
		{
			name:     "empty array",
			input:    `[]`,
			expected: FlexibleStringSlice{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result FlexibleStringSlice
			err := json.Unmarshal([]byte(tt.input), &result)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlexibleStringSlice_UnmarshalJSON_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid JSON",
			input: `{invalid json}`,
		},
		{
			name:  "number instead of string",
			input: `123`,
		},
		{
			name:  "object instead of string or array",
			input: `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result FlexibleStringSlice
			err := json.Unmarshal([]byte(tt.input), &result)
			assert.Error(t, err)
		})
	}
}

func TestApiWhoamiResp_UnmarshalJSON_OldRabbitMQFormat(t *testing.T) {
	// Test the actual use case: old RabbitMQ format with single string tags
	oldFormatJSON := `{
		"name": "myuser",
		"tags": "administrator"
	}`

	var resp apiWhoamiResp
	err := json.Unmarshal([]byte(oldFormatJSON), &resp)
	require.NoError(t, err)

	assert.Equal(t, "myuser", resp.Name)
	assert.Equal(t, FlexibleStringSlice{"administrator"}, resp.Tags)

	// Verify that slices.Contains works with the result
	assert.True(t, len(resp.Tags) > 0)
	// This is how it's used in collect.go
	hasAdministrator := false
	for _, tag := range resp.Tags {
		if tag == "administrator" {
			hasAdministrator = true
			break
		}
	}
	assert.True(t, hasAdministrator)
}

func TestApiWhoamiResp_UnmarshalJSON_NewRabbitMQFormat(t *testing.T) {
	// Test the current format: new RabbitMQ format with array of strings
	newFormatJSON := `{
		"name": "guest",
		"tags": ["administrator"]
	}`

	var resp apiWhoamiResp
	err := json.Unmarshal([]byte(newFormatJSON), &resp)
	require.NoError(t, err)

	assert.Equal(t, "guest", resp.Name)
	assert.Equal(t, FlexibleStringSlice{"administrator"}, resp.Tags)

	// Verify that slices.Contains works with the result
	assert.True(t, len(resp.Tags) > 0)
	// This is how it's used in collect.go
	hasAdministrator := false
	for _, tag := range resp.Tags {
		if tag == "administrator" {
			hasAdministrator = true
			break
		}
	}
	assert.True(t, hasAdministrator)
}

func TestApiWhoamiResp_RealTestData(t *testing.T) {
	// Test with actual test data files to ensure real-world compatibility
	tests := []struct {
		name        string
		testDataFile string
		expectArrayFormat bool
	}{
		{
			name:         "v4.0.3 - new format with array tags",
			testDataFile: "testdata/v4.0.3/cluster/whoami.json",
			expectArrayFormat: true,
		},
		{
			name:         "v3.8.2 - old format with single string tags",
			testDataFile: "testdata/v3.8.2/cluster/whoami.json",
			expectArrayFormat: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.testDataFile)
			require.NoError(t, err)

			var resp apiWhoamiResp
			err = json.Unmarshal(data, &resp)
			require.NoError(t, err)

			assert.NotEmpty(t, resp.Name)
			assert.NotEmpty(t, resp.Tags)
			
			// Verify administrator tag is present (both formats should have it)
			hasAdministrator := false
			for _, tag := range resp.Tags {
				if tag == "administrator" {
					hasAdministrator = true
					break
				}
			}
			assert.True(t, hasAdministrator, "Expected to find 'administrator' tag")
		})
	}
}