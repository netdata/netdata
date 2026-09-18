// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogsDiscoverAndReadAllPages(t *testing.T) {
	docs := testutil.LogDocuments(513)
	var entryReads, requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasPrefix(r.URL.Path, testutil.LogEntriesURI) {
			entryReads.Add(1)
		}
		require.Equal(t, http.MethodGet, r.Method)
		doc, ok := docs[r.URL.RequestURI()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		testutil.WriteJSON(w, doc)
	}))
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	services, err := client.Logs().Services(t.Context())
	require.NoError(t, err)
	require.Equal(t, []LogService{{URI: testutil.LogServiceURI, Name: "Events"}}, services)
	assert.Zero(t, entryReads.Load(), "info only discovers services")
	assert.EqualValues(t, 13, requests.Load(), "root plus three owner, collection and service traversals")
	result, err := client.Logs().Entries(t.Context(), testutil.LogServiceURI)
	require.NoError(t, err)
	require.Len(t, result.Entries, 513, "no event cap or partial last-page result")
	assert.Equal(t, services, result.Services)
	assert.EqualValues(t, 259, entryReads.Load(), "two pages and 257 linked members; inline entries need no GET")
	assert.Equal(t, "Fixture event 512", result.Entries[512].Message)
	assert.Equal(t, "2023-11-14T22:13:20Z", result.Entries[512].EventTimestamp)
	before := requests.Load()
	_, err = client.Logs().Entries(t.Context(), "https://other.example/private")
	require.ErrorIs(t, err, ErrLogServiceUnavailable)
	assert.EqualValues(t, 13, requests.Load()-before, "unadvertised selection is never fetched")
}

func TestLogsEmptyUnsupportedAndFailedReads(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate   func(map[string]map[string]any)
		failPath string
		want     error
		count    int
	}{
		"empty":            {count: 0},
		"missing entries":  {count: 0, mutate: func(d map[string]map[string]any) { delete(d[testutil.LogServiceURI], "Entries") }, want: ErrLogEntriesUnsupported},
		"failed page":      {count: 300, failPath: testutil.LogEntriesURI + "?$skip=256"},
		"failed member":    {count: 4, failPath: testutil.LogEntriesURI + "/2"},
		"failed discovery": {count: 4, failPath: "/redfish/v1/Chassis/1"},
		"cross-origin page": {count: 4, mutate: func(d map[string]map[string]any) {
			d[testutil.LogEntriesURI]["Members@odata.nextLink"] = "https://other.example/redfish/v1/private?token=secret"
		}},
		"malformed member": {count: 4, mutate: func(d map[string]map[string]any) { d[testutil.LogEntriesURI+"/2"]["Message"] = []any{1} }},
	} {
		t.Run(name, func(t *testing.T) {
			docs := testutil.LogDocuments(tc.count)
			if tc.mutate != nil {
				tc.mutate(docs)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RequestURI() == tc.failPath {
					http.Error(w, "secret source diagnostics", 403)
					return
				}
				doc, ok := docs[r.URL.RequestURI()]
				if !ok {
					http.NotFound(w, r)
					return
				}
				testutil.WriteJSON(w, doc)
			}))
			t.Cleanup(server.Close)
			client := newTestProtocolClient(t, testConfig(server.URL, "none"))
			result, err := client.Logs().Entries(t.Context(), testutil.LogServiceURI)
			if name == "empty" {
				require.NoError(t, err)
				assert.Empty(t, result.Entries)
				return
			}
			require.Error(t, err)
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
			}
			assert.Empty(t, result.Entries, "partial reads are not published as complete")
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestLogsSessionsAreOwnedAndRecoveredIndependently(t *testing.T) {
	docs := testutil.LogDocuments(2)
	docs["/redfish/v1/"]["Links"] = map[string]any{"Sessions": testutil.Link("/redfish/v1/SessionService/Sessions")}
	var expired atomic.Bool
	var metricToken, logToken string
	var mu sync.Mutex
	server := testutil.NewServer(
		t,
		testutil.ServerConfig{
			SupportSession: true,
			HandleRequest: func(w http.ResponseWriter, r *http.Request) bool {
				if strings.Contains(r.URL.Path, "/SessionService/") {
					return false
				}
				if r.URL.Path == testutil.LogEntriesURI && !expired.Swap(true) {
					http.Error(w, "expired secret", 401)
					return true
				}
				if r.URL.Path == testutil.LogEntriesURI {
					mu.Lock()
					logToken = r.Header.Get("X-Auth-Token")
					mu.Unlock()
				}
				if doc, ok := docs[r.URL.RequestURI()]; ok {
					testutil.WriteJSON(w, doc)
					return true
				}
				return false
			},
		},
	)
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	t.Cleanup(client.Close)
	_, err := client.Acquire(t.Context())
	require.NoError(t, err)
	session, err := client.sdk.GetSession()
	require.NoError(t, err)
	metricToken = session.Token
	result, err := client.Logs().Entries(t.Context(), testutil.LogServiceURI)
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)
	assert.NotEqual(t, metricToken, logToken)
	assert.EqualValues(t, 3, server.SessionCreates.Load(), "metric session plus log session and one recovery")
	assert.EqualValues(t, 2, server.SessionDeletes.Load())
	assert.EqualValues(t, 1, server.ActiveSessions.Load(), "only metric session remains")
	_, err = client.Acquire(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, 3, server.SessionCreates.Load(), "metric session survives log cleanup")
}

