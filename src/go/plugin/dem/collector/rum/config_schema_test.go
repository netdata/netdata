// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportFormAndRuntimeAddressValidation(t *testing.T) {
	raw, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var bundle map[string]any
	require.NoError(t, json.Unmarshal(raw, &bundle))
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	require.NoError(t, compiler.AddResource("rum.json", bundle["jsonSchema"]))
	schema, err := compiler.Compile("rum.json")
	require.NoError(t, err)
	for _, tc := range []struct {
		address string
		valid   bool
	}{
		{"https://[:::]:4317", false},
		{"https://[1:2:3:4:5:6:7]:4317", false},
		{"https://[1:2:3:4:5:6:7:8:9]:4317", false},
		{"https://[::ffff:192.0.2.999]:4317", false},
		{"https://[::ffff:192.0.2.01]:4317", false},
		{"https://[::1%25eth0]:4317", false},
		{"https://[::1]:0", false},
		{"https://[::1]:65536", false},
		{"https://[::1]:4317", true},
		{"http://[2001:db8::1]:4317", true},
		{"https://[1:2:3:4:5:6:7:8]:4317", true},
		{"https://[::ffff:192.0.2.1]:4317", true},
		{"https://collector.example.:4317", true},
	} {
		for _, field := range []string{"event_logs", "tracing", "propagate_to"} {
			for _, enabled := range []bool{false, true} {
				t.Run(
					field+"/"+tc.address+map[bool]string{false: "/disabled", true: "/enabled"}[enabled],
					func(t *testing.T) {
						feature := map[string]any{
							"enabled":     enabled,
							"destination": map[string]any{"endpoint": tc.address},
						}
						key := field
						if field == "propagate_to" {
							key = "tracing"
							feature = map[string]any{"enabled": enabled, "propagate_to": []any{tc.address}}
						}
						input := map[string]any{
							"name":            "shop",
							"allowed_origins": []any{"https://shop.example.org"},
							key:               feature,
						}
						valid := tc.valid || !enabled
						assert.Equal(t, valid, schema.Validate(input) == nil, "form")
						raw, err := json.Marshal(input)
						require.NoError(t, err)
						var site config.Site
						require.NoError(t, json.Unmarshal(raw, &site))
						assert.Equal(t, valid, len(config.ValidateSiteExtras(site)) == 0, "runtime")
					},
				)
			}
		}
	}
}
