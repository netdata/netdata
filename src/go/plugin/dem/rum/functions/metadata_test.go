// SPDX-License-Identifier: GPL-3.0-or-later
package functions_test

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

func TestMetadataDocumentsProcessFunctions(t *testing.T) {
	data, err := os.ReadFile("../../collector/rum/metadata.yaml")
	require.NoError(t, err)
	module, err := collecttest.DecodeMetadataModule(data, "")
	require.NoError(t, err)
	declarations := rumfunctions.Declarations()
	require.Len(t, module.Functions, len(declarations))
	documented := make(map[string]collecttest.MetadataFunction)
	for _, function := range module.Functions {
		require.NotContains(t, documented, function.ID)
		documented[function.ID] = function
	}
	handler := rumfunctions.New(metadataFunctionSource{})
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

// These fixtures supply domain observations; the production handler builds each table.
type metadataFunctionSource struct{}

func (metadataFunctionSource) Receiver() query.Receiver {
	return query.Receiver{
		Serving: true,
	}
}
func (metadataFunctionSource) Sites(context.Context) ([]query.Site, error) {
	return []query.Site{
		{Name: "shop", Label: "shop", Sampling: query.Sampling{
			MeasureRate:     1,
			InvestigateRate: 1,
		}},
	}, nil
}
func (metadataFunctionSource) Pages(context.Context, string) ([]query.Page, error) {
	return []query.Page{{Site: "shop", Page: "/"}}, nil
}
func (metadataFunctionSource) Live(context.Context, string, string, int) ([]query.LiveEvent, string, error) {
	return nil, "", nil
}
func (metadataFunctionSource) Sessions(context.Context, string, int64, int64) ([]query.Session, error) {
	return []query.Session{{Site: "shop", SessionID: "visit"}}, nil
}
func (metadataFunctionSource) SessionEvents(context.Context, string, string) ([]query.SessionEvent, error) {
	return []query.SessionEvent{{Site: "shop", SessionID: "visit", Type: "pageview", Page: "/"}}, nil
}
func (metadataFunctionSource) Errors(context.Context, string, string, int64, int64) ([]query.ErrorGroup, error) {
	return nil, nil
}
