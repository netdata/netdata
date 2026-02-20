// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineLoadScenarios(t *testing.T) {
	tests := map[string]struct {
		initialYAML string
		initialRev  uint64
		reloadYAML  string
		reloadRev   uint64
		reloadErr   bool
		assert      func(t *testing.T, e *Engine)
	}{
		"load from yaml populates program and revision": {
			initialYAML: validTemplateYAML(),
			initialRev:  100,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				require.True(t, e.ready())
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(100), p.Revision())
				assert.Equal(t, "v1", p.Version())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
		"failed reload keeps previous compiled program": {
			initialYAML: validTemplateYAML(),
			initialRev:  200,
			reloadYAML: `
version: v1
groups:
  - family: Database
    charts:
      - title: Broken
        context: broken
        units: "1"
        dimensions:
          - selector: mysql_queries_total
`,
			reloadRev: 201,
			reloadErr: true,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(200), p.Revision())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
		"invalid selector syntax on reload is rejected and keeps previous program": {
			initialYAML: validTemplateYAML(),
			initialRev:  300,
			reloadYAML: `
version: v1
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Broken selector
        context: broken_selector
        units: queries/s
        dimensions:
          - selector: mysql_queries_total{method="GET",}
            name: total
`,
			reloadRev: 301,
			reloadErr: true,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(300), p.Revision())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
		"invalid engine selector on reload is rejected and keeps previous program": {
			initialYAML: validTemplateYAML(),
			initialRev:  400,
			reloadYAML: `
version: v1
engine:
  selector:
    allow:
      - mysql_queries_total{db="main",}
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        dimensions:
          - selector: mysql_queries_total
            name: total
`,
			reloadRev: 401,
			reloadErr: true,
			assert: func(t *testing.T, e *Engine) {
				t.Helper()
				p := e.program()
				require.NotNil(t, p)
				assert.Equal(t, uint64(400), p.Revision())
				assert.Equal(t, []string{"mysql_queries_total"}, p.MetricNames())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New()
			require.NoError(t, err)

			require.NoError(t, e.LoadYAML([]byte(tc.initialYAML), tc.initialRev))
			if tc.reloadYAML != "" {
				err = e.LoadYAML([]byte(tc.reloadYAML), tc.reloadRev)
				if tc.reloadErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			if tc.assert != nil {
				tc.assert(t, e)
			}
		})
	}
}

func validTemplateYAML() string {
	return `
version: v1
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        dimensions:
          - selector: mysql_queries_total
            name: total
`
}
