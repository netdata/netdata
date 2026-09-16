// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseMetadataClassifiesCompatibilityHeaders(t *testing.T) {
	tests := map[string]struct {
		header          http.Header
		wantContentType string
		wantOData       string
	}{
		"valid": {
			header: http.Header{
				"Content-Type":  []string{"application/json; charset=utf-8"},
				"Odata-Version": []string{"4.0"},
			},
			wantContentType: "valid",
			wantOData:       "valid",
		},
		"missing": {
			header:          http.Header{},
			wantContentType: "missing",
			wantOData:       "missing",
		},
		"wrong media type": {
			header: http.Header{
				"Content-Type":  []string{"text/plain"},
				"Odata-Version": []string{"3.0"},
			},
			wantContentType: "invalid",
			wantOData:       "invalid",
		},
		"wrong charset": {
			header: http.Header{
				"Content-Type": []string{"application/json; charset=iso-8859-1"},
			},
			wantContentType: "invalid",
			wantOData:       "missing",
		},
		"malformed media type": {
			header: http.Header{
				"Content-Type": []string{"application/json; charset"},
			},
			wantContentType: "invalid",
			wantOData:       "missing",
		},
		"oversize headers": {
			header: http.Header{
				"Content-Type":  []string{strings.Repeat(" ", maxContentTypeBytes) + "application/json"},
				"Odata-Version": []string{strings.Repeat(" ", maxODataVersionBytes) + "4.0"},
			},
			wantContentType: "invalid",
			wantOData:       "invalid",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := responseContentTypeState(test.header); got != test.wantContentType {
				t.Errorf("content type state = %q, want %q", got, test.wantContentType)
			}
			if got := responseODataVersionState(test.header); got != test.wantOData {
				t.Errorf("OData version state = %q, want %q", got, test.wantOData)
			}
		})
	}
}

func TestDecodeJSONToleratesCompatibilityHeadersAfterStructuralChecks(t *testing.T) {
	response := &responseData{
		header: http.Header{
			"Content-Type":  []string{"text/plain"},
			"Odata-Version": []string{"3.0"},
		},
		body: []byte(`{"@odata.id":"/redfish/v1/Managers/1"}`),
	}
	var decoded map[string]any
	require.NoError(t, decodeJSON(response, &decoded))
	assert.Equal(t, "/redfish/v1/Managers/1", decoded["@odata.id"])
}
