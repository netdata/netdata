// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc_test

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/syntheticfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBothIntegrationsDocumentSharedProcessFunctions(t *testing.T) {
	for _, collector := range []string{"journey", "lighthouse"} {
		t.Run(collector, func(t *testing.T) {
			data, err := os.ReadFile("../collector/" + collector + "/metadata.yaml")
			require.NoError(t, err)
			module, err := collecttest.DecodeMetadataModule(data, "")
			require.NoError(t, err)
			declarations := syntheticfunc.Declarations()
			require.Len(t, module.Functions, len(declarations))
			docs := make(map[string]collecttest.MetadataFunction)
			for _, doc := range module.Functions {
				require.NotContains(t, docs, doc.ID)
				docs[doc.ID] = doc
			}
			h := syntheticfunc.New(nil)
			for _, decl := range declarations {
				t.Run(decl.ID, func(t *testing.T) {
					doc, exists := docs[decl.ID]
					require.True(t, exists)
					assert.Equal(t, decl.FunctionName, doc.FunctionName)
					var args []string
					for _, param := range doc.Parameters {
						args = append(args, param.ID)
					}
					assert.ElementsMatch(t, decl.AcceptedParams, args)
					assert.NotContains(t, args, "__job")
					info := h.HandleRaw(context.Background(), funcapi.RawMethodRequest{
						Method: decl.ID,
						Info:   true,
					})
					columns, err := collecttest.FunctionResponseColumns(info)
					require.NoError(t, err)
					assert.Equal(t, columns, doc.Columns)
				})
			}
		})
	}
}
