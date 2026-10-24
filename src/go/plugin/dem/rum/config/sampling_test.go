// SPDX-License-Identifier: GPL-3.0-or-later
package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestInvestigateYAMLRoundTrip(t *testing.T) {
	for name, keep := range map[string][]string{"default": nil, "none": {}, "errors": {KeepErrors}, "both": {KeepErrors, KeepPoorVitals}} {
		t.Run(name, func(t *testing.T) {
			rate := .1
			original := Site{
				AllowedOrigins: []string{"https://example.org"},
				Investigate: &Investigate{
					SampleRate: &rate,
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

func TestSamplingDecodeAndEffectiveRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, json, yaml string
		rate             float64
		keep             bool
	}{
		{"omitted", `{}`, `{}`, 1, true},
		{"null", `{"measure_sample_rate":null,"investigate":null}`, "measure_sample_rate: null\ninvestigate: null", 1, true},
		{"null fields", `{"investigate":{"sample_rate":null,"always_keep":null}}`, "investigate: {sample_rate: null, always_keep: null}", 1, true},
		{"zero", `{"measure_sample_rate":0,"investigate":{"sample_rate":0}}`, "measure_sample_rate: 0\ninvestigate: {sample_rate: 0}", 0, true},
		{"zero no overrides", `{"measure_sample_rate":0,"investigate":{"sample_rate":0,"always_keep":[]}}`, "measure_sample_rate: 0\ninvestigate: {sample_rate: 0, always_keep: []}", 0, false},
		{"fraction", `{"measure_sample_rate":0.25,"investigate":{"sample_rate":0.25}}`, "measure_sample_rate: 0.25\ninvestigate: {sample_rate: 0.25}", .25, true},
		{"one", `{"measure_sample_rate":1,"investigate":{"sample_rate":1}}`, "measure_sample_rate: 1\ninvestigate: {sample_rate: 1}", 1, true},
	} {
		for _, format := range []string{"json", "yaml"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				site := Site{
					AllowedOrigins: []string{"https://shop.example.org"},
				}
				if format == "json" {
					require.NoError(t, json.Unmarshal([]byte(tc.json), &site))
				} else {
					require.NoError(t, yaml.Unmarshal([]byte(tc.yaml), &site))
				}
				before, err := json.Marshal(site)
				require.NoError(t, err)
				effective := site.Effective()
				after, err := json.Marshal(site)
				require.NoError(t, err)
				assert.Equal(t, before, after, "default materialization must not mutate its input")
				for _, candidate := range []Site{site, effective} {
					assert.Equal(t, tc.rate, candidate.MeasureRate())
					assert.Equal(t, tc.rate, candidate.InvestigateRate())
					assert.Equal(t, tc.keep, candidate.KeepsErrors())
					assert.Equal(t, tc.keep, candidate.KeepsPoorVitals())
					for _, codec := range []struct {
						marshal   func(any) ([]byte, error)
						unmarshal func([]byte, any) error
					}{{json.Marshal, json.Unmarshal}, {yaml.Marshal, yaml.Unmarshal}} {
						encoded, err := codec.marshal(candidate)
						require.NoError(t, err)
						var restored Site
						require.NoError(t, codec.unmarshal(encoded, &restored))
						assert.Equal(t, candidate, restored)
					}
				}
			})
		}
	}
}
