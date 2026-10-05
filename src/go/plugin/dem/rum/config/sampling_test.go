// SPDX-License-Identifier: GPL-3.0-or-later
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestInvestigateYAMLRoundTrip(t *testing.T) {
	for name, keep := range map[string][]string{"default": nil, "none": {}, "errors": {KeepErrors}, "both": {KeepErrors, KeepPoorVitals}} {
		t.Run(name, func(t *testing.T) {
			original := Site{
				AllowedOrigins: []string{"https://example.org"},
				Investigate: &Investigate{
					SampleRate: .1,
					AlwaysKeep: keep,
				},
			}
			encoded, err := yaml.Marshal(original)
			require.NoError(t, err)
			var decoded Site
			require.NoError(t, yaml.Unmarshal(encoded, &decoded))
			assert.Equal(t, original, decoded)
			assert.Equal(t, original.KeepsErrors(), decoded.KeepsErrors())
			assert.Equal(t, original.KeepsPoorVitals(), decoded.KeepsPoorVitals())
		})
	}
}
