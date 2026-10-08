// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	registry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/require"
)

func TestSiteInstallURLUsesOnlyExplicitConfiguration(t *testing.T) {
	for _, tc := range []struct{ site, receiver, want string }{
		{"", "", ""},
		{"", "https://receiver.example/prefix/", "https://receiver.example/prefix/rum/shop.js"},
		{"https://site.example/other/", "https://receiver.example", "https://site.example/other/rum/shop.js"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			hub := registry.New()
			revoke := hub.PublishReceiver(registry.Availability{
				Serving:   true,
				PublicURL: tc.receiver,
			})
			defer revoke()
			cfg := config.Site{
				Name:      "shop",
				PublicURL: tc.site,
			}
			a := aggregate.New(time.Minute, aggregate.SiteCfg{
				Name: "shop",
			})
			state := diagnostics.New()
			retire, err := hub.Register("shop", &registry.Site{
				Route:       httpapi.NewRoute(cfg, measurementProcessor{a}, state),
				Diagnostics: state,
				Aggregator:  a,
				Generation:  "runtime",
			})
			require.NoError(t, err)
			defer retire()
			rows, err := query.New(hub, nil).Sites(context.Background())
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, tc.want, rows[0].ScriptURL)
			require.True(t, rows[0].CollectionEnabled)
			require.Equal(t, "runtime", rows[0].Generation)
		})
	}
}

func TestSiteEvidencePersistsAfterAcceptanceAndResetsWithGeneration(t *testing.T) {
	hub := registry.New()
	_, retire := addSite(t, hub, "shop", "first")
	receiver := config.Receiver{
		MaxBodyBytes: 262144,
		RateLimit: config.RateLimit{
			PerIPPerMin:   120,
			PerSitePerSec: 500,
		},
	}
	handler := httpapi.New(&receiver, hub, nil).Handler()
	request := httptest.NewRequest(http.MethodOptions, "/rum/shop/collect", nil)
	request.Header.Set("Origin", "https://wrong.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
	request = httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(`{"meta":{"page":{"id":"document","url":"https://example.org/accepted"},"session":{"id":"session"}},"events":[{"name":"document_activated","attributes":{"observation_id":"document","observation_sequence":"1"}}]}`))
	request.Header.Set("Origin", "https://example.org")
	request.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0.0.0 Safari/537.36")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusAccepted, response.Code)
	service := query.New(hub, nil)
	rows, err := service.Sites(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://wrong.example", rows[0].Rejected.Origin)
	require.False(t, rows[0].Rejected.At.IsZero())
	require.False(t, rows[0].Activity.LastBeaconAt.IsZero())
	require.Equal(t, 1, rows[0].Activity.BeaconsPerMin)
	retire()
	addSite(t, hub, "shop", "second")
	rows, err = service.Sites(context.Background())
	require.NoError(t, err)
	require.Equal(t, "second", rows[0].Generation)
	require.True(t, rows[0].Rejected.At.IsZero())
	require.True(t, rows[0].Activity.LastBeaconAt.IsZero())
	require.Equal(t, -1, rows[0].Activity.LastBeaconAgeS)
}
