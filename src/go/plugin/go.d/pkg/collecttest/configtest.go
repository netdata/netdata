// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type configurationProvider interface {
	Configuration() any
}

func TestConfigurationSerialize(t *testing.T, mod configurationProvider, cfgJSON, cfgYAML []byte) {
	t.Helper()
	tests := map[string]struct {
		config    []byte
		unmarshal func(in []byte, out any) error
		marshal   func(in any) ([]byte, error)
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
