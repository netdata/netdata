// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestVnodeSchemaModesAndCredentials(t *testing.T) {
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(ConfigSchemaFor(true)), &doc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("vnode.json", doc["jsonSchema"]))
	schema, err := compiler.Compile("vnode.json")
	require.NoError(t, err)
	for _, tc := range []struct {
		config string
		valid  bool
	}{
		{`{"guid":"6e17cd2c-0518-4e94-965b-d25673decb21"}`, true},
		{`{}`, false},
		{`{"mode":"static"}`, false},
		{`{"mode":"snmp","mode_snmp":{"address":"device","version":"2c","credentials":{"community":"fixture"}}}`, true},
		{`{"mode":"snmp","mode_snmp":{"address":"device","version":"3","credentials3":{"username":"fixture","security_level":"noAuthNoPriv"}}}`, true},
		{`{"mode":"snmp","mode_snmp":{"address":"device","version":"3","credentials3":{"username":"fixture","security_level":"authPriv"}}}`, false},
		{`{"mode":"snmp","mode_snmp":{"address":"device","version":"2c"}}`, false},
		{`{"mode":"snmp"}`, false},
	} {
		var config any
		require.NoError(t, json.Unmarshal([]byte(tc.config), &config))
		if tc.valid {
			require.NoError(t, schema.Validate(config), tc.config)
		} else {
			require.Error(t, schema.Validate(config), tc.config)
		}
	}
	static := ConfigSchemaFor(false)
	require.NotContains(t, static, "mode_snmp")
	require.NotContains(t, static, "credentials")
	require.NotContains(t, static, "SNMP")
}
