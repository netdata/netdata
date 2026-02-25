// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDiscovererPayload_RoundTrip(t *testing.T) {
	tests := map[string]struct {
		input     string
		unmarshal func([]byte, *DiscovererPayload) error
		marshal   func(DiscovererPayload) ([]byte, error)
		assertOut func(*testing.T, string)
		wantType  string
	}{
		"json": {
			input: `{"docker":{"address":"unix:///var/run/docker.sock","timeout":"5s"}}`,
			unmarshal: func(data []byte, p *DiscovererPayload) error {
				return json.Unmarshal(data, p)
			},
			marshal: func(p DiscovererPayload) ([]byte, error) {
				return json.Marshal(p)
			},
			wantType: "docker",
			assertOut: func(t *testing.T, out string) {
				assert.JSONEq(t, `{"docker":{"address":"unix:///var/run/docker.sock","timeout":"5s"}}`, out)
			},
		},
		"yaml": {
			input: "docker:\n  address: unix:///var/run/docker.sock\n  timeout: 5s\n",
			unmarshal: func(data []byte, p *DiscovererPayload) error {
				return yaml.Unmarshal(data, p)
			},
			marshal: func(p DiscovererPayload) ([]byte, error) {
				return yaml.Marshal(p)
			},
			wantType: "docker",
			assertOut: func(t *testing.T, out string) {
				assert.Contains(t, out, "docker:")
				assert.Contains(t, out, "address: unix:///var/run/docker.sock")
				assert.Contains(t, out, "timeout: 5s")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var p DiscovererPayload
			require.NoError(t, tc.unmarshal([]byte(tc.input), &p))
			require.Equal(t, tc.wantType, p.Type())

			out, err := tc.marshal(p)
			require.NoError(t, err)
			tc.assertOut(t, string(out))
		})
	}
}

func TestDiscovererPayload_RejectsMultipleDiscoverers(t *testing.T) {
	tests := map[string]struct {
		input     string
		unmarshal func([]byte, *DiscovererPayload) error
	}{
		"json": {
			input: `{"docker":{},"snmp":{}}`,
			unmarshal: func(data []byte, p *DiscovererPayload) error {
				return json.Unmarshal(data, p)
			},
		},
		"yaml": {
			input: "docker: {}\nsnmp: {}\n",
			unmarshal: func(data []byte, p *DiscovererPayload) error {
				return yaml.Unmarshal(data, p)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var p DiscovererPayload
			err := tc.unmarshal([]byte(tc.input), &p)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "multiple discoverers configured")
		})
	}
}

func TestConfig_UnmarshalYAMLLegacyDiscover(t *testing.T) {
	tests := map[string]struct {
		input          string
		wantErr        bool
		wantErrContain []string
		assertCfg      func(*testing.T, Config)
	}{
		"k8s merge": {
			input: `
name: test-k8s
discover:
  - discoverer: k8s
    k8s:
      - role: pod
        namespaces:
          - default
  - discoverer: k8s
    k8s:
      - role: service
services:
  - id: "test-rule"
    match: "true"
`,
			assertCfg: func(t *testing.T, cfg Config) {
				require.Equal(t, "k8s", cfg.Discoverer.Type())

				var got []map[string]any
				require.NoError(t, json.Unmarshal(cfg.Discoverer.Config, &got))
				require.Len(t, got, 2)
				assert.Equal(t, "pod", got[0]["role"])
				assert.Equal(t, "service", got[1]["role"])
			},
		},
		"missing discoverer config fails": {
			input: `
name: test-invalid
discover:
  - discoverer: docker
    snmp: {}
services:
  - id: "test-rule"
    match: "true"
`,
			wantErr:        true,
			wantErrContain: []string{"missing config for discoverer", "docker"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			err := yaml.Unmarshal([]byte(tc.input), &cfg)

			if tc.wantErr {
				require.Error(t, err)
				for _, s := range tc.wantErrContain {
					assert.Contains(t, err.Error(), s)
				}
				return
			}

			require.NoError(t, err)
			if tc.assertCfg != nil {
				tc.assertCfg(t, cfg)
			}
		})
	}
}
