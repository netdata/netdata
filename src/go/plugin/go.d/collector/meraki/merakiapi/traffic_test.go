// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
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

var (
	trafficStart = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	trafficEnd   = trafficStart.Add(5 * time.Minute)
)

func listUsage(ctx context.Context, c *merakiapi.Client) ([]merakiapi.SwitchPortUsageHistory, error) {
	return c.ListSwitchPortUsageHistory(ctx, "ORG", trafficStart, trafficEnd)
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

func TestTrafficMethods(t *testing.T) {
	type request struct {
		path  string
		query url.Values
	}
	var requests []request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, request{r.URL.EscapedPath(), r.URL.Query()})
		assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
		switch {
		case strings.HasSuffix(r.URL.Path, "/byInterval"):
			fmt.Fprint(w, `{"items":[{"serial":"SWITCH","network":{"id":"NETWORK"},"ports":[{"portId":"1",`+
				`"intervals":[{"startTs":"start","endTs":"end","bandwidth":{"usage":{"upstream":1.25,"downstream":0}}}]}]}],`+
				`"meta":{}}`)
		case strings.HasSuffix(r.URL.Path, "/packets"):
			fmt.Fprint(w, `[{"portId":"1","packets":[{"desc":"Total","sent":999999,"ratePerSec":{"sent":2.5,"recv":null}}]}]`)
		default:
			fmt.Fprint(w, `[{"networkId":"NETWORK","byUplink":[{"interface":"wan1","serial":"A","sent":0,"received":null}]}]`)
		}
	}))
	defer server.Close()
	c := clientForServer(t, server)

	usage, err := listUsage(t.Context(), c)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"serial":"SWITCH","network":{"id":"NETWORK"},"ports":[{"portId":"1","intervals":[`+
		`{"startTs":"start","endTs":"end","bandwidth":{"usage":{"upstream":1.25,"downstream":0}}}]}]}]`, asJSON(t, usage))

	packets, err := c.ListSwitchPortPackets(t.Context(), "opaque/serial", 5*time.Minute)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"portId":"1","packets":[{"desc":"Total","ratePerSec":{"sent":2.5,"recv":null}}]}]`,
		asJSON(t, packets))

	uplinks, err := c.ListApplianceUplinkUsage(t.Context(), "ORG", trafficStart, trafficEnd)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"networkId":"NETWORK","byUplink":[{"interface":"wan1","sent":0,"received":null}]}]`,
		asJSON(t, uplinks))

	window := url.Values{"t0": {"2026-10-01T10:00:00Z"}, "t1": {"2026-10-01T10:05:00Z"}}
	assert.Equal(t, []request{
		{
			path:  "/api/v1/organizations/ORG/switch/ports/usage/history/byDevice/byInterval",
			query: url.Values{"perPage": {"50"}, "t0": window["t0"], "t1": window["t1"]},
		},
		{
			path:  "/api/v1/devices/opaque%2Fserial/switch/ports/statuses/packets",
			query: url.Values{"timespan": {"300"}},
		},
		{
			path:  "/api/v1/organizations/ORG/appliance/uplinks/usage/byNetwork",
			query: window,
		},
	}, requests)
}

func TestSwitchUsagePagination(t *testing.T) {
	const otherPath = "/api/v1/organizations/OTHER/switch/ports/usage/history/byDevice/byInterval"
	tests := map[string]struct {
		link     string   // next link of the first page
		wantRows []string // serials read; nil when the link must be refused
	}{
		"window left out": {
			link:     "?startingAfter=next",
			wantRows: []string{"FIRST", "SECOND"},
		},
		"window spelled differently": {
			link:     "?startingAfter=next&t0=2026-10-01T10%3A00%3A00.000Z&t1=2026-10-01T10%3A05%3A00%2B00%3A00",
			wantRows: []string{"FIRST", "SECOND"},
		},
		"derived interval echoed": {
			link:     "?startingAfter=next&interval=300&perPage=3",
			wantRows: []string{"FIRST", "SECOND"},
		},
		"another interval echoed": {
			link:     "?startingAfter=next&interval=1200",
			wantRows: []string{"FIRST", "SECOND"},
		},
		"t0 moved":              {link: "?startingAfter=next&t0=2026-10-01T09%3A55%3A00Z"},
		"t1 moved":              {link: "?startingAfter=next&t1=2026-10-01T10%3A10%3A00Z"},
		"t0 unparsable":         {link: "?startingAfter=next&t0=changed"},
		"t0 repeated":           {link: "?startingAfter=next&t0=a&t0=b"},
		"undocumented param":    {link: "?startingAfter=next&api_key=SECRET"},
		"page size over 50":     {link: "?startingAfter=next&perPage=51"},
		"another operation":     {link: otherPath},
		"link back to the page": {link: "?perPage=50"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Link", "<"+tc.link+`>; rel="next"`)
					fmt.Fprint(w, `{"items":[{"serial":"FIRST"}]}`)
					return
				}
				// The second page carries the requested window, whatever the link said.
				q := r.URL.Query()
				assert.Equal(t, []string{"2026-10-01T10:00:00Z"}, q["t0"])
				assert.Equal(t, []string{"2026-10-01T10:05:00Z"}, q["t1"])
				assert.NotContains(t, q, "interval")
				fmt.Fprint(w, `{"items":[{"serial":"SECOND"}]}`)
			}))
			defer server.Close()

			rows, err := listUsage(t.Context(), clientForServer(t, server))
			if tc.wantRows == nil {
				require.Error(t, err)
				assert.Nil(t, rows)
				assert.NotContains(t, err.Error(), "SECRET")
				assert.EqualValues(t, 1, calls.Load())
				return
			}
			require.NoError(t, err)
			var serials []string
			for _, row := range rows {
				serials = append(serials, row.Serial)
			}
			assert.Equal(t, tc.wantRows, serials)
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestTrafficDecodeFailures(t *testing.T) {
	operations := map[string]struct {
		read   func(*merakiapi.Client) (gotRows bool, err error)
		bodies []string
	}{
		"array response": {
			read: func(c *merakiapi.Client) (bool, error) {
				rows, err := c.ListSwitchPortPackets(context.Background(), "SWITCH", 5*time.Minute)
				return rows != nil, err
			},
			bodies: []string{`null`, `{}`, `[null]`, `[`, `[] {}`, `[] SECRET`},
		},
		"items object response": {
			read: func(c *merakiapi.Client) (bool, error) {
				rows, err := listUsage(context.Background(), c)
				return rows != nil, err
			},
			bodies: []string{
				`null`,
				`{}`,
				`{"items":null}`,
				`{"items":[null]}`,
				`{"items":{}}`,
				`{"items":[]} {}`,
				`{"items":[]} SECRET`,
				`[]`,
				`{"items":[}`,
			},
		},
	}
	for name, op := range operations {
		for _, body := range op.bodies {
			t.Run(name+" "+body, func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, body)
				}))
				defer server.Close()
				gotRows, err := op.read(clientForServer(t, server))
				require.Error(t, err)
				assert.False(t, gotRows)
				assert.NotContains(t, err.Error(), "SECRET")
			})
		}
	}
}

func TestTrafficPathIDsAndCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	c := clientForServer(t, server)

	for _, id := range []string{"", ".", ".."} {
		_, err := c.ListSwitchPortPackets(t.Context(), id, 5*time.Minute)
		require.Error(t, err)
		_, err = c.ListSwitchPortUsageHistory(t.Context(), id, trafficStart, trafficEnd)
		require.Error(t, err)
		_, err = c.ListApplianceUplinkUsage(t.Context(), id, trafficStart, trafficEnd)
		require.Error(t, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := c.ListSwitchPortPackets(ctx, "SWITCH", 5*time.Minute)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = listUsage(ctx, c)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = c.ListApplianceUplinkUsage(ctx, "ORG", trafficStart, trafficEnd)
	assert.ErrorIs(t, err, context.Canceled)
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

	rows, err := listUsage(t.Context(), clientForServer(t, server))
	assert.Nil(t, rows)
	_, ok := errors.AsType[*merakiapi.HTTPError](err)
	require.True(t, ok, "%v", err)
	assert.NotContains(t, err.Error(), "SECRET")
}

func TestSwitchUsageHTTPPolicy(t *testing.T) {
	tests := map[string]struct {
		respond func(w http.ResponseWriter, r *http.Request, call int32)
		timeout time.Duration
		wantOK  bool  // an empty list and no error
		wantErr error // a specific error when not ok; nil accepts any
	}{
		"gzip-compressed response": {
			respond: func(w http.ResponseWriter, _ *http.Request, _ int32) {
				w.Header().Set("Content-Encoding", "gzip")
				z := gzip.NewWriter(w)
				fmt.Fprint(z, `{"items":[]}`)
				_ = z.Close()
			},
			wantOK: true,
		},
		"response over the body limit": {
			respond: func(w http.ResponseWriter, _ *http.Request, _ int32) {
				fmt.Fprintf(w, `{"items":[],"padding":"%s"}`, strings.Repeat("x", 128))
			},
			wantErr: web.ErrResponseTooLarge,
		},
		"unsupported encoding": {
			respond: func(w http.ResponseWriter, _ *http.Request, _ int32) {
				w.Header().Set("Content-Encoding", "br")
				fmt.Fprint(w, `{"items":[]}`)
			},
		},
		"redirect keeping the window": {
			respond: func(w http.ResponseWriter, r *http.Request, call int32) {
				if call == 1 {
					http.Redirect(w, r, r.URL.String(), http.StatusTemporaryRedirect)
					return
				}
				fmt.Fprint(w, `{"items":[]}`)
			},
			wantOK: true,
		},
		"redirect moving the window": {
			respond: func(w http.ResponseWriter, r *http.Request, _ int32) {
				target := *r.URL
				q := target.Query()
				q.Set("t0", "2026-10-01T09:55:00Z")
				target.RawQuery = q.Encode()
				http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
			},
		},
		"deadline during the request": {
			respond: func(_ http.ResponseWriter, r *http.Request, _ int32) { <-r.Context().Done() },
			timeout: 100 * time.Millisecond,
			wantErr: context.DeadlineExceeded,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
				tc.respond(w, r, calls.Add(1))
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
			ctx := t.Context()
			if tc.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.timeout)
				defer cancel()
			}

			rows, err := listUsage(ctx, c)
			if tc.wantOK {
				require.NoError(t, err)
				assert.Equal(t, []merakiapi.SwitchPortUsageHistory{}, rows)
				return
			}
			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			assert.Nil(t, rows)
		})
	}
}
