// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	for _, tc := range []struct {
		field string
		value any
		valid bool
	}{
		{"timeout", 0.000999, false}, {"timeout", 0.001, true},
		{"url", "/relative", false}, {"url", "ftp://example.org/", false},
		{"url", "https://user:password@example.org/", false}, {"url", "https://user@example.org/", false},
		{"url", "http://:80/", false}, {"url", "https://example.org/", true},
		{"url", "http://127.0.0.1:8000/?value=@home", true}, {"url", "https://[::1]/", true},
	} {
		t.Run(tc.field+"/"+fmt.Sprint(tc.value), func(t *testing.T) {
			cfg := map[string]any{"name": "test", "url": "https://example.org/"}
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
