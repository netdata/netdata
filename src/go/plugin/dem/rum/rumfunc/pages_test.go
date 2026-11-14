// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureFunctionSource struct {
	metadataFunctionSource
	known bool
	count int
}

func (s captureFunctionSource) Pages(context.Context, string, string) ([]query.Page, error) {
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
			handler := rumfunc.New(captureFunctionSource{
				known: tc.known,
				count: tc.count,
			}, nil)
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:      "rum-pages",
				Permissions: "0x1b",
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

type measurementSource struct {
	metadataFunctionSource
	page query.Page
}

func (s measurementSource) Pages(context.Context, string, string) ([]query.Page, error) {
	return []query.Page{s.page}, nil
}

func TestPagePopulationCountsAndAbsentPercentiles(t *testing.T) {
	for name, tc := range map[string]struct {
		n    int
		want any
	}{
		"absent": {0, nil}, "zero": {1, float64(0)}, "large population": {15001, float64(0)},
	} {
		t.Run(name, func(t *testing.T) {
			handler := rumfunc.New(measurementSource{
				page: query.Page{
					Site:      "shop",
					Page:      "/entry",
					Sessions:  2,
					VnodeGUID: "12345678-1234-4234-8234-123456789abc",
					Vitals:    map[string]aggregate.VitalStats{beacon.CLS: {N: tc.n}},
				},
			}, nil)
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:      "rum-pages",
				Permissions: "0x1b",
			}).RawResponse
			require.NotNil(t, response)
			rows := response["data"].([][]any)
			columns := response["columns"].(map[string]any)
			value := func(key string) any { return rows[0][columns[key].(map[string]any)["index"].(int)] }
			require.Equal(t, tc.want, value("cls_p75"))
			require.Equal(t, tc.n, value("cls_samples"))
			require.Equal(t, 2, value("sessions"))
			require.Equal(t, "12345678-1234-4234-8234-123456789abc", value("vnode_guid"))
			require.Contains(t, response["help"], "outside chart top-N")
			require.Contains(t, response["help"], "not people or concurrency")
		})
	}
}

func TestPageActivityCountsAndOptionalFrustration(t *testing.T) {
	for name, tc := range map[string]struct {
		count                 int
		capture               bool
		activity, frustration any
	}{
		"observed activity": {4, true, 4, 4},
		"quiet window":      {0, true, 0, 0},
		"capture disabled":  {4, false, 4, nil},
	} {
		t.Run(name, func(t *testing.T) {
			handler := rumfunc.New(measurementSource{
				page: query.Page{
					Site:              "shop",
					Page:              "/entry",
					PageviewsWindow:   tc.count,
					ErrorsWindow:      tc.count,
					FrustrationWindow: tc.count,
					FrustrationsKnown: tc.capture,
				},
			}, nil)
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:      "rum-pages",
				Permissions: "0x1b",
			}).RawResponse
			require.NotNil(t, response)
			rows := response["data"].([][]any)
			require.Len(t, rows, 1)
			columns := response["columns"].(map[string]any)
			got := map[string]any{}
			for _, key := range []string{"pageviews_window", "errors_window", "frustration_window"} {
				got[key] = rows[0][columns[key].(map[string]any)["index"].(int)]
			}
			require.Equal(t, map[string]any{"pageviews_window": tc.activity, "errors_window": tc.activity, "frustration_window": tc.frustration}, got)
		})
	}
}
