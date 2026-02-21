// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Module is an interface that represents a module.
type Module interface {
	// Init does initialization.
	// If it returns error, the job will be disabled.
	Init(context.Context) error

	// Check is called after Init.
	// If it returns error, the job will be disabled.
	Check(context.Context) error

	// Charts returns the chart definition.
	Charts() *Charts

	// Collect collects metrics.
	Collect(context.Context) map[string]int64

	// Cleanup Cleanup
	Cleanup(context.Context)

	GetBase() *Base

	Configuration() any

	VirtualNode() *vnodes.VirtualNode
}

type MetricCollector interface {
	CollectMetrics(context.Context) map[string]float64
}

// ModuleV2 is the collector contract for the new metrics+template runtime.
//
// Collectors implementing this interface:
//   - write metrics into CollectorStore during Collect(),
//   - provide chart template YAML consumed by chartengine.
type ModuleV2 interface {
	Init(context.Context) error
	Check(context.Context) error
	Collect(context.Context) error
	Cleanup(context.Context)

	GetBase() *Base
	Configuration() any
	VirtualNode() *vnodes.VirtualNode

	MetricStore() metrix.CollectorStore
	ChartTemplateYAML() string
}

// ModuleV2EnginePolicy allows a V2 collector to provide chartengine policy
// (series selector + autogen behavior).
type ModuleV2EnginePolicy interface {
	EnginePolicy() chartengine.EnginePolicy
}

// Base is a helper struct. All modules should embed this struct.
type Base struct {
	*logger.Logger
}

func (b *Base) GetBase() *Base { return b }

func (b *Base) VirtualNode() *vnodes.VirtualNode { return nil }

type configurationProvider interface {
	Configuration() any
}

func TestConfigurationSerialize(t *testing.T, mod configurationProvider, cfgJSON, cfgYAML []byte) {
	t.Helper()
	tests := map[string]struct {
		config    []byte
		unmarshal func(in []byte, out any) (err error)
		marshal   func(in any) (out []byte, err error)
	}{
		"json": {config: cfgJSON, marshal: json.Marshal, unmarshal: json.Unmarshal},
		"yaml": {config: cfgYAML, marshal: yaml.Marshal, unmarshal: yaml.Unmarshal},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {

			require.NoError(t, test.unmarshal(test.config, mod), "unmarshal test->mod")
			bs, err := test.marshal(mod.Configuration())
			require.NoError(t, err, "marshal mod config")

			var want map[string]any
			var got map[string]any

			require.NoError(t, test.unmarshal(test.config, &want), "unmarshal test->map")
			require.NoError(t, test.unmarshal(bs, &got), "unmarshal mod->map")

			require.NotNil(t, want, "want map")
			require.NotNil(t, got, "got map")

			assert.Equal(t, want, got)
		})
	}
}
