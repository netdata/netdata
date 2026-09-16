// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestSourceTypeFromPath(t *testing.T) {
	tests := map[string]struct {
		path string
		want string
	}{
		"user path under /etc": {
			path: "/etc/netdata/sd.d/test.conf",
			want: confgroup.TypeUser,
		},
		"stock path outside /etc": {
			path: "/usr/lib/netdata/conf.d/sd.d/test.conf",
			want: confgroup.TypeStock,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, sourceTypeFromPath(tc.path))
		})
	}
}

func TestSDConfigTrustRoundTrip(t *testing.T) {
	for name, option := range map[string]string{
		"absent": "",
		"false":  "trust_discovered_targets: false",
		"true":   "trust_discovered_targets: true",
		"null":   "trust_discovered_targets: null",
	} {
		t.Run(name, func(t *testing.T) {
			input := []byte("name: job\n" + option + "\ndiscoverer: {fixture: {}}\nservices: [{id: fixture, match: '{{ true }}'}]\n")
			config, err := newSDConfigFromYAML(input, "fixture.conf", confgroup.TypeUser, "fixture:job")
			require.NoError(t, err)
			fromJSON, err := newSDConfigFromJSON(config.DataJSON(), "job", "user", confgroup.TypeDyncfg, "fixture", "fixture:job")
			require.NoError(t, err)
			pipelineConfig, err := fromJSON.ToPipelineConfig(nil)
			require.NoError(t, err)
			require.Equal(t, name == "true", pipelineConfig.TrustDiscoveredTargets)
			yamlData, err := yaml.Marshal(pipelineConfig)
			require.NoError(t, err)
			roundtrip, err := newSDConfigFromYAML(yamlData, "fixture.conf", confgroup.TypeUser, "fixture:job")
			require.NoError(t, err)
			actual, err := roundtrip.ToPipelineConfig(nil)
			require.NoError(t, err)
			require.Equal(t, name == "true", actual.TrustDiscoveredTargets)
			// Exercise explicit JSON false/null as well as the normalized YAML-to-JSON path.
			value := map[string]string{"absent": "null", "false": "false", "true": "true", "null": "null"}[name]
			direct, err := newSDConfigFromJSON([]byte(fmt.Sprintf(`{"discoverer":{"fixture":{}},"trust_discovered_targets":%s}`, value)),
				"job", "user", confgroup.TypeDyncfg, "fixture", "fixture:job")
			require.NoError(t, err)
			directConfig, err := direct.ToPipelineConfig(nil)
			require.NoError(t, err)
			require.Equal(t, name == "true", directConfig.TrustDiscoveredTargets)

			var raw map[string]any
			require.NoError(t, json.Unmarshal(config.DataJSON(), &raw))
			raw["trust_discovered_targets"] = "yes"
			invalid, err := json.Marshal(raw)
			require.NoError(t, err)
			invalidConfig, err := newSDConfigFromJSON(invalid, "job", "user", confgroup.TypeDyncfg, "fixture", "fixture:job")
			require.NoError(t, err)
			_, err = invalidConfig.ToPipelineConfig(nil)
			require.Error(t, err)
		})
	}
}

func TestSDConfigPipelineIdentityIsOwnerStamped(t *testing.T) {
	for _, sourceType := range []string{confgroup.TypeUser, confgroup.TypeDyncfg} {
		t.Run(sourceType, func(t *testing.T) {
			for _, key := range []string{"pipeline-a", "pipeline-b"} {
				var config sdConfig
				var err error
				if sourceType == confgroup.TypeUser {
					config, err = newSDConfigFromYAML([]byte("name: job\ndiscoverer: {fixture: {}}\n__pipeline_key__: forged\npipeline_id: forged\n"), "shared-source", sourceType, key)
				} else {
					config, err = newSDConfigFromJSON([]byte(`{"discoverer":{"fixture":{}},"__pipeline_key__":"forged","pipeline_id":"forged"}`), "job", "shared-source", sourceType, "fixture", key)
				}
				require.NoError(t, err)
				actual, err := config.ToPipelineConfig(nil)
				require.NoError(t, err)
				require.Equal(t, key, actual.PipelineID)
				// Serialized operator configuration must not carry owner authority.
				jsonData, err := json.Marshal(actual)
				require.NoError(t, err)
				yamlData, err := yaml.Marshal(actual)
				require.NoError(t, err)
				for _, data := range [][]byte{jsonData, yamlData, config.DataJSON()} {
					require.NotContains(t, string(data), key)
				}
				reloaded, err := newSDConfigFromYAML(yamlData, "shared-source", sourceType, key)
				require.NoError(t, err)
				roundtrip, err := reloaded.ToPipelineConfig(nil)
				require.NoError(t, err)
				require.Equal(t, key, roundtrip.PipelineID)
			}
		})
	}
}
