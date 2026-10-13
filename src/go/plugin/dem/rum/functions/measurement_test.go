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

func (s measurementSource) Pages(context.Context, string) ([]query.Page, error) {
	return []query.Page{s.page}, nil
}

func TestPagePopulationCountsAndIncompleteNulls(t *testing.T) {
	for name, tc := range map[string]struct {
		n    int
		lost uint64
		want any
	}{
		"absent": {0, 0, nil}, "zero": {1, 0, float64(0)}, "capacity loss": {2, 1, nil},
	} {
		t.Run(name, func(t *testing.T) {
			handler := rumfunctions.New(measurementSource{
				page: query.Page{
					Site:         "shop",
					Page:         "/entry",
					Sessions:     2,
					SessionsLost: tc.lost,
					Lost:         tc.lost,
					Vitals:       map[string]aggregate.VitalStats{beacon.CLS: {N: tc.n, Lost: tc.lost}},
				},
			})
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "rum-pages",
			}).RawResponse
			require.NotNil(t, response)
			rows := response["data"].([][]any)
			columns := response["columns"].(map[string]any)
			value := func(key string) any { return rows[0][columns[key].(map[string]any)["index"].(int)] }
			require.Equal(t, tc.want, value("cls_p75"))
			require.Equal(t, tc.n, value("cls_samples"))
			require.Equal(t, tc.lost, value("cls_lost"))
			require.Equal(t, tc.lost, value("window_lost"))
			if tc.lost != 0 {
				require.Nil(t, value("sessions"))
			} else {
				require.Equal(t, 2, value("sessions"))
			}
			require.Contains(t, response["help"], "outside chart top-N")
			require.Contains(t, response["help"], "not people or concurrency")
		})
	}
}

type measurementSiteSource struct {
	metadataFunctionSource
	serving    bool
	rate       float64
	lost       uint64
	windowLost uint64
}

func (s measurementSiteSource) Receiver() query.Receiver {
	return query.Receiver{
		Serving: s.serving,
	}
}
func (s measurementSiteSource) Sites(context.Context) ([]query.Site, error) {
	return []query.Site{{Name: "shop", Sampling: query.Sampling{
		MeasureRate: s.rate,
	}, Activity: query.Activity{
		ObservedSessions:       3,
		InvestigatedSessions:   7,
		PageviewsWindow:        2,
		ApplicationViewsWindow: 4,
		SessionsLost:           s.lost,
		WindowLost:             s.windowLost,
		JSErrorsWindow:         6,
	}}}, nil
}
func TestSiteMeasurementAvailabilityAndDetailPopulation(t *testing.T) {
	for name, tc := range map[string]struct {
		serving                    bool
		rate                       float64
		lost                       uint64
		sessions, documents, views any
	}{
		"measured":             {true, 1, 0, 3, 2, uint64(4)},
		"session capacity":     {true, 1, 1, nil, 2, uint64(4)},
		"receiver unavailable": {false, 1, 0, nil, nil, nil},
		"measurement disabled": {true, 0, 0, nil, nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			handler := rumfunctions.New(measurementSiteSource{
				serving: tc.serving,
				rate:    tc.rate,
				lost:    tc.lost,
			})
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

func TestPageActivityCountsAreUnavailableWhenPopulationIncomplete(t *testing.T) {
	for name, tc := range map[string]struct {
		count                 int
		lost                  uint64
		capture               bool
		activity, frustration any
	}{
		"complete activity":       {4, 0, true, 4, 4},
		"complete quiet window":   {0, 0, true, 0, 0},
		"partial activity":        {4, 1, true, nil, nil},
		"partial quiet remainder": {0, 1, true, nil, nil},
		"capture disabled":        {4, 0, false, 4, nil},
	} {
		t.Run(name, func(t *testing.T) {
			handler := rumfunctions.New(measurementSource{
				page: query.Page{
					Site:              "shop",
					Page:              "/entry",
					PageviewsWindow:   tc.count,
					ErrorsWindow:      tc.count,
					FrustrationWindow: tc.count,
					Lost:              tc.lost,
					FrustrationsKnown: tc.capture,
				},
			})
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method: "rum-pages",
			}).RawResponse
			require.NotNil(t, response)
			rows := response["data"].([][]any)
			require.Len(t, rows, 1)
			columns := response["columns"].(map[string]any)
			got := map[string]any{}
			for _, key := range []string{"pageviews_window", "errors_window", "frustration_window", "window_lost"} {
				got[key] = rows[0][columns[key].(map[string]any)["index"].(int)]
			}
			require.Equal(t, map[string]any{"pageviews_window": tc.activity, "errors_window": tc.activity, "frustration_window": tc.frustration, "window_lost": tc.lost}, got)
		})
	}
}

func TestSiteActivityCountsAreUnavailableWhenPopulationIncomplete(t *testing.T) {
	handler := rumfunctions.New(measurementSiteSource{
		serving:    true,
		rate:       1,
		windowLost: 1,
	})
	response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method: "rum-sites",
	}).RawResponse
	require.NotNil(t, response)
	rows := response["data"].([][]any)
	require.Len(t, rows, 1)
	columns := response["columns"].(map[string]any)
	got := map[string]any{}
	for _, key := range []string{"pageviews_window", "js_errors_window", "application_views_window", "window_lost", "observed_sessions", "investigated_sessions"} {
		got[key] = rows[0][columns[key].(map[string]any)["index"].(int)]
	}
	require.Equal(t, map[string]any{
		"pageviews_window": nil, "js_errors_window": nil, "application_views_window": nil, "window_lost": uint64(1),
		"observed_sessions": 3, "investigated_sessions": 7,
	}, got, "activity loss does not change independently complete session populations")
}
