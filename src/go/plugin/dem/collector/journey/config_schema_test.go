// SPDX-License-Identifier: GPL-3.0-or-later
package journey

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestFormsRejectInvalidWorkflowInputs(t *testing.T) {
	raw, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var bundle map[string]any
	require.NoError(t, json.Unmarshal(raw, &bundle))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("config.json", bundle["jsonSchema"]))
	schema, err := compiler.Compile("config.json")
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "entry.ts")
	require.NoError(t, os.WriteFile(file, []byte("export {};"), 0600))
	require.NoError(t, schema.Validate(map[string]any{"name": "test", "script": "", "script_path": file}))
	c := configured()
	c.Script = ""
	c.ScriptPath = file
	require.NoError(t, c.Init(context.Background()))
	for _, tc := range []struct {
		name        string
		trace       any
		screenshots bool
		form, valid bool
	}{
		{"off", false, false, true, true},
		{"enabled", true, true, true, true},
		{"needs screenshots", true, false, false, false},
		// Like other scalar booleans, null leaves the Go default off but is not a form value.
		{"null", nil, false, false, true},
	} {
		t.Run("trace/"+tc.name, func(t *testing.T) {
			c := configured()
			c.ScreenshotOnFailure = tc.screenshots
			raw, err := json.Marshal(c.Configuration())
			require.NoError(t, err)
			var input map[string]any
			require.NoError(t, json.Unmarshal(raw, &input))
			input["trace_on_failure"] = tc.trace
			require.Equal(t, tc.form, schema.Validate(input) == nil)
			raw, err = json.Marshal(input)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, c))
			copy := New(Dependencies{})
			serialized, err := json.Marshal(c.Configuration()) // Before Init.
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(serialized, copy))
			require.Equal(t, c.TraceOnFailure, copy.TraceOnFailure)
			require.Equal(t, tc.valid, c.Init(context.Background()) == nil)
		})
	}
	for _, tc := range []struct {
		field string
		value any
		valid bool
	}{
		{"browser_error_summary", true, true}, {"browser_error_summary", false, true},
		{"check_every", 1, true}, {"check_every", 0, false}, {"check_every", -1, false},
		{"check_every", int64(9223372036), true}, {"check_every", int64(9223372037), false},
		{"timeout", 0.000999, false}, {"timeout", 0.001, true},
		{"script", " \t\n", false}, {"script", "\u0085", false}, {"script", "\u00a0", false}, {"script", "\u2003", false},
	} {
		t.Run(tc.field+"/"+fmt.Sprint(tc.value), func(t *testing.T) {
			cfg := map[string]any{"name": "test", "script": "export {};"}
			cfg[tc.field] = tc.value
			err := schema.Validate(cfg)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			c := configured()
			err = json.Unmarshal(raw, c)
			if value, ok := tc.value.(int64); ok && int64(int(value)) != value {
				require.Error(t, err) // The collector config uses int on 32-bit platforms.
				return
			}
			require.NoError(t, err)
			err = c.Init(context.Background())
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
