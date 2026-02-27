// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTemplateLiteralOnly(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in      string
		wantRaw string
		wantErr bool
	}{
		"plain literal": {
			in:      "mysql_queries",
			wantRaw: "mysql_queries",
		},
		"trims surrounding whitespace": {
			in:      "  mysql_queries  ",
			wantRaw: "mysql_queries",
		},
		"keeps braces as literal characters": {
			in:      "osd_{osd_uuid}_space_usage",
			wantRaw: "osd_{osd_uuid}_space_usage",
		},
		"keeps pipes as literal characters": {
			in:      "disk|iops|total",
			wantRaw: "disk|iops|total",
		},
		"keeps malformed placeholder-like text as literal": {
			in:      `{key|replace("unterminated)}`,
			wantRaw: `{key|replace("unterminated)}`,
		},
		"rejects empty template": {
			in:      " \t\n ",
			wantErr: true,
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := parseTemplate(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantRaw, got.Raw)
		})
	}
}
