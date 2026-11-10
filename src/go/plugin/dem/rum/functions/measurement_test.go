// SPDX-License-Identifier: GPL-3.0-or-later
package functions_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/stretchr/testify/require"
)

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
			handler := rumfunctions.New(measurementSource{
				page: query.Page{
					Site:      "shop",
					Page:      "/entry",
					Sessions:  2,
					VnodeGUID: "12345678-1234-4234-8234-123456789abc",
					Vitals:    map[string]aggregate.VitalStats{beacon.CLS: {N: tc.n}},
				},
			}, nil)
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "rum-pages",
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

type measurementSiteSource struct {
	metadataFunctionSource
	serving bool
	rate    float64
}

func (s measurementSiteSource) Receiver() query.Receiver {
	return query.Receiver{
		Serving: s.serving,
	}
}
func (s measurementSiteSource) Sites(context.Context, string) ([]query.Site, error) {
	return []query.Site{{Name: "shop", Sampling: query.Sampling{
		MeasureRate: s.rate,
	}, Activity: query.Activity{
		ObservedSessions:       3,
		InvestigatedSessions:   7,
		PageviewsWindow:        2,
		ApplicationViewsWindow: 4,
		JSErrorsWindow:         6,
	}}}, nil
}
func TestSiteMeasurementAvailabilityAndDetailPopulation(t *testing.T) {
	for name, tc := range map[string]struct {
		serving                    bool
		rate                       float64
		sessions, documents, views any
	}{
		"measured":             {true, 1, 3, 2, uint64(4)},
		"receiver unavailable": {false, 1, nil, nil, nil},
		"measurement disabled": {true, 0, nil, nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			handler := rumfunctions.New(measurementSiteSource{
				serving: tc.serving,
				rate:    tc.rate,
			}, nil)
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "rum-sites",
			}).RawResponse
			require.NotNil(t, response)
			rows := response["data"].([][]any)
			cols := response["columns"].(map[string]any)
			value := func(key string) any { return rows[0][cols[key].(map[string]any)["index"].(int)] }
			require.Equal(t, tc.sessions, value("observed_sessions"))
			require.Equal(t, tc.documents, value("pageviews_window"))
			require.Equal(t, tc.views, value("application_views_window"))
			require.Equal(t, 7, value("investigated_sessions"), "bounded detail cache is a distinct population")
			require.Contains(t, response["help"], "not people or concurrent visitors")
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
			handler := rumfunctions.New(measurementSource{
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
				Method: "rum-pages",
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

func TestSiteActivityCountsAndDetailPopulation(t *testing.T) {
	handler := rumfunctions.New(measurementSiteSource{
		serving: true,
		rate:    1,
	}, nil)
	response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method: "rum-sites",
	}).RawResponse
	require.NotNil(t, response)
	rows := response["data"].([][]any)
	require.Len(t, rows, 1)
	columns := response["columns"].(map[string]any)
	got := map[string]any{}
	for _, key := range []string{"pageviews_window", "js_errors_window", "application_views_window", "observed_sessions", "investigated_sessions"} {
		got[key] = rows[0][columns[key].(map[string]any)["index"].(int)]
	}
	require.Equal(t, map[string]any{
		"pageviews_window": 2, "js_errors_window": uint64(6), "application_views_window": uint64(4),
		"observed_sessions": 3, "investigated_sessions": 7,
	}, got, "measurement counts remain distinct from bounded investigation detail")
}
