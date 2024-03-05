// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Module is an interface that represents a module.
type Module interface {
	// Init does initialization.
	// If it returns error, the job will be disabled.
	Init() error

	// Check is called after Init.
	// If it returns error, the job will be disabled.
	Check() error

	// Charts returns the chart definition.
	Charts() *Charts

	// Collect collects metrics.
	Collect() map[string]int64

	// Cleanup Cleanup
	Cleanup()

	GetBase() *Base

	Configuration() any
}

// Base is a helper struct. All modules should embed this struct.
type Base struct {
	*logger.Logger
}

func (b *Base) GetBase() *Base { return b }

func TestConfigurationSerialize(t *testing.T, mod Module, cfgJSON, cfgYAML []byte) {
	t.Helper()
	tests := map[string]struct {
		config    []byte
		unmarshal func(in []byte, out interface{}) (err error)
		marshal   func(in interface{}) (out []byte, err error)
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
