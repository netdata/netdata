// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDiscovererPayload_JSONRoundTrip(t *testing.T) {
	input := `{"docker":{"address":"unix:///var/run/docker.sock","timeout":"5s"}}`

	var p DiscovererPayload
	require.NoError(t, json.Unmarshal([]byte(input), &p))
	require.Equal(t, "docker", p.Type())

	out, err := json.Marshal(p)
	require.NoError(t, err)
	assert.JSONEq(t, input, string(out))
}

func TestDiscovererPayload_YAMLRoundTrip(t *testing.T) {
	input := "docker:\n  address: unix:///var/run/docker.sock\n  timeout: 5s\n"

	var p DiscovererPayload
	require.NoError(t, yaml.Unmarshal([]byte(input), &p))
	require.Equal(t, "docker", p.Type())

	out, err := yaml.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(out), "docker:")
	assert.Contains(t, string(out), "address: unix:///var/run/docker.sock")
	assert.Contains(t, string(out), "timeout: 5s")
}

func TestDiscovererPayload_RejectsMultipleDiscoverersJSON(t *testing.T) {
	var p DiscovererPayload
	err := json.Unmarshal([]byte(`{"docker":{},"snmp":{}}`), &p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple discoverers configured")
}

func TestDiscovererPayload_RejectsMultipleDiscoverersYAML(t *testing.T) {
	var p DiscovererPayload
	err := yaml.Unmarshal([]byte("docker: {}\nsnmp: {}\n"), &p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple discoverers configured")
}

func TestConfig_UnmarshalYAMLLegacyDiscoverK8sMerge(t *testing.T) {
	input := `
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
`

	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(input), &cfg))
	require.Equal(t, "k8s", cfg.Discoverer.Type())

	var got []map[string]any
	require.NoError(t, json.Unmarshal(cfg.Discoverer.Config, &got))
	require.Len(t, got, 2)
	assert.Equal(t, "pod", got[0]["role"])
	assert.Equal(t, "service", got[1]["role"])
}

func TestConfig_UnmarshalYAMLLegacyDiscoverMissingConfigFails(t *testing.T) {
	input := `
name: test-invalid
discover:
  - discoverer: docker
    snmp: {}
services:
  - id: "test-rule"
    match: "true"
`

	var cfg Config
	err := yaml.Unmarshal([]byte(input), &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing config for discoverer")
	assert.Contains(t, err.Error(), "docker")
}
