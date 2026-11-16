// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc_test

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/syntheticfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each integration documents the shared Functions and its own kind's run history; every
// declared Function is documented.
func TestIntegrationsDocumentTheirProcessFunctions(t *testing.T) {
	data, err := os.ReadFile("../../collector/synthetic/metadata.yaml")
	require.NoError(t, err)
	declarations := make(map[string]funcapi.FunctionConfig)
	for _, decl := range syntheticfunc.Declarations() {
		declarations[decl.ID] = decl
	}
	documented := make(map[string]bool)
	for id, history := range map[string]string{
		"collector-dem.plugin-journey":    "synthetics-journey-runs",
		"collector-dem.plugin-lighthouse": "synthetics-lighthouse-runs",
	} {
		t.Run(id, func(t *testing.T) {
			module, err := collecttest.DecodeMetadataModule(data, id)
			require.NoError(t, err)
			var ids []string
			for _, doc := range module.Functions {
				ids = append(ids, doc.ID)
			}
			assert.Equal(
				t,
				[]string{"synthetics-checks", history, "synthetics-run", "synthetics-artifacts", "synthetics-artifact"},
				ids,
			)
			h, _, _ := newRecordingHandler(nil)
			for _, doc := range module.Functions {
				t.Run(doc.ID, func(t *testing.T) {
					decl, exists := declarations[doc.ID]
					require.True(t, exists)
					documented[doc.ID] = true
					// Registration must match the handler's own 0x1b permission check.
					require.True(t, decl.RequireCloud)
					// Viewing browser tests (journeys, Lighthouse) requires a paid plan (access 0x0017).
					require.True(t, decl.RequireCommercial)
					assert.Equal(t, decl.FunctionName, doc.FunctionName)
					var args []string
					for _, param := range doc.Parameters {
						args = append(args, param.ID)
					}
					assert.ElementsMatch(t, decl.AcceptedParams, args)
					assert.NotContains(t, args, "__job")
					response := h.HandleRaw(context.Background(), funcapi.RawMethodRequest{
						Method:      decl.ID,
						Args:        requiredArgs(decl.ID),
						Permissions: "0x1b",
					})
					require.NotNil(t, response.RawResponse, response.Message)
					table, ok := response.RawResponse["columns"].(map[string]any)
					require.True(t, ok)
					columns, err := collecttest.FunctionResponseColumns(&funcapi.FunctionResponse{
						Columns: table,
					})
					require.NoError(t, err)
					assert.Equal(t, columns, doc.Columns)
				})
			}
		})
	}
	for id := range declarations {
		assert.True(t, documented[id], "%s is not documented", id)
	}
}
