// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	shared "github.com/netdata/netdata/go/plugins/plugin/dem/collector/synthetic"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

type prepared struct{}

func (prepared) Check(context.Context, model.Kind) error { return nil }
func (prepared) Execute(context.Context, model.Request, func(string)) model.Execution {
	panic("unexpected execution")
}

func TestFormsRejectInvalidWorkflowInputs(t *testing.T) {
	for _, kind := range []string{"journey", "lighthouse"} {
		t.Run(kind, func(t *testing.T) {
			raw, err := os.ReadFile("../" + kind + "/config_schema.json")
			require.NoError(t, err)
			var bundle map[string]any
			require.NoError(t, json.Unmarshal(raw, &bundle))
			compiler := jsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource("config.json", bundle["jsonSchema"]))
			schema, err := compiler.Compile("config.json")
			require.NoError(t, err)
			cases := []struct {
				field string
				value any
				valid bool
			}{
				{"timeout", 0.000999, false}, {"timeout", 0.001, true},
			}
			if kind == "journey" {
				for _, value := range []string{" \t\n", "\n", "\u0085", "\u00a0", "\u2003"} {
					cases = append(cases, struct {
						field string
						value any
						valid bool
					}{"script", value, false})
				}
			} else {
				for _, v := range []string{"/relative", "ftp://example.org/", "https://user:password@example.org/", "https://user@example.org/", "http://:80/"} {
					cases = append(cases, struct {
						field string
						value any
						valid bool
					}{"url", v, false})
				}
				for _, v := range []string{"https://example.org/", "http://127.0.0.1:8000/?value=@home", "https://[::1]/"} {
					cases = append(cases, struct {
						field string
						value any
						valid bool
					}{"url", v, true})
				}
			}
			if kind == "journey" {
				file := filepath.Join(t.TempDir(), "entry.ts")
				require.NoError(t, os.WriteFile(file, []byte("export {};"), 0600))
				cfg := map[string]any{"name": "test", "script": "", "script_path": file}
				require.NoError(t, schema.Validate(cfg))
				c := journey.New(shared.Dependencies{Executor: prepared{}, Hub: model.NewHub()})
				c.Name = "test"
				c.ScriptPath = file
				require.NoError(t, c.Init(context.Background()))
			}

			for _, tc := range cases {
				t.Run(tc.field+"/"+fmt.Sprint(tc.value), func(t *testing.T) {
					cfg := map[string]any{"name": "test"}
					if kind == "journey" {
						cfg["script"] = "export {};"
					} else {
						cfg["url"] = "https://example.org/"
					}
					cfg[tc.field] = tc.value
					err := schema.Validate(cfg)
					if tc.valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
					raw, err := json.Marshal(cfg)
					require.NoError(t, err)
					deps := shared.Dependencies{Executor: prepared{}, Hub: model.NewHub()}
					var collector interface{ Init(context.Context) error }
					if kind == "journey" {
						collector = journey.New(deps)
					} else {
						collector = lighthouse.New(deps)
					}
					require.NoError(t, json.Unmarshal(raw, collector))
					err = collector.Init(context.Background())
					if tc.valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}
}
