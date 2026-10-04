// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rumfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestMetadataDocumentsProcessFunctions(t *testing.T) {
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	module, err := collecttest.DecodeMetadataModule(data, "")
	require.NoError(t, err)
	declarations := rumfunc.Declarations()
	require.Len(t, module.Functions, len(declarations))
	documented := make(map[string]collecttest.MetadataFunction)
	for _, function := range module.Functions {
		require.NotContains(t, documented, function.ID)
		documented[function.ID] = function
	}
	handler := rumfunc.New(metadataFunctionSource{})
	for _, declaration := range declarations {
		t.Run(declaration.ID, func(t *testing.T) {
			doc, ok := documented[declaration.ID]
			require.True(t, ok, "missing Function documentation")
			require.Equal(t, funcapi.FunctionName("rum", declaration), doc.FunctionName)
			params := make([]string, 0, len(doc.Parameters))
			for _, param := range doc.Parameters {
				params = append(params, param.ID)
			}
			require.ElementsMatch(t, declaration.AcceptedParams, params)
			require.NotContains(t, params, collecttest.FunctionJobParameter)
			var args []string
			for _, parameter := range declaration.AcceptedParams {
				switch parameter {
				case "site":
					args = append(args, "site:shop")
				case "session_id":
					args = append(args, "session_id:visit")
				}
			}
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: declaration.ID,
				Args:   args,
			})
			require.NotNil(t, response.RawResponse)
			require.Equal(t, 200, response.RawResponse["status"])
			columns, ok := response.RawResponse["columns"].(map[string]any)
			require.True(t, ok)
			actual, err := collecttest.FunctionResponseColumns(&funcapi.FunctionResponse{
				Columns: columns,
			})
			require.NoError(t, err)
			require.Equal(t, actual, doc.Columns)
		})
	}
}

func TestMetadataDocumentsOptionalDisplayLabel(t *testing.T) {
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	var document struct {
		Modules []struct {
			Metrics struct {
				Scopes []struct {
					Name   string `yaml:"name"`
					Labels []struct {
						Name        string `yaml:"name"`
						Description string `yaml:"description"`
					} `yaml:"labels"`
				} `yaml:"scopes"`
			} `yaml:"metrics"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &document))
	require.Len(t, document.Modules, 1)
	for _, scope := range document.Modules[0].Metrics.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			found := false
			for _, label := range scope.Labels {
				if label.Name == "display_name" {
					found = true
					require.Contains(t, label.Description, "Optional")
				}
			}
			require.True(t, found, "display_name applies to every site chart scope")
		})
	}
}

// These fixtures supply domain observations; the production handler builds each table.
type metadataFunctionSource struct{}

func (metadataFunctionSource) Receiver() runtimehub.Availability {
	return runtimehub.Availability{
		Serving: true,
	}
}
func (metadataFunctionSource) Sites(context.Context) ([]rumfunc.Observation, error) {
	return []rumfunc.Observation{{Config: config.Site{
		Name: "shop",
	}}}, nil
}
func (metadataFunctionSource) Pages(context.Context, string) ([]agg.PageInfo, error) {
	return []agg.PageInfo{{Site: "shop", Page: "/"}}, nil
}
func (metadataFunctionSource) Live(context.Context, string, string, int) ([]rumfunc.LiveEvent, string, error) {
	return nil, "", nil
}
func (metadataFunctionSource) Sessions(context.Context, string, int64, int64) ([]store.RumSessionRecord, error) {
	return []store.RumSessionRecord{{Site: "shop", SessionID: "visit"}}, nil
}
func (metadataFunctionSource) SessionEvents(context.Context, string, string) ([]store.RumSessionEventRecord, error) {
	return []store.RumSessionEventRecord{{Site: "shop", SessionID: "visit", Type: "pageview", Page: "/"}}, nil
}
func (metadataFunctionSource) Errors(context.Context, string, string, int64, int64) ([]store.RumErrorAgg, error) {
	return nil, nil
}
