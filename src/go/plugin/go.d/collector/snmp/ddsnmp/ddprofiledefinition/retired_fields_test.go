// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestSymbolConfig_IgnoresRetiredConstantField(t *testing.T) {
	for name, tc := range map[string]struct {
		data   string
		decode func([]byte, any) error
	}{
		"YAML true": {data: `OID: 1.2.3.0
name: real
constant_value_one: true`, decode: yaml.Unmarshal},
		"YAML untyped value": {data: `OID: 1.2.3.0
name: real
constant_value_one: [ignored]`, decode: yaml.Unmarshal},
		"JSON true":          {data: `{"OID":"1.2.3.0","name":"real","constant_value_one":true}`, decode: json.Unmarshal},
		"JSON untyped value": {data: `{"OID":"1.2.3.0","name":"real","constant_value_one":["ignored"]}`, decode: json.Unmarshal},
	} {
		t.Run(name, func(t *testing.T) {
			var symbol SymbolConfig
			require.NoError(t, tc.decode([]byte(tc.data), &symbol))
			assert.Equal(t, SymbolConfig{
				OID:  "1.2.3.0",
				Name: "real",
			}, symbol)
		})
	}
}
