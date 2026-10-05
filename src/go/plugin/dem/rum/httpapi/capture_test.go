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
