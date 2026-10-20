// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/snmpauth"
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
	t.Run("authored get roundtrip", func(t *testing.T) {
		config := Config{VirtualNode: VirtualNode{Name: "router"}, Mode: "snmp", ModeSNMP: &SNMPConfig{
			Address: "device", Timeout: confopt.Duration(5 * time.Second),
			Config: snmpauth.Config{Version: "2c", Credentials: &snmpauth.Community{Community: "fixture"}},
		}}
		raw, err := json.Marshal(config)
		require.NoError(t, err)
		var form any
		require.NoError(t, json.Unmarshal(raw, &form))
		require.NoError(t, schema.Validate(form), "authored get must be editable in its schema")
	})
	for _, level := range []string{"authNoPriv", "authPriv"} {
		t.Run("multibyte password/"+level, func(t *testing.T) {
			c := Config{VirtualNode: VirtualNode{Name: "router", SourceType: "user"}, Mode: "snmp", ModeSNMP: &SNMPConfig{
				Address: "device",
				Config:  snmpauth.Config{Version: "3", Credentials3: &snmpauth.USM{Username: "fixture", SecurityLevel: level, AuthPassword: "éééé", PrivPassword: "éééé"}},
			}}
			c.NormalizeCredentials()
			require.NoError(t, c.Validate())
			raw, err := json.Marshal(c)
			require.NoError(t, err)
			var form any
			require.NoError(t, json.Unmarshal(raw, &form))
			require.NoError(t, schema.Validate(form), "valid byte-length passwords must remain editable")
		})
	}
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
