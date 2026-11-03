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
		field string
		value any
		valid bool
	}{
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
			require.NoError(t, json.Unmarshal(raw, c))
			err = c.Init(context.Background())
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
