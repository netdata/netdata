// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
)

const testDiscovererSchema = `{"jsonSchema":{"type":"object"}}`

func testDiscovererRegistry() Registry {
	return NewRegistry(
		NewDescriptor(
			testDiscovererTypeNetListeners,
			testDiscovererSchema,
			parseJSONTestConfig[testNetListenersConfig],
			newTestDiscoverers[testNetListenersConfig],
		),
		NewDescriptor(
			testDiscovererTypeDocker,
			testDiscovererSchema,
			parseJSONTestConfig[testDockerConfig],
			newTestDiscoverers[testDockerConfig],
		),
		NewDescriptor(
			testDiscovererTypeK8s,
			testDiscovererSchema,
			parseJSONTestConfig[[]testK8sConfig],
			newTestDiscoverers[[]testK8sConfig],
		),
		NewDescriptor(
			testDiscovererTypeSNMP,
			testDiscovererSchema,
			parseJSONTestConfig[testSNMPConfig],
			newTestDiscoverers[testSNMPConfig],
		),
	)
}

func parseJSONTestConfig[T any](raw json.RawMessage) (T, error) {
	var cfg T
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func newTestDiscoverers[T any](_ T, _ string) ([]model.Discoverer, error) {
	return nil, nil
}
