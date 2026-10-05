// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureGeo struct{ calls int }

func (g *captureGeo) Lookup(string) (string, string, float64, float64, bool) {
	g.calls++
	return "GR", "Example City", 37.9, 23.7, true
}

func TestCaptureGeolocationAtReceiver(t *testing.T) {
	for _, mode := range []string{"", config.GeolocationOff, config.GeolocationCountry, config.GeolocationCity} {
		t.Run("mode="+mode, func(t *testing.T) {
			cfg := testCfg()
			if mode != "" {
				cfg.Sites[0].Capture = &config.Capture{Geolocation: new(mode)}
			}
			sink, geo := newRecSink(), &captureGeo{}
			handler := newFixture(cfg, sink, geo).Handler()
			req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(faroBody))
			req.RemoteAddr = "198.51.100.20:12345"
			req.Header.Set("Origin", "https://shop.example.com")
			req.Header.Set("User-Agent", browserUA)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, http.StatusAccepted, response.Code)
			require.Len(t, sink.ingested, 1)
			b := sink.ingested[0]
			if mode == config.GeolocationOff {
				assert.Zero(t, geo.calls)
				assert.Empty(t, b.Country)
			} else {
				assert.Equal(t, 1, geo.calls)
				assert.Equal(t, "GR", b.Country)
			}
			if mode == config.GeolocationCity {
				assert.Equal(t, "Example City", b.City)
				assert.Equal(t, float64(37.9), b.Lat)
				assert.Equal(t, float64(23.7), b.Lon)
				assert.True(t, b.HasGeo)
			} else {
				assert.Empty(t, b.City)
				assert.Zero(t, b.Lat)
				assert.Zero(t, b.Lon)
				assert.False(t, b.HasGeo)
			}
		})
	}
}

func TestCaptureCredentialRedactionPreservesProtocolSemantics(t *testing.T) {
	body := `{"meta":{"session":{"id":"session"},"page":{"id":"document-1","url":"https://shop.example.com/"}},"events":[
	 {"name":"faro.performance.navigation","attributes":{"observation_id":"navigation-1","observation_sequence":"2","pageLoadTime":"1000","domContentLoadHandlerTime":"10"}},
	 {"name":"faro.performance.resource","attributes":{"observation_id":"resource-1","observation_sequence":"3","name":"app.js?private=value#fragment","httpHost":"cdn.example.com","duration":"1000","transferSize":"1000","initiatorType":"fetch"}},
	 {"name":"faro.tracing.fetch","attributes":{"url.full":"https://user:password@api.example.com/api?private=value#fragment","http.request.method":"POST","http.response.status_code":"200","duration_ns":"1000"}},
	 {"name":"rage_click","attributes":{"target":"button#pay"}}
	]}`
	for _, token := range []string{"document-1", "navigation-1", "resource-1", "observation_id", "observation_sequence", "unrelated-token", "resource", "duration", "1000", "rage_click", "url.full"} {
		t.Run(token, func(t *testing.T) {
			cfg := testCfg()
			cfg.Sites[0].EventLogs = &config.EventLogs{Destination: config.Destination{AuthToken: token}}
			sink := newRecSink()
			handler := newFixture(cfg, sink, nil).Handler()
			req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(body))
			req.Header.Set("Origin", "https://shop.example.com")
			req.Header.Set("User-Agent", browserUA)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, http.StatusAccepted, response.Code)
			require.Len(t, sink.ingested, 1)
			b := sink.ingested[0]
			assert.Equal(t, "document-1", b.ExperienceID)
			require.NotNil(t, b.Navigation)
			assert.EqualValues(t, 2, b.Navigation.Revision)
			assert.Equal(t, float64(1000), b.Navigation.LoadMS)
			require.Len(t, b.Resources, 1)
			assert.Equal(t, "resource-1", b.Resources[0].ID)
			assert.True(t, b.Resources[0].HasDuration)
			assert.Equal(t, float64(1000), b.Resources[0].DurationMS)
			assert.Equal(t, float64(1000), b.Resources[0].TransferB)
			require.Len(t, b.Events, 3, "disabled frustration must not become a generic event")
			assert.Equal(t, "app.js", b.Events[1].Attrs["name"])
			assert.Equal(t, "https://api.example.com/api", b.Events[2].Attrs["url.full"])
			assert.Equal(t, "1000", b.Events[2].Attrs["duration_ns"])
		})
	}
}
