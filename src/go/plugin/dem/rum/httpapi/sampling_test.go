// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type samplingGeo struct{ calls atomic.Int64 }

func (g *samplingGeo) Lookup(string) (string, string, float64, float64, bool) {
	g.calls.Add(1)
	return "GR", "Athens", 38, 23.7, true
}

func samplingConfig(t *testing.T, rate float64) *fixtureConfig {
	t.Helper()
	cfg := testCfg()
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"measure_sample_rate":%g}`, rate)), &cfg.Sites[0]))
	return cfg
}

func TestSamplingZeroDiscardsBeforeDecodeAndProcessing(t *testing.T) {
	// Malformed data proves that disabled collection does not run the decoder.
	// Both a stale SDK beacon and a custom producer receive a quiet success.
	for name, body := range map[string]string{"stale page": faroBody, "custom malformed payload": "not JSON"} {
		t.Run(name, func(t *testing.T) {
			sink, geo := newRecSink(), &samplingGeo{}
			_, ts := newTestServer(t, samplingConfig(t, 0), sink, geo)
			resp := post(t, ts, "/rum/shop/collect", body, map[string]string{
				"Origin": "https://shop.example.com", "X-Forwarded-For": "8.8.8.8",
			})
			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
			assert.Equal(t, "https://shop.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
			accepted, rejected := sink.snapshot()
			assert.Zero(t, accepted)
			assert.Empty(t, rejected)
			assert.Zero(t, geo.calls.Load())
		})
	}
}

func TestSamplingZeroPreservesAdmissionPolicies(t *testing.T) {
	for name, tc := range map[string]struct {
		origin  string
		ua      string
		body    string
		chunked bool
		status  int
		reject  string
	}{
		"origin":       {origin: "https://other.example.net", body: faroBody, status: http.StatusForbidden, reject: beacon.RejectOrigin},
		"known size":   {body: strings.Repeat("x", 2049), status: http.StatusRequestEntityTooLarge, reject: beacon.RejectSize},
		"unknown size": {body: strings.Repeat("x", 2049), chunked: true, status: http.StatusRequestEntityTooLarge, reject: beacon.RejectSize},
		"bot":          {ua: "Googlebot", body: faroBody, status: http.StatusNoContent, reject: beacon.RejectBot},
	} {
		t.Run(name, func(t *testing.T) {
			sink, geo := newRecSink(), &samplingGeo{}
			_, ts := newTestServer(t, samplingConfig(t, 0), sink, geo)
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/rum/shop/collect", strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Origin", "https://shop.example.com")
			req.Header.Set("User-Agent", browserUA)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.ua != "" {
				req.Header.Set("User-Agent", tc.ua)
			}
			if tc.chunked {
				req.ContentLength = -1
			}
			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, tc.status, resp.StatusCode)
			accepted, rejected := sink.snapshot()
			assert.Zero(t, accepted)
			assert.Equal(t, map[string]int{"shop/" + tc.reject: 1}, rejected)
			assert.Zero(t, geo.calls.Load())
		})
	}
}

func TestSamplingZeroPreservesRateLimit(t *testing.T) {
	cfg := samplingConfig(t, 0)
	cfg.RateLimit.PerIPPerMin = 1
	sink := newRecSink()
	s, ts := newTestServer(t, cfg, sink, nil)
	s.now = func() time.Time { return time.Unix(100, 0) }
	for _, status := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": "https://shop.example.com"})
		assert.Equal(t, status, resp.StatusCode)
	}
	accepted, rejected := sink.snapshot()
	assert.Zero(t, accepted)
	assert.Equal(t, map[string]int{"shop/" + beacon.RejectRate: 1}, rejected)
}

func TestSamplingPositiveRatesDoNotResampleReceivedBeacons(t *testing.T) {
	for name, rate := range map[string]float64{"fraction": 0.000001, "full": 1} {
		t.Run(name, func(t *testing.T) {
			cfg := samplingConfig(t, rate)
			cfg.RateLimit.PerIPPerMin = 1000
			sink, geo := newRecSink(), &samplingGeo{}
			_, ts := newTestServer(t, cfg, sink, geo)
			for i := 0; i < 20; i++ {
				resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{
					"Origin": "https://shop.example.com", "X-Forwarded-For": "8.8.8.8",
				})
				assert.Equal(t, http.StatusAccepted, resp.StatusCode)
			}
			accepted, rejected := sink.snapshot()
			assert.Equal(t, 20, accepted)
			assert.Empty(t, rejected)
			assert.EqualValues(t, 20, geo.calls.Load())
		})
	}
}
