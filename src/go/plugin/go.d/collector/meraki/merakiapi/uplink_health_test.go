// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUplinkHealthMethods(t *testing.T) {
	start := time.Date(2026, 11, 5, 10, 0, 0, 123, time.FixedZone("offset", 3600))
	end := start.Add(5 * time.Minute)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
		if call == 1 {
			assert.Equal(t, "/api/v1/organizations/opaque%2Forg/appliance/uplink/statuses", r.URL.EscapedPath())
			assert.Equal(t, url.Values{
				"perPage": {"1000"},
			}, r.URL.Query())
			fmt.Fprint(w, `[{"serial":"MX-A","networkId":"NETWORK","model":"MX68C",`+
				`"lastReportedAt":"2026-11-05T09:00:00Z","highAvailability":{"enabled":true,"role":"primary"},`+
				`"uplinks":[{"interface":"wan3","status":"ready","ip":"192.0.2.1"},`+
				`{"interface":"cellular","status":"future-state"}]}]`)
			return
		}
		assert.Equal(t, "/api/v1/organizations/opaque%2Forg/devices/uplinksLossAndLatency", r.URL.EscapedPath())
		assert.Equal(t, url.Values{
			"t0": {"2026-11-05T09:00:00.000000123Z"},
			"t1": {"2026-11-05T09:05:00.000000123Z"},
		}, r.URL.Query())
		// This operation has no pagination, even if a response supplies a Link.
		w.Header().Set("Link", `<?startingAfter=unexpected>; rel="next"`)
		fmt.Fprint(w, `[{"serial":"MX-A","networkId":"NETWORK","uplink":"wan3","ip":"198.51.100.1","timeSeries":[`+
			`{"ts":"2026-11-05T09:04:00Z","lossPercent":0,"latencyMs":0},`+
			`{"ts":"2026-11-05T09:03:00Z","lossPercent":null,"latencyMs":12.25},`+
			`{"ts":"2026-11-05T09:02:00Z","lossPercent":5.3}]}]`)
	}))
	defer server.Close()
	c := clientForServer(t, server)

	statuses, err := c.ListApplianceUplinkStatuses(t.Context(), "opaque/org")
	require.NoError(t, err)
	assert.Equal(t, []merakiapi.ApplianceUplinkStatus{
		{
			Serial:         "MX-A",
			NetworkID:      "NETWORK",
			LastReportedAt: "2026-11-05T09:00:00Z",
			Uplinks: []merakiapi.ApplianceUplinkState{
				{
					Interface: "wan3",
					Status:    "ready",
				},
				{
					Interface: "cellular",
					Status:    "future-state",
				},
			},
		},
	}, statuses)

	rows, err := c.ListUplinkLossAndLatency(t.Context(), "opaque/org", start, end)
	require.NoError(t, err)
	assert.Equal(t, []merakiapi.UplinkLossAndLatency{
		{
			Serial:        "MX-A",
			NetworkID:     "NETWORK",
			Uplink:        "wan3",
			DestinationIP: "198.51.100.1",
			Samples: []merakiapi.UplinkHealthSample{
				{
					Timestamp:   "2026-11-05T09:04:00Z",
					LossPercent: new(0.0),
					LatencyMS:   new(0.0),
				},
				{
					Timestamp: "2026-11-05T09:03:00Z",
					LatencyMS: new(12.25),
				},
				{
					Timestamp:   "2026-11-05T09:02:00Z",
					LossPercent: new(5.3),
				},
			},
		},
	}, rows)
	assert.EqualValues(t, 2, calls.Load())
}

func TestUplinkStatusesPagination(t *testing.T) {
	tests := map[string]struct {
		link string
		ok   bool
	}{
		"server cursor":      {link: "?startingAfter=second", ok: true},
		"minimum page":       {link: "?startingAfter=second&perPage=3", ok: true},
		"maximum page":       {link: "?startingAfter=second&perPage=1000", ok: true},
		"oversize page":      {link: "?startingAfter=second&perPage=1001"},
		"undersize page":     {link: "?startingAfter=second&perPage=2"},
		"wrong organization": {link: "/api/v1/organizations/OTHER/appliance/uplink/statuses?startingAfter=second"},
		"wrong operation":    {link: "/api/v1/organizations/ORG/devices?startingAfter=second"},
		"unknown parameter":  {link: "?startingAfter=second&api_key=SECRET"},
		"unexpected window":  {link: "?startingAfter=second&t0=2026-11-05T09:00:00Z"},
		"duplicate cursor":   {link: "?startingAfter=second&startingAfter=third"},
		"wrong host": {
			link: "https://example.invalid/api/v1/organizations/ORG/appliance/uplink/statuses?startingAfter=second",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
				if calls.Add(1) == 1 {
					assert.Equal(t, "1000", r.URL.Query().Get("perPage"))
					w.Header().Set("Link", "<"+tc.link+`>; rel="next"`)
					fmt.Fprint(w, `[{"serial":"FIRST"}]`)
					return
				}
				assert.Equal(t, "second", r.URL.Query().Get("startingAfter"))
				fmt.Fprint(w, `[{"serial":"SECOND"}]`)
			}))
			defer server.Close()
			rows, err := clientForServer(t, server).ListApplianceUplinkStatuses(t.Context(), "ORG")
			if !tc.ok {
				require.Error(t, err)
				assert.Nil(t, rows)
				assert.NotContains(t, err.Error(), "SECRET")
				assert.EqualValues(t, 1, calls.Load())
				return
			}
			require.NoError(t, err)
			require.Len(t, rows, 2)
			assert.Equal(t, "FIRST", rows[0].Serial)
			assert.Equal(t, "SECOND", rows[1].Serial)
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestUplinkStatusesLatePageFailure(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Link", `<?startingAfter=second>; rel="next"`)
					fmt.Fprint(w, `[{"serial":"FIRST"}]`)
					return
				}
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
				fmt.Fprint(w, "SECRET")
			}))
			defer server.Close()
			rows, err := clientForServer(t, server).ListApplianceUplinkStatuses(t.Context(), "ORG")
			assert.Nil(t, rows)
			httpErr, ok := errors.AsType[*merakiapi.HTTPError](err)
			require.True(t, ok, "%v", err)
			assert.Equal(t, status, httpErr.StatusCode)
			assert.Equal(t, "getOrganizationApplianceUplinkStatuses", httpErr.Operation)
			assert.Equal(t, time.Minute, httpErr.RetryAfter)
			assert.NotContains(t, err.Error(), "SECRET")
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}

