// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
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
		{"https://[::ffff:0192.0.2.1]:4317", false},
		{"https://[::ffff:192.00.2.1]:4317", false},
		{"https://[::ffff:192.0.02.1]:4317", false},
		{"https://[::1%25eth0]:4317", false},
		{"https://[::1]:0", false},
		{"https://[::1]:65536", false},
		{"https://[::1]:4317", true},
		{"http://[2001:db8::1]:4317", true},
		{"https://[1:2:3:4:5:6:7:8]:4317", true},
		{"https://[::ffff:192.0.2.1]:4317", true},
		{"https://[0:00:000:0000::0001]:4317", true},
		{"https://[::0000:192.0.2.1]:4317", true},
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
	for _, signal := range []string{"event_logs", "tracing"} {
		for name, destination := range map[string]any{"null": nil, "empty": map[string]any{}, "null endpoint": map[string]any{"endpoint": nil}} {
			t.Run(signal+"/"+name, func(t *testing.T) {
				input := map[string]any{"name": "shop", "allowed_origins": []any{"https://shop.example.org"},
					signal: map[string]any{"enabled": true, "destination": destination}}
				require.NoError(t, schema.Validate(input), "nullable destination fields use the local default")
			})
		}
	}

}

func TestSamplingFormAndRuntimeValidation(t *testing.T) {
	var bundle map[string]any
	require.NoError(t, json.Unmarshal([]byte(schema), &bundle))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("rum.json", bundle["jsonSchema"]))
	form, err := compiler.Compile("rum.json")
	require.NoError(t, err)
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, tc := range []struct {
		name   string
		fields string
		valid  bool
	}{
		{"omitted", ``, true},
		{"null", `,"measure_sample_rate":null,"investigate":null`, true},
		{"null fields", `,"investigate":{"sample_rate":null,"always_keep":null}`, true},
		{"zero", `,"measure_sample_rate":0,"investigate":{"sample_rate":0,"always_keep":[]}`, true},
		{"fraction", `,"measure_sample_rate":0.25,"investigate":{"sample_rate":0.25}`, true},
		{"one", `,"measure_sample_rate":1,"investigate":{"sample_rate":1}`, true},
		{"negative collection", `,"measure_sample_rate":-0.01`, false},
		{"large collection", `,"measure_sample_rate":1.01`, false},
		{"negative detail", `,"investigate":{"sample_rate":-0.01}`, false},
		{"large detail", `,"investigate":{"sample_rate":1.01}`, false},
		{"unknown override", `,"investigate":{"always_keep":["unknown"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"name":"shop","allowed_origins":["https://shop.example.org"]` + tc.fields + `}`)
			var input map[string]any
			require.NoError(t, json.Unmarshal(raw, &input))
			assert.Equal(t, tc.valid, form.Validate(input) == nil, "form")
			c := New(Dependencies{
				Registry: rumregistry.New(),
				History:  rumhistory.NewStore(db),
			})
			require.NoError(t, json.Unmarshal(raw, &c.Config))
			assert.Equal(t, tc.valid, c.Init(context.Background()) == nil, "runtime")
		})
	}
}

func TestCaptureFormAndRuntimeValidation(t *testing.T) {
	var bundle map[string]any
	require.NoError(t, json.Unmarshal([]byte(schema), &bundle))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("rum.json", bundle["jsonSchema"]))
	form, err := compiler.Compile("rum.json")
	require.NoError(t, err)
	for _, tc := range []struct {
		capture string
		valid   bool
	}{
		{`null`, true}, {`{}`, true}, {`{"geolocation":null,"frustration_signals":null}`, true},
		{`{"geolocation":"off"}`, true}, {`{"geolocation":"country"}`, true}, {`{"geolocation":"city","frustration_signals":true}`, true},
		{`{"geolocation":""}`, false}, {`{"geolocation":"precise"}`, false}, {`{"geolocation":1}`, false}, {`{"frustration_signals":"true"}`, false}, {`{"frustration_signals":1}`, false},
	} {
		t.Run(tc.capture, func(t *testing.T) {
			raw := []byte(`{"name":"shop","allowed_origins":["https://shop.example.org"],"capture":` + tc.capture + `}`)
			var input map[string]any
			require.NoError(t, json.Unmarshal(raw, &input))
			assert.Equal(t, tc.valid, form.Validate(input) == nil, "form")
			var site config.Site
			err := json.Unmarshal(raw, &site)
			valid := err == nil && len(config.ValidateSiteExtras(site)) == 0
			assert.Equal(t, tc.valid, valid, "runtime")
		})
	}
}
