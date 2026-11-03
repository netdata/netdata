// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"compress/gzip"
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

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var trafficStart = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

func TestTrafficMethods(t *testing.T) {
	var calls []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
		switch {
		case strings.HasSuffix(r.URL.Path, "/switch/ports/usage/history/byDevice/byInterval"):
			assert.Equal(
				t,
				url.Values{
					"perPage": {"50"},
					"t0":      {"2026-10-01T10:00:00Z"},
					"t1":      {"2026-10-01T10:05:00Z"},
				},
				r.URL.Query(),
			)
			fmt.Fprint(
				w,
				`{"items":[{"serial":"SWITCH","network":{"id":"NETWORK"},"ports":[{"portId":"1_C9300-NM-8X_2","intervals":[{"startTs":"start","endTs":"end","bandwidth":{"usage":{"upstream":1.25,"downstream":0}}}]}]}],"meta":{}}`,
			)
		case strings.HasSuffix(r.URL.Path, "/switch/ports/statuses/packets"):
			assert.Equal(t, url.Values{
				"timespan": {"300"},
			}, r.URL.Query())
			assert.Equal(t, "/api/v1/devices/opaque%2Fserial/switch/ports/statuses/packets", r.URL.EscapedPath())
			fmt.Fprint(
				w,
				`[{"portId":"1","packets":[{"desc":"Total","sent":999999,"ratePerSec":{"sent":2.5,"recv":null}}]}]`,
			)
		case strings.HasSuffix(r.URL.Path, "/appliance/uplinks/usage/byNetwork"):
			assert.Equal(t, url.Values{
				"t0": {"2026-10-01T10:00:00Z"},
				"t1": {"2026-10-01T10:05:00Z"},
			}, r.URL.Query())
			fmt.Fprint(
				w,
				`[{"networkId":"NETWORK","byUplink":[{"interface":"wan1","serial":"ignored","sent":0,"received":null}]}]`,
			)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := clientForServer(t, server)
	rows, err := c.ListSwitchPortUsageHistory(
		context.Background(),
		"ORG",
		trafficStart,
		trafficStart.Add(5*time.Minute),
	)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "SWITCH", rows[0].Serial)
	assert.Equal(t, "NETWORK", rows[0].Network.ID)
	require.Len(t, rows[0].Ports, 1)
	assert.Equal(t, "1_C9300-NM-8X_2", rows[0].Ports[0].PortID)
	interval := rows[0].Ports[0].Intervals[0]
	assert.Equal(t, "start", interval.Start)
	assert.Equal(t, "end", interval.End)
	require.NotNil(t, interval.Bandwidth.Usage.Upstream)
	assert.Equal(t, 1.25, *interval.Bandwidth.Usage.Upstream)
	require.NotNil(t, interval.Bandwidth.Usage.Downstream)
	assert.Zero(t, *interval.Bandwidth.Usage.Downstream)
	packets, err := c.ListSwitchPortPackets(context.Background(), "opaque/serial", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, packets, 1)
	require.Len(t, packets[0].Categories, 1)
	assert.Equal(t, "Total", packets[0].Categories[0].Description)
	assert.Equal(t, 2.5, *packets[0].Categories[0].RatePerSec.Sent)
	assert.Nil(t, packets[0].Categories[0].RatePerSec.Received)
	uplinks, err := c.ListApplianceUplinkUsage(
		context.Background(),
		"ORG",
		trafficStart,
		trafficStart.Add(5*time.Minute),
	)
	require.NoError(t, err)
	require.Len(t, uplinks, 1)
	assert.Equal(t, "NETWORK", uplinks[0].NetworkID)
	assert.Equal(t, "wan1", uplinks[0].ByUplink[0].Interface)
	require.NotNil(t, uplinks[0].ByUplink[0].Sent)
	assert.Zero(t, *uplinks[0].ByUplink[0].Sent)
	assert.Nil(t, uplinks[0].ByUplink[0].Received)
	assert.Len(t, calls, 3)
}

func TestSwitchUsagePagination(t *testing.T) {
	for name, query := range map[string]string{
		"restore window":    "startingAfter=next",
		"equivalent window": "startingAfter=next&t0=2026-10-01T10%3A00%3A00.000Z&t1=2026-10-01T10%3A05%3A00%2B00%3A00",
		"fixed interval":    "startingAfter=next&interval=300&perPage=3",
		"changed t0":        "startingAfter=next&t0=changed",
		"changed t1":        "startingAfter=next&t1=changed",
		"duplicate":         "startingAfter=next&t0=a&t0=b",
		"unknown":           "startingAfter=next&api_key=SECRET",
		"large page":        "startingAfter=next&perPage=51",
		"bad interval":      "startingAfter=next&interval=1200",
		"changed path":      "changed-path",
		"cycle":             "perPage=50",
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					link := "?" + query
					if name == "changed path" {
						link = "/api/v1/organizations/OTHER/switch/ports/usage/history/byDevice/byInterval"
					}
					w.Header().Set("Link", "<"+link+">; rel=\"next\"")
					fmt.Fprint(w, `{"items":[{"serial":"FIRST"}]}`)
					return
				}
				assert.Equal(t, "2026-10-01T10:00:00Z", r.URL.Query().Get("t0"))
				assert.Equal(t, "2026-10-01T10:05:00Z", r.URL.Query().Get("t1"))
				assert.Empty(t, r.URL.Query().Get("interval"))
				fmt.Fprint(w, `{"items":[{"serial":"SECOND"}]}`)
			}))
			defer server.Close()
			rows, err := clientForServer(
				t,
				server,
			).ListSwitchPortUsageHistory(context.Background(), "ORG", trafficStart, trafficStart.Add(5*time.Minute))
			if name == "restore window" || name == "fixed interval" || name == "equivalent window" {
				require.NoError(t, err)
				require.Len(t, rows, 2)
				assert.Equal(t, "FIRST", rows[0].Serial)
				assert.Equal(t, "SECOND", rows[1].Serial)
				assert.EqualValues(t, 2, calls.Load())
			} else {
				require.Error(t, err)
				assert.Nil(t, rows)
				assert.NotContains(t, err.Error(), "SECRET")
				assert.EqualValues(t, 1, calls.Load())
			}
		})
	}
}

