// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			handler := rumfunc.New(measurementSiteSource{
				serving: tc.serving,
				rate:    tc.rate,
			}, nil)
			response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:      "rum-sites",
				Permissions: "0x1b",
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

func TestSiteActivityCountsAndDetailPopulation(t *testing.T) {
	handler := rumfunc.New(measurementSiteSource{
		serving: true,
		rate:    1,
	}, nil)
	response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method:      "rum-sites",
		Permissions: "0x1b",
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

func TestSitesGeoIPSourceEvidence(t *testing.T) {
	path := "../../../../../crates/netflow-plugin/testdata/mmdb/GeoLite2-City-Test.mmdb"
	resolver := geoip.New(path, geoip.Paths{})
	t.Cleanup(resolver.Close)
	require.NoError(t, resolver.Refresh())
	hub := registry.New()
	handler := rumfunc.New(query.New(hub, nil), nil)
	collector := func() map[string]any {
		response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
			Method:      "rum-sites",
			Permissions: "0x1b",
		}).RawResponse
		data, err := json.Marshal(response["collector"])
		require.NoError(t, err)
		assert.NotContains(t, string(data), path)
		assert.NotContains(t, string(data), "81.2.69.142")
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		return decoded
	}
	initial := collector()["geoip"].(map[string]any)
	assert.Equal(t, "unavailable", initial["state"])
	assert.NotContains(t, initial, "selection")
	publication := hub.PublishReceiver(registry.Availability{
		Serving:   true,
		PublicURL: "https://rum.example",
	})
	publication.SetGeoIP(resolver.Status())
	active := collector()
	assert.Equal(t, true, active["ingress_available"])
	evidence := active["geoip"].(map[string]any)
	assert.Equal(t, "explicit", evidence["selection"])
	assert.Equal(t, "loaded", evidence["state"])
	assert.Equal(t, "GeoLite2-City", evidence["database_type"])
	assert.Equal(t, float64(resolver.Status().BuildAt), evidence["build_at"])
	assert.Equal(t, float64(resolver.Status().LoadedAt), evidence["loaded_at"])
	assert.Equal(t, float64(resolver.Status().LastCheckedAt), evidence["last_checked_at"])
	assert.Equal(t, float64(0), evidence["lookup_errors"])
	publication.Close()
	retired := collector()
	assert.Equal(t, false, retired["ingress_available"])
	evidence = retired["geoip"].(map[string]any)
	assert.Equal(t, "unavailable", evidence["state"])
	assert.NotContains(t, evidence, "source")
	assert.NotContains(t, evidence, "database_type")
	assert.NotContains(t, evidence, "loaded_at")
}

type onboardingSource struct {
	metadataFunctionSource
	site query.Site
}

func (s onboardingSource) Sites(context.Context, string) ([]query.Site, error) {
	return []query.Site{s.site}, nil
}

func TestSiteFunctionPublishesIndependentFacts(t *testing.T) {
	for _, received := range []bool{false, true} {
		site := query.Site{
			Name:              "shop",
			Generation:        "receiving-runtime",
			CollectionEnabled: true,
			Activity: query.Activity{
				LastBeaconAgeS: -1,
			},
		}
		if received {
			site.Activity.LastBeaconAt = time.Unix(1234, 0)
			site.Activity.LastBeaconAgeS = 7200
			site.ScriptURL = "https://receiver.example/a&b/rum/shop.js"
		}
		response := rumfunc.New(onboardingSource{
			site: site,
		}, nil).HandleRaw(context.Background(), funcapi.RawMethodRequest{
			Method:      "rum-sites",
			Permissions: "0x1b",
		}).RawResponse
		require.NotNil(t, response)
		columns := response["columns"].(map[string]any)
		rows := response["data"].([][]any)
		value := func(key string) any { return rows[0][columns[key].(map[string]any)["index"].(int)] }
		require.Equal(t, "receiving-runtime", value("generation"))
		require.Equal(t, 1, value("collection_enabled"))
		for _, key := range []string{"status", "reachable", "reach_detail", "snippet_check", "snippet_detail", "beacon_url"} {
			require.NotContains(t, columns, key)
		}
		require.Nil(t, value("last_rejected_at"))
		require.Nil(t, value("last_rejected_age_s"))
		for _, key := range []string{"last_beacon_at", "last_rejected_at"} {
			require.Equal(t, "datetime_usec", columns[key].(map[string]any)["value_options"].(map[string]any)["transform"])
		}
		if received {
			require.Equal(t, int64(1234000000), value("last_beacon_at"))
			require.Equal(t, 7200, value("last_beacon_age_s"))
			require.Equal(t, `<script async src="https://receiver.example/a&amp;b/rum/shop.js"></script>`, value("snippet"))
		} else {
			require.Nil(t, value("last_beacon_at"))
			require.Nil(t, value("last_beacon_age_s"))
			require.Nil(t, value("script_url"))
			require.Nil(t, value("snippet"))
		}
	}
}
