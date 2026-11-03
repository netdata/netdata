// SPDX-License-Identifier: GPL-3.0-or-later

package functions_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureFunctionSource struct {
	metadataFunctionSource
	known bool
	count int
}

func (s captureFunctionSource) Pages(context.Context, string) ([]query.Page, error) {
	return []query.Page{{Site: "shop", Page: "/", FrustrationsKnown: s.known, FrustrationWindow: s.count}}, nil
}

func TestPageFrustrationDistinguishesDisabledFromMeasuredZero(t *testing.T) {
	for _, tc := range []struct {
		name  string
		known bool
		count int
		want  any
	}{
		{"disabled", false, 0, nil},
		{"enabled_without_signals", true, 0, 0},
		{"enabled_with_signals", true, 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := rumfunctions.New(captureFunctionSource{
				known: tc.known,
				count: tc.count,
			})
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "rum-pages",
			})
			require.NotNil(t, response.RawResponse)
			rows := response.RawResponse["data"].([][]any)
			require.Len(t, rows, 1)
			columns := response.RawResponse["columns"].(map[string]any)
			index := columns["frustration_window"].(map[string]any)["index"].(int)
			assert.Equal(t, tc.want, rows[0][index])
		})
	}
}