func TestTrafficDecodeFailures(t *testing.T) {
	for _, envelope := range []bool{false, true} {
		bodies := []string{`null`, `{}`, `[null]`, `[`, `[] {}`, `[] SECRET`}
		if envelope {
			bodies = []string{
				`null`,
				`{}`,
				`{"items":null}`,
				`{"items":[null]}`,
				`{"items":{}}`,
				`{"items":[]} {}`,
				`{"items":[]} SECRET`,
				`[]`,
				`{"items":[}`,
			}
		}
		for _, body := range bodies {
			t.Run(fmt.Sprintf("envelope=%v/%s", envelope, body), func(t *testing.T) {
				server := httptest.NewTLSServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }),
				)
				defer server.Close()
				c := clientForServer(t, server)
				var err error
				if envelope {
					rows, e := c.ListSwitchPortUsageHistory(
						context.Background(),
						"ORG",
						trafficStart,
						trafficStart.Add(5*time.Minute),
					)
					assert.Nil(t, rows)
					err = e
				} else {
					rows, e := c.ListSwitchPortPackets(context.Background(), "SWITCH", 5*time.Minute)
					assert.Nil(t, rows)
					err = e
				}
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "SECRET")
			})
		}
	}
}

func TestTrafficPathIDsAndCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `[]`) }),
	)
	defer server.Close()
	c := clientForServer(t, server)
	for _, id := range []string{"", ".", ".."} {
		_, err := c.ListSwitchPortPackets(context.Background(), id, 5*time.Minute)
		require.Error(t, err)
		_, err = c.ListSwitchPortUsageHistory(context.Background(), id, trafficStart, trafficStart.Add(5*time.Minute))
		require.Error(t, err)
		_, err = c.ListApplianceUplinkUsage(context.Background(), id, trafficStart, trafficStart.Add(5*time.Minute))
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.ListSwitchPortPackets(ctx, "SWITCH", 5*time.Minute)
	assert.True(t, errors.Is(err, context.Canceled))
	_, err = c.ListSwitchPortUsageHistory(ctx, "ORG", trafficStart, trafficStart.Add(5*time.Minute))
	assert.True(t, errors.Is(err, context.Canceled))
	_, err = c.ListApplianceUplinkUsage(ctx, "ORG", trafficStart, trafficStart.Add(5*time.Minute))
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Zero(t, calls.Load())
}

func TestSwitchUsageLatePageFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Link", `<?startingAfter=next>; rel="next"`)
			fmt.Fprint(w, `{"items":[{"serial":"FIRST"}]}`)
			return
		}
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `SECRET`)
	}))
	defer server.Close()
	rows, err := clientForServer(
		t,
		server,
	).ListSwitchPortUsageHistory(context.Background(), "ORG", trafficStart, trafficStart.Add(5*time.Minute))
	assert.Nil(t, rows)
	var httpErr *merakiapi.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.NotContains(t, err.Error(), "SECRET")
}

func TestSwitchUsageEnvelopeHTTPPolicy(t *testing.T) {
	for _, mode := range []string{"gzip empty", "body limit", "unsupported encoding", "redirect", "redirect mutation", "cancel in flight"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
				if mode == "cancel in flight" {
					<-r.Context().Done()
					return
				}
				if (mode == "redirect" || mode == "redirect mutation") && n == 1 {
					target := *r.URL
					if mode == "redirect mutation" {
						q := target.Query()
						q.Set("t0", "changed")
						target.RawQuery = q.Encode()
					}
					w.Header().Set("Location", target.String())
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
				if mode == "gzip empty" {
					w.Header().Set("Content-Encoding", "gzip")
					z := gzip.NewWriter(w)
					fmt.Fprint(z, `{"items":[]}`)
					_ = z.Close()
					return
				}
				if mode == "unsupported encoding" {
					w.Header().Set("Content-Encoding", "br")
				}
				if mode == "body limit" {
					fmt.Fprintf(w, `{"items":[],"padding":"%s"}`, strings.Repeat("x", 128))
					return
				}
				fmt.Fprint(w, `{"items":[]}`)
			}))
			defer server.Close()
			hc := web.NewHTTPClientWithTransport(
				web.ClientConfig{
					ResponseBodyLimit: confopt.ByteSize(64),
				},
				server.Client().Transport,
			)
			c, err := merakiapi.New(merakiapi.Config{
				BaseURL: server.URL + "/api/v1",
				APIKey:  syntheticKey,
			}, hc)
			require.NoError(t, err)
			ctx := context.Background()
			if mode == "cancel in flight" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			rows, err := c.ListSwitchPortUsageHistory(ctx, "ORG", trafficStart, trafficStart.Add(5*time.Minute))
			switch mode {
			case "gzip empty", "redirect":
				require.NoError(t, err)
				require.NotNil(t, rows)
				assert.Empty(t, rows)
			case "body limit":
				require.ErrorIs(t, err, web.ErrResponseTooLarge)
				assert.Nil(t, rows)
			case "cancel in flight":
				require.ErrorIs(t, err, context.DeadlineExceeded)
				assert.Nil(t, rows)
			default:
				require.Error(t, err)
				assert.Nil(t, rows)
			}
		})
	}
}
