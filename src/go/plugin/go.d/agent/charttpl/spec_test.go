// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeYAMLScenarios(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantErr bool
		assert  func(t *testing.T, spec *Spec)
	}{
		"valid grouped spec and default chart type": {
			input: `
version: v1
context_namespace: mysql
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        algorithm: incremental
        dimensions:
          - selector: mysql_queries_total
            name: total
`,
			assert: func(t *testing.T, spec *Spec) {
				t.Helper()
				require.Len(t, spec.Groups, 1)
				require.Len(t, spec.Groups[0].Charts, 1)
				assert.Equal(t, "line", spec.Groups[0].Charts[0].Type)
			},
		},
		"rejects unknown yaml field via strict unmarshal": {
			input: `
version: v1
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        algorithm: incremental
        unknown_field: true
        dimensions:
          - selector: mysql_queries_total
            name: total
`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			spec, err := DecodeYAML([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, spec)
			if tc.assert != nil {
				tc.assert(t, spec)
			}
		})
	}
}
