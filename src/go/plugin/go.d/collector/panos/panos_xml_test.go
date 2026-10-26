// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodePANOSResultInner(t *testing.T) {
	for name, tc := range map[string]struct {
		body    string
		want    string
		wantErr string
	}{
		"missing result": {
			body: `<response status="success"/>`, wantErr: "expected <result>",
		},
		"empty result": {
			body: `<response status="success"><result/></response>`,
		},
		"populated result": {
			body: `<response status="success"><result><entry/></result></response>`, want: `<entry/>`,
		},
		"error without result preserves message": {
			body: `<response status="error"><msg><line>permission denied</line></msg></response>`, wantErr: "permission denied",
		},
		"error inside result preserves message": {
			body: `<response status="error"><result><msg><line>invalid command</line></msg></result></response>`, wantErr: "invalid command",
		},
		"error code without result": {
			body: `<response code="403"/>`, wantErr: "403",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodePANOSResultInner([]byte(tc.body), "test response")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
