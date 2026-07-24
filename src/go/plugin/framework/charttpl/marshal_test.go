// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"testing"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecMarshalTemplate(t *testing.T) {
	tests := map[string]struct {
		spec    Spec
		wantErr bool
		check   func(t *testing.T, out string)
	}{
		"omits unset optional fields without applying decode defaults": {
			spec: Spec{
				Version: VersionV1,
				Groups: []Group{{
					Family:  "G",
					Metrics: []string{"m"},
					Charts: []Chart{{
						Title:      "C",
						Context:    "c",
						Units:      "u",
						Dimensions: []Dimension{{Selector: "m", Name: "d"}},
					}},
				}},
			},
			check: func(t *testing.T, out string) {
				// applyDefaults would inject type: line; MarshalTemplate must not.
				assert.NotContains(t, out, "type:")
			},
		},
		"errors when groups missing": {
			spec:    Spec{Version: VersionV1},
			wantErr: true,
		},
		"errors on wrong version": {
			spec: Spec{
				Version: "v2",
				Groups:  []Group{richGroup()},
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := tc.spec.MarshalTemplate()
			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, out)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, out)
			}
		})
	}
}

func TestSpecMarshalTemplateUsesYAMLV2(t *testing.T) {
	spec := richSpec()

	out, err := spec.MarshalTemplate()
	require.NoError(t, err)

	// The emitted bytes must be exactly what the decoder's yaml library produces.
	assert.Equal(t, mustMarshalV2(t, spec), out)
}

func TestSpecMarshalTemplateRoundTrip(t *testing.T) {
	// No chart_defaults: decode-time inheritance must not diverge across the
	// round-trip. The fixture is decoded first so defaults are already applied.
	const tmpl = `
version: v1
context_namespace: ns
groups:
  - family: Root
    metrics: [metric_a]
    charts:
      - title: Chart A
        context: chart_a
        units: units/s
        dimensions:
          - selector: metric_a
            name: a
`
	spec, err := DecodeYAML([]byte(tmpl))
	require.NoError(t, err)

	out, err := spec.MarshalTemplate()
	require.NoError(t, err)

	reDecoded, err := DecodeYAML([]byte(out))
	require.NoError(t, err)

	assert.Equal(t, spec, reDecoded)
}

func richSpec() Spec {
	return Spec{
		Version:          VersionV1,
		ContextNamespace: "ns",
		Engine: &Engine{
			Autogen: &EngineAutogen{
				Enabled: true,
				Rules: []EngineAutogenRule{
					{
						Scope: "metric_*",
						Selector: metrixselector.Expr{
							Deny: []string{"metric_*"},
						},
					},
				},
				MaxTypeIDLen: 512,
			},
		},
		Groups: []Group{richGroup()},
	}
}
