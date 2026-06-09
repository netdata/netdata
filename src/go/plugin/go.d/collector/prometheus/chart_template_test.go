// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChartTemplate(t *testing.T) {
	tests := map[string]struct {
		app           string
		wantNamespace string
	}{
		"no app uses the prometheus namespace": {
			app:           "",
			wantNamespace: "prometheus",
		},
		"app is folded into the namespace with the separating dot": {
			app:           "myapp",
			wantNamespace: "prometheus.myapp",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := buildChartTemplate(tc.app)
			require.NoError(t, err)

			// Parse back through charttpl's own canonical decoder (yaml.v2 UnmarshalStrict, the path
			// chartengine uses) so the round-trip is validated against the real template contract.
			spec, err := charttpl.DecodeYAML([]byte(out))
			require.NoError(t, err)

			assert.Equal(t, charttpl.VersionV1, spec.Version)
			assert.Equal(t, tc.wantNamespace, spec.ContextNamespace, "context_namespace drives the V1-parity chart context")
			require.NotNil(t, spec.Engine)
			require.NotNil(t, spec.Engine.Autogen)
			assert.True(t, spec.Engine.Autogen.Enabled, "autogen must be enabled (no static charts)")
			assert.Equal(t, uint64(chartExpireAfterCycles), spec.Engine.Autogen.ExpireAfterSuccessCycles,
				"autogen chart expiry must mirror V1's 10-cycle stale removal")
			require.Len(t, spec.Groups, 1, "a stub group satisfies the non-empty groups requirement")
			assert.Equal(t, "prometheus", spec.Groups[0].Family)

			// chartengine must accept the generated template (compiles + publishes a revision).
			eng, err := chartengine.New()
			require.NoError(t, err)
			require.NoError(t, eng.LoadYAML([]byte(out), 1))
		})
	}
}