// These adapters exercise both public operation paths through the shared HTTP
// decoder and safety policy, without asserting private operation descriptors.
var healthReads = map[string]func(context.Context, *merakiapi.Client, string) (bool, error){
	"status": func(ctx context.Context, c *merakiapi.Client, id string) (bool, error) {
		rows, err := c.ListApplianceUplinkStatuses(ctx, id)
		return rows != nil, err
	},
	"loss and latency": func(ctx context.Context, c *merakiapi.Client, id string) (bool, error) {
		rows, err := c.ListUplinkLossAndLatency(ctx, id, trafficStart, trafficEnd)
		return rows != nil, err
	},
}

func TestUplinkHealthResponseFailures(t *testing.T) {
	tests := map[string]struct {
		body   string
		status int
		want   error
	}{
		"null": {body: `null`}, "object": {body: `{}`}, "null item": {body: `[null]`},
		"trailing response": {body: `[] SECRET`}, "invalid field": {body: `[{"uplinks":1,"timeSeries":1}]`},
		"oversized response": {body: `[]` + strings.Repeat(" ", 65), want: web.ErrResponseTooLarge},
		"forbidden":          {body: `SECRET`, status: http.StatusForbidden},
		"rate limited":       {body: `SECRET`, status: http.StatusTooManyRequests},
	}
	for operation, read := range healthReads {
		for name, tc := range tests {
			t.Run(operation+"/"+name, func(t *testing.T) {
				c, err := merakiapi.New(
					merakiapi.Config{
						APIKey: syntheticKey,
					},
					&http.Client{
						Transport: advertisedLimitTransport{
							limit: 64,
							roundTripFunc: func(r *http.Request) (*http.Response, error) {
								status := tc.status
								if status == 0 {
									status = http.StatusOK
								}
								return response(r, status, http.Header{
									"Retry-After": {"30"},
								}, tc.body), nil
							},
						},
					},
				)
				require.NoError(t, err)
				got, err := read(t.Context(), c, "ORG")
				require.Error(t, err)
				assert.False(t, got)
				assert.NotContains(t, err.Error(), "SECRET")
				if tc.want != nil {
					assert.ErrorIs(t, err, tc.want)
				}
				if tc.status != 0 {
					httpErr, ok := errors.AsType[*merakiapi.HTTPError](err)
					require.True(t, ok, "%v", err)
					assert.Equal(t, tc.status, httpErr.StatusCode)
					assert.Equal(t, 30*time.Second, httpErr.RetryAfter)
				}
			})
		}
	}
}

func TestUplinkHealthCancellationAndIDs(t *testing.T) {
	for operation, read := range healthReads {
		t.Run(operation, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `[]`) }),
			)
			defer server.Close()
			c := clientForServer(t, server)
			for _, id := range []string{"", ".", ".."} {
				got, err := read(t.Context(), c, id)
				require.Error(t, err)
				assert.False(t, got)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			got, err := read(ctx, c, "ORG")
			require.ErrorIs(t, err, context.Canceled)
			assert.False(t, got)
			assert.Zero(t, calls.Load())
		})
	}
}

func TestUplinkHealthRejectsCancellationAfterBufferedResponse(t *testing.T) {
	for operation, read := range healthReads {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			closed := false
			c, err := merakiapi.New(
				merakiapi.Config{
					APIKey: syntheticKey,
				},
				&http.Client{
					Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
						resp := response(r, http.StatusOK, nil, "")
						resp.Body = trackedBody{
							Reader: cancelAtEOF{
								Reader: strings.NewReader(`[]`),
								cancel: cancel,
							},
							closed: &closed,
						}
						return resp, nil
					}),
				},
			)
			require.NoError(t, err)
			got, err := read(ctx, c, "ORG")
			require.ErrorIs(t, err, context.Canceled)
			assert.False(t, got)
			assert.True(t, closed)
		})
	}
}

func TestUplinkHealthDeadlineDuringResponse(t *testing.T) {
	for operation, read := range healthReads {
		t.Run(operation, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `[`)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			got, err := read(ctx, clientForServer(t, server), "ORG")
			require.ErrorIs(t, err, context.DeadlineExceeded)
			assert.False(t, got)
		})
	}
}