func TestLogsShareRequestBudgetWithoutHoldingCollection(t *testing.T) {
	for _, tc := range []struct{ multiple, redirect bool }{
		{true, false}, {false, false}, {true, true}, {false, true},
	} {
		t.Run(fmt.Sprintf("multiple=%v/redirect=%v", tc.multiple, tc.redirect), func(t *testing.T) {
			docs := testutil.LogDocuments(0)
			docs["/redfish/v1/"]["ProtocolFeaturesSupported"] = map[string]any{"MultipleHTTPRequests": tc.multiple}
			if tc.redirect {
				docs["/redfish/v1/"]["@odata.id"] = "/redfish/v1/redirected"
			}
			arrived := make(chan struct{}, 1)
			var current, peak atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := current.Add(1)
				defer current.Add(-1)
				for {
					old := peak.Load()
					if n <= old || peak.CompareAndSwap(old, n) {
						break
					}
				}
				if r.URL.Path == testutil.LogEntriesURI {
					select {
					case arrived <- struct{}{}:
					default:
					}
					<-r.Context().Done()
					return
				}
				if tc.redirect {
					switch r.URL.Path {
					case "/redfish/v1/":
						http.Redirect(w, r, "/redfish/v1/redirected", http.StatusTemporaryRedirect)
						return
					case "/redfish/v1/redirected":
						testutil.WriteJSON(w, docs["/redfish/v1/"])
						return
					}
				}
				testutil.WriteJSON(w, docs[r.URL.RequestURI()])
			}))
			t.Cleanup(server.Close)
			cfg := testConfig(server.URL, "none")
			cfg.MaxConcurrentRequests = 2
			client := newTestProtocolClient(t, cfg)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			logs := make(chan error, 1)
			go func() { _, err := client.Logs().Entries(ctx, testutil.LogServiceURI); logs <- err }()
			select {
			case <-arrived:
			case <-time.After(asyncTestTimeout):
				t.Fatal("log read did not start")
			}
			collected := make(chan error, 1)
			go func() { _, err := client.Acquire(ctx); collected <- err }()
			if tc.multiple {
				select {
				case err := <-collected:
					require.NoError(t, err)
				case <-time.After(asyncTestTimeout):
					t.Fatal("log request blocked the entire metric collection")
				}
				assert.LessOrEqual(t, peak.Load(), int64(2))
				assert.EqualValues(t, 2, peak.Load())
			} else {
				select {
				case <-collected:
					t.Fatal("serial BMC allowed concurrent collection")
				case <-time.After(50 * time.Millisecond):
				}
				assert.EqualValues(t, 1, peak.Load())
			}
			cancel()
			select {
			case err := <-logs:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(asyncTestTimeout):
				t.Fatal("log cancellation did not settle")
			}
			if !tc.multiple {
				select {
				case err := <-collected:
					require.ErrorIs(t, err, context.Canceled)
				case <-time.After(asyncTestTimeout):
					t.Fatal("queued collection did not cancel")
				}
			}
		})
	}
}

func TestLogsConcurrentQueriesHonorJobRequestLimit(t *testing.T) {
	docs := testutil.LogDocuments(12)
	var active, peak atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		testutil.WriteJSON(w, docs[r.URL.RequestURI()])
	}))
	t.Cleanup(server.Close)
	cfg := testConfig(server.URL, "none")
	cfg.MaxConcurrentRequests = 2
	client := newTestProtocolClient(t, cfg)
	start := make(chan struct{})
	done := make(chan error, 5)
	for range 4 {
		go func() { <-start; _, err := client.Logs().Entries(t.Context(), testutil.LogServiceURI); done <- err }()
	}
	go func() { <-start; _, err := client.Acquire(t.Context()); done <- err }()
	close(start)
	for range 5 {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(asyncTestTimeout):
			t.Fatal("concurrent queries did not finish")
		}
	}
	assert.EqualValues(t, 2, peak.Load(), "independent SDK sessions must share one configured request budget")
}
