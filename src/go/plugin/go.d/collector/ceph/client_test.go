// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func TestParseDashboardBaseURL(t *testing.T) {
	tests := map[string]struct {
		raw      string
		want     string
		wantFail bool
	}{
		"HTTP":               {raw: "http://ceph.example/", want: "http://ceph.example"},
		"HTTPS with prefix":  {raw: "https://ceph.example/dashboard/", want: "https://ceph.example/dashboard"},
		"missing":            {wantFail: true},
		"unsupported scheme": {raw: "ftp://ceph.example", wantFail: true},
		"missing host":       {raw: "https:///dashboard", wantFail: true},
		"userinfo":           {raw: "https://user:example" + "@" + "ceph.example", wantFail: true},
		"query":              {raw: "https://ceph.example?token=secret", wantFail: true},
		"fragment":           {raw: "https://ceph.example/#dashboard", wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseDashboardBaseURL(test.raw)
			if test.wantFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got.String())
		})
	}
}

func TestCephClientActiveLoginAndRequest(t *testing.T) {
	var discoveryRequests, loginRequests atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				discoveryRequests.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer test-token" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"fsid": "cluster-fsid"})
		case urlPathApiAuth:
			loginRequests.Add(1)
			if r.Header.Get("Authorization") != "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var credentials map[string]string
			if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil ||
				credentials["username"] != "netdata" || credentials["password"] != "test-password" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusCreated, authLoginResp{
				Token: "test-token",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := newTestCephClient(t, srv.URL, false, func(cfg *web.RequestConfig) {
		cfg.Username = "netdata"
		cfg.Password = "test-password"
	})
	var got struct {
		FSID string `json:"fsid"`
	}
	require.NoError(
		t,
		client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &got),
	)
	assert.Equal(t, "cluster-fsid", got.FSID)
	assert.EqualValues(t, 1, discoveryRequests.Load())
	assert.EqualValues(t, 1, loginRequests.Load())
}

func TestCephClientStandbyRedirectDiscoversActiveWithoutCredentials(t *testing.T) {
	var activeURL string
	var standbySawCredentials atomic.Bool
	var activeDiscoveryRequests atomic.Int64

	active := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				activeDiscoveryRequests.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer test-active" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"fsid": "active-fsid"})
		case urlPathApiAuth:
			writeJSON(w, http.StatusCreated, authLoginResp{
				Token: "test-active",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer active.Close()
	activeURL = active.URL

	standby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			standbySawCredentials.Store(true)
		}
		http.Redirect(w, r, activeURL+"/", http.StatusSeeOther)
	}))
	defer standby.Close()

	client := newTestCephClient(t, standby.URL, false, func(cfg *web.RequestConfig) {
		cfg.Username = "netdata"
		cfg.Password = "test-password"
	}, active.URL)
	var got map[string]any
	require.NoError(
		t,
		client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &got),
	)
	assert.Equal(t, "active-fsid", got["fsid"])
	assert.False(t, standbySawCredentials.Load())
	assert.EqualValues(t, 1, activeDiscoveryRequests.Load())
}

func TestCephClientRejectsUntrustedActiveOriginBeforeAuthentication(t *testing.T) {
	var activeSawRequest atomic.Bool
	active := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		activeSawRequest.Store(true)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer active.Close()

	standby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, active.URL+"/", http.StatusSeeOther)
	}))
	defer standby.Close()

	client := newTestCephClient(t, standby.URL, false, func(cfg *web.RequestConfig) {
		cfg.Username = "netdata"
		cfg.Password = "test-password"
	})
	var got map[string]any
	err := client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not trusted")
	assert.False(t, activeSawRequest.Load())
}

func TestCephClientRejectsRedirectWhenConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://active.example/", http.StatusSeeOther)
	}))
	defer srv.Close()

	client := newTestCephClient(t, srv.URL, true, func(cfg *web.RequestConfig) {
		cfg.Username = "netdata"
		cfg.Password = "test-password"
	})
	var got map[string]any
	err := client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not_follow_redirects")
}

func TestCephClientDetectsRedirectLoop(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/", http.StatusSeeOther)
	}))
	defer srv.Close()

	client := newTestCephClient(t, srv.URL, false, func(cfg *web.RequestConfig) {
		cfg.Username = "netdata"
		cfg.Password = "test-password"
	})
	var got map[string]any
	err := client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect loop")
}

func TestCephClientRetriesManagedTokenOnceOnUnauthorized(t *testing.T) {
	var loginCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			switch r.Header.Get("Authorization") {
			case "":
				w.WriteHeader(http.StatusUnauthorized)
			case "Bearer token-1":
				w.WriteHeader(http.StatusUnauthorized)
			case "Bearer token-2":
				writeJSON(w, http.StatusOK, map[string]any{"fsid": "cluster-fsid"})
			default:
				w.WriteHeader(http.StatusForbidden)
			}
		case urlPathApiAuth:
			n := loginCount.Add(1)
			writeJSON(w, http.StatusCreated, authLoginResp{
				Token: "token-" + string(rune('0'+n)),
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := newTestCephClient(t, srv.URL, false, func(cfg *web.RequestConfig) {
		cfg.Username = "netdata"
		cfg.Password = "test-password"
	})
	var got map[string]any
	require.NoError(
		t,
		client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &got),
	)
	assert.EqualValues(t, 2, loginCount.Load())
}

func TestCephClientRereadsStaticBearerToken(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-1\n"), 0o600))

	var authenticatedRequests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authenticatedRequests.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"token": r.Header.Get("Authorization")})
	}))
	defer srv.Close()

	client := newTestCephClient(t, srv.URL, false, func(cfg *web.RequestConfig) {
		cfg.BearerTokenFile = tokenFile
	})
	var first map[string]any
	require.NoError(
		t,
		client.getJSON(context.Background(), "first", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &first),
	)
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-2\n"), 0o600))
	var second map[string]any
	require.NoError(
		t,
		client.getJSON(context.Background(), "second", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &second),
	)

	assert.Equal(t, "Bearer token-1", first["token"])
	assert.Equal(t, "Bearer token-2", second["token"])
	assert.EqualValues(t, 2, authenticatedRequests.Load())
}

func TestCephClientRejectsUnsafeStaticBearerTokenFile(t *testing.T) {
	tests := map[string]struct {
		prepare func(*testing.T) string
		wantErr error
	}{
		"oversized regular file": {
			prepare: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(path, nil, 0o600))
				require.NoError(t, os.Truncate(path, safefile.MaxSize+1))
				return path
			},
			wantErr: safefile.ErrTooLarge,
		},
		"non-regular file": {
			prepare: func(t *testing.T) string { return t.TempDir() },
			wantErr: safefile.ErrNotRegular,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client := newTestCephClient(t, "https://ceph.example", false, func(cfg *web.RequestConfig) {
				cfg.BearerTokenFile = test.prepare(t)
			})

			_, _, err := client.tokenForRequest(context.Background(), requireURL(t, "https://ceph.example"))
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestCephClientLogicalOperationUsesSingleDeadline(t *testing.T) {
	const requestDelay = 70 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "Bearer token-1" {
				time.Sleep(requestDelay)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `"cluster-fsid"`)
		case urlPathApiAuth:
			time.Sleep(requestDelay)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":"token-2"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client, err := newCephClient(&http.Client{
		Timeout: 100 * time.Millisecond,
	}, web.RequestConfig{
		URL:      srv.URL,
		Username: "netdata",
		Password: "test-password",
	}, false, nil)
	require.NoError(t, err)
	client.activeBase = requireURL(t, srv.URL)
	client.jwt = "token-1"

	var fsid string
	err = client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &fsid)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestCephClientIdentityFallbackUsesSingleDeadline(t *testing.T) {
	const requestDelay = 70 * time.Millisecond

	var modernRequests, legacyRequests atomic.Int64
	httpClient := &http.Client{
		Timeout: 100 * time.Millisecond,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			var status int
			var body string
			switch req.URL.Path {
			case urlPathAPIClusterFSID:
				modernRequests.Add(1)
				status = http.StatusNotFound
			case urlPathApiMonitor:
				legacyRequests.Add(1)
				status = http.StatusOK
				body = `{"mon_status":{"monmap":{"fsid":"cluster-fsid"}}}`
			default:
				return nil, fmt.Errorf("unexpected request path %q", req.URL.Path)
			}
			return delayedHTTPResponse(req, requestDelay, status, body)
		}),
	}
	client, err := newCephClient(httpClient, web.RequestConfig{
		URL: "https://ceph.example",
	}, false, nil)
	require.NoError(t, err)
	client.activeBase = requireURL(t, "https://ceph.example")
	client.activeGen = 1
	client.jwt = "test-token"

	_, err = client.probeClusterIdentity(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualValues(t, 1, modernRequests.Load())
	assert.EqualValues(t, 1, legacyRequests.Load())
}

func TestCephClientIdentityGenerationRetryUsesSingleDeadline(t *testing.T) {
	const requestDelay = 70 * time.Millisecond

	var startedRequests, completedRequests atomic.Int64
	var client *cephClient
	httpClient := &http.Client{
		Timeout: 100 * time.Millisecond,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			startedRequests.Add(1)
			response, err := delayedHTTPResponse(req, requestDelay, http.StatusOK, `"cluster-fsid"`)
			if err != nil {
				return nil, err
			}
			if completedRequests.Add(1) == 1 {
				client.stateMu.Lock()
				client.activeGen++
				client.stateMu.Unlock()
			}
			return response, nil
		}),
	}
	var err error
	client, err = newCephClient(httpClient, web.RequestConfig{
		URL: "https://ceph.example",
	}, false, nil)
	require.NoError(t, err)
	client.activeBase = requireURL(t, "https://ceph.example")
	client.activeGen = 1
	client.jwt = "test-token"

	_, err = client.probeClusterIdentity(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualValues(t, 2, startedRequests.Load())
	assert.EqualValues(t, 1, completedRequests.Load())
}

func TestCephClientOSDFallbackUsesSingleDeadline(t *testing.T) {
	const requestDelay = 70 * time.Millisecond

	var modernRequests, legacyRequests atomic.Int64
	httpClient := &http.Client{
		Timeout: 100 * time.Millisecond,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != urlPathApiOsd {
				return nil, fmt.Errorf("unexpected request path %q", req.URL.Path)
			}
			status := http.StatusOK
			body := `[]`
			switch req.Header.Get("Accept") {
			case hdrAcceptVersionV11:
				modernRequests.Add(1)
				status = http.StatusUnsupportedMediaType
				body = ""
			case hdrAcceptVersion:
				legacyRequests.Add(1)
			default:
				return nil, fmt.Errorf("unexpected Accept header %q", req.Header.Get("Accept"))
			}
			return delayedHTTPResponse(req, requestDelay, status, body)
		}),
	}
	client, err := newCephClient(httpClient, web.RequestConfig{
		URL: "https://ceph.example",
	}, false, nil)
	require.NoError(t, err)
	client.activeBase = requireURL(t, "https://ceph.example")
	client.activeGen = 1
	client.validatedGen = 1
	client.expectedFSID = "cluster-fsid"
	client.jwt = "test-token"

	c := &Collector{
		apiClient: client,
	}
	_, err = c.fetchAllOSDs(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualValues(t, 1, modernRequests.Load())
	assert.EqualValues(t, 1, legacyRequests.Load())
}

func TestCephClientDiscoveryWaitHonorsContext(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			close(requestStarted)
			<-releaseRequest
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}),
	}
	client, err := newCephClient(httpClient, web.RequestConfig{
		URL: "https://ceph.example",
	}, false, nil)
	require.NoError(t, err)

	ownerDone := make(chan error, 1)
	go func() {
		_, err := client.ensureActiveBase(context.Background())
		ownerDone <- err
	}()
	<-requestStarted

	ctx, cancel := context.WithCancel(context.Background())
	waiterStarted := make(chan struct{})
	waiterDone := make(chan error, 1)
	go func() {
		close(waiterStarted)
		_, err := client.ensureActiveBase(ctx)
		waiterDone <- err
	}()
	<-waiterStarted
	cancel()

	var waiterErr error
	waitTimedOut := false
	select {
	case waiterErr = <-waiterDone:
	case <-time.After(250 * time.Millisecond):
		waitTimedOut = true
	}
	close(releaseRequest)
	require.NoError(t, <-ownerDone)
	if waitTimedOut {
		waiterErr = <-waiterDone
		t.Fatalf("discovery waiter ignored its canceled context: %v", waiterErr)
	}
	require.ErrorIs(t, waiterErr, context.Canceled)
}

func TestCephClientLoginWaitHonorsContext(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			close(requestStarted)
			<-releaseRequest
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"token":"test-token"}`)),
				Request:    req,
			}, nil
		}),
	}
	client, err := newCephClient(httpClient, web.RequestConfig{
		URL:      "https://ceph.example",
		Username: "netdata",
		Password: "test-password",
	}, false, nil)
	require.NoError(t, err)
	base := requireURL(t, "https://ceph.example")

	type tokenResult struct {
		token string
		err   error
	}
	ownerDone := make(chan tokenResult, 1)
	go func() {
		token, _, err := client.tokenForRequest(context.Background(), base)
		ownerDone <- tokenResult{
			token: token,
			err:   err,
		}
	}()
	<-requestStarted

	ctx, cancel := context.WithCancel(context.Background())
	waiterStarted := make(chan struct{})
	waiterDone := make(chan error, 1)
	go func() {
		close(waiterStarted)
		_, _, err := client.tokenForRequest(ctx, base)
		waiterDone <- err
	}()
	<-waiterStarted
	cancel()

	var waiterErr error
	waitTimedOut := false
	select {
	case waiterErr = <-waiterDone:
	case <-time.After(250 * time.Millisecond):
		waitTimedOut = true
	}
	close(releaseRequest)
	owner := <-ownerDone
	require.NoError(t, owner.err)
	require.Equal(t, "test-token", owner.token)
	if waitTimedOut {
		waiterErr = <-waiterDone
		t.Fatalf("login waiter ignored its canceled context: %v", waiterErr)
	}
	require.ErrorIs(t, waiterErr, context.Canceled)
}

func TestCephClientRuntimeRedirectFailoverValidatesClusterIdentity(t *testing.T) {
	const originalPath = "/api/pool"
	tests := map[string]struct {
		secondFSID string
		wantErr    bool
	}{
		"same cluster":      {secondFSID: "cluster-fsid"},
		"different cluster": {secondFSID: "other-cluster", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var secondURL string
			var failedOver atomic.Bool
			var secondFSID atomic.Value
			var secondDiscoveryRequests, secondIdentityRequests, secondPoolRequests atomic.Int64
			secondFSID.Store(test.secondFSID)

			second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case urlPathAPIClusterFSID:
					if r.Header.Get("Authorization") == "" {
						secondDiscoveryRequests.Add(1)
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					secondIdentityRequests.Add(1)
					writeJSON(w, http.StatusOK, secondFSID.Load().(string))
				case urlPathApiAuth:
					writeJSON(w, http.StatusCreated, authLoginResp{
						Token: "test-second",
					})
				case originalPath:
					secondPoolRequests.Add(1)
					if r.URL.Query().Get("stats") != "true" || r.Header.Get("Authorization") != "Bearer test-second" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					writeJSON(w, http.StatusOK, []map[string]any{{"pool_name": "pool-a"}})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer second.Close()
			secondURL = second.URL

			first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failedOver.Load() {
					http.Redirect(w, r, secondURL+"/", http.StatusSeeOther)
					return
				}
				switch r.URL.Path {
				case urlPathAPIClusterFSID:
					if r.Header.Get("Authorization") == "" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					writeJSON(w, http.StatusOK, "cluster-fsid")
				case urlPathApiAuth:
					writeJSON(w, http.StatusCreated, authLoginResp{
						Token: "test-first",
					})
				case originalPath:
					failedOver.Store(true)
					http.Redirect(w, r, secondURL+"/", http.StatusSeeOther)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer first.Close()

			client := newTestCephClient(t, first.URL, false, func(cfg *web.RequestConfig) {
				cfg.Username = "netdata"
				cfg.Password = "test-password"
			}, second.URL)
			collector := &Collector{
				Config: Config{
					FunctionOnly: true,
				},
				apiClient: client,
			}
			_, err := collector.probeClusterIdentity(context.Background())
			require.NoError(t, err)

			var got []map[string]any
			err = client.getJSON(context.Background(), "list pools", originalPath, hdrAcceptVersion,
				url.Values{
					"stats": {"true"},
				}, &got)
			if test.wantErr {
				require.ErrorContains(t, err, "cluster identity changed")
				assert.Empty(t, got)
				assert.Zero(t, secondPoolRequests.Load())

				secondFSID.Store("cluster-fsid")
				err = client.getJSON(context.Background(), "list pools", originalPath, hdrAcceptVersion,
					url.Values{
						"stats": {"true"},
					}, &got)
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, "pool-a", got[0]["pool_name"])
				assert.EqualValues(t, 2, secondDiscoveryRequests.Load())
				assert.EqualValues(t, 2, secondIdentityRequests.Load())
				assert.EqualValues(t, 1, secondPoolRequests.Load())
			} else {
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, "pool-a", got[0]["pool_name"])
				assert.EqualValues(t, 1, secondPoolRequests.Load())
				assert.EqualValues(t, 1, secondDiscoveryRequests.Load())
				assert.EqualValues(t, 1, secondIdentityRequests.Load())
			}
		})
	}
}

func TestCephClientRuntimeTransportFailoverValidatesClusterIdentity(t *testing.T) {
	const originalPath = "/api/pool"
	tests := map[string]struct {
		secondFSID string
		wantErr    bool
	}{
		"same cluster":      {secondFSID: "cluster-fsid"},
		"different cluster": {secondFSID: "other-cluster", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var activeTarget atomic.Value
			var standbySawCredentials atomic.Bool
			var secondIdentityRequests, secondPoolRequests atomic.Int64

			newActive := func(token, pool, fsid string, identityRequests, poolRequests *atomic.Int64) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case urlPathAPIClusterFSID:
						if r.Header.Get("Authorization") == "" {
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						identityRequests.Add(1)
						if r.Header.Get("Authorization") != "Bearer "+token {
							w.WriteHeader(http.StatusForbidden)
							return
						}
						writeJSON(w, http.StatusOK, fsid)
					case urlPathApiAuth:
						writeJSON(w, http.StatusCreated, authLoginResp{
							Token: token,
						})
					case originalPath:
						poolRequests.Add(1)
						if r.Header.Get("Authorization") != "Bearer "+token {
							w.WriteHeader(http.StatusForbidden)
							return
						}
						writeJSON(w, http.StatusOK, []map[string]any{{"pool_name": pool}})
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
			}

			var firstIdentityRequests, firstPoolRequests atomic.Int64
			first := newActive("test-first", "pool-first", "cluster-fsid", &firstIdentityRequests, &firstPoolRequests)
			second := newActive(
				"test-second",
				"pool-second",
				test.secondFSID,
				&secondIdentityRequests,
				&secondPoolRequests,
			)
			defer second.Close()
			activeTarget.Store(first.URL)

			standby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					standbySawCredentials.Store(true)
				}
				http.Redirect(w, r, activeTarget.Load().(string)+"/", http.StatusSeeOther)
			}))
			defer standby.Close()

			client := newTestCephClient(t, standby.URL, false, func(cfg *web.RequestConfig) {
				cfg.Username = "netdata"
				cfg.Password = "test-password"
			}, first.URL, second.URL)
			collector := &Collector{
				Config: Config{
					FunctionOnly: true,
				},
				apiClient: client,
			}
			_, err := collector.probeClusterIdentity(context.Background())
			require.NoError(t, err)

			var got []map[string]any
			require.NoError(
				t,
				client.getJSON(context.Background(), "list pools", originalPath, hdrAcceptVersion, nil, &got),
			)
			require.Len(t, got, 1)
			assert.Equal(t, "pool-first", got[0]["pool_name"])

			activeTarget.Store(second.URL)
			first.Close()
			got = nil
			err = client.getJSON(context.Background(), "list pools", originalPath, hdrAcceptVersion, nil, &got)
			if test.wantErr {
				require.ErrorContains(t, err, "cluster identity changed")
				assert.Empty(t, got)
				assert.Zero(t, secondPoolRequests.Load())
			} else {
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, "pool-second", got[0]["pool_name"])
				assert.EqualValues(t, 1, secondPoolRequests.Load())
			}
			assert.EqualValues(t, 1, firstIdentityRequests.Load())
			assert.EqualValues(t, 1, firstPoolRequests.Load())
			assert.EqualValues(t, 1, secondIdentityRequests.Load())
			assert.False(t, standbySawCredentials.Load())
		})
	}
}

func TestCephClientLoginTransportFailureRediscoversThroughConfiguredStandby(t *testing.T) {
	var activeTarget atomic.Value
	var standbySawCredentials atomic.Bool

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeJSON(w, http.StatusOK, "cluster-fsid")
		case urlPathApiAuth:
			writeJSON(w, http.StatusCreated, authLoginResp{
				Token: "test-second",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer second.Close()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			w.WriteHeader(http.StatusUnauthorized)
		case urlPathApiAuth:
			activeTarget.Store(second.URL)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				return
			}
			_ = conn.Close()
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer first.Close()
	activeTarget.Store(first.URL)

	standby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			standbySawCredentials.Store(true)
		}
		http.Redirect(w, r, activeTarget.Load().(string)+"/", http.StatusSeeOther)
	}))
	defer standby.Close()

	client := newTestCephClient(t, standby.URL, false, func(cfg *web.RequestConfig) {
		cfg.Username = "netdata"
		cfg.Password = "test-password"
	}, first.URL, second.URL)

	var fsid string
	require.NoError(
		t,
		client.getJSON(context.Background(), "get cluster FSID", urlPathAPIClusterFSID, hdrAcceptVersion, nil, &fsid),
	)
	assert.Equal(t, "cluster-fsid", fsid)
	assert.False(t, standbySawCredentials.Load())
}

func TestRedirectedDashboardBaseSecurity(t *testing.T) {
	current := requireURL(t, "https://standby.example:8443")

	tests := map[string]struct {
		location string
		want     string
		wantFail bool
	}{
		"Ceph root": {location: "https://active.example:8443/", want: "https://active.example:8443"},
		"preserved API path": {
			location: "https://active.example:8443/api/health/get_cluster_fsid",
			want:     "https://active.example:8443",
		},
		"relative root":    {location: "/dashboard", want: "https://standby.example:8443/dashboard"},
		"HTTPS downgrade":  {location: "http://active.example:8080/", wantFail: true},
		"userinfo":         {location: "https://user:example" + "@" + "active.example/", wantFail: true},
		"missing Location": {wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := redirectedDashboardBase(current, test.location, urlPathAPIClusterFSID)
			if test.wantFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got.String())
		})
	}
}

func TestParseAllowedRedirectOrigins(t *testing.T) {
	tests := map[string]struct {
		configured string
		allowed    []string
		want       []string
		wantFail   bool
	}{
		"deduplicates normalized origins": {
			configured: "https://mgr-a.example:8443/dashboard",
			allowed:    []string{"https://mgr-b.example:8443", "https://MGR-B.EXAMPLE:8443/"},
			want:       []string{"https://mgr-a.example:8443", "https://mgr-b.example:8443"},
		},
		"normalizes default HTTPS port": {
			configured: "https://mgr-a.example",
			allowed:    []string{"https://MGR-A.EXAMPLE:443"},
			want:       []string{"https://mgr-a.example"},
		},
		"rejects path": {
			configured: "https://mgr-a.example:8443/dashboard",
			allowed:    []string{"https://mgr-b.example:8443/dashboard"},
			wantFail:   true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			origins, err := parseAllowedRedirectOrigins(requireURL(t, test.configured), test.allowed)
			if test.wantFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, origins, len(test.want))
			for _, want := range test.want {
				assert.Contains(t, origins, want)
			}
		})
	}
}

func TestCephClientRejectsManagedAuthenticationHeaders(t *testing.T) {
	tests := map[string]struct {
		header string
	}{
		"Authorization":             {header: "Authorization"},
		"case-folded Authorization": {header: "authorization"},
		"Cookie":                    {header: "Cookie"},
		"Host":                      {header: "Host"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newCephClient(&http.Client{}, web.RequestConfig{
				URL:     "https://ceph.example",
				Headers: map[string]string{test.header: "secret"},
			}, false, nil)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestCephClientPreservesEscapedPathSegments(t *testing.T) {
	var escapedPath string
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			escapedPath = req.URL.EscapedPath()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Request:    req,
			}, nil
		}),
	}
	client, err := newCephClient(httpClient, web.RequestConfig{
		URL: "https://ceph.example/dashboard",
	}, false, nil)
	require.NoError(t, err)
	base := requireURL(t, "https://ceph.example/dashboard")
	resp, err := client.request(
		context.Background(),
		base,
		http.MethodGet,
		"/api/resource/tenant%2Fuser",
		hdrAcceptVersion,
		nil,
		nil,
		"token",
	)
	require.NoError(t, err)
	web.CloseBody(resp)
	assert.Equal(t, "/dashboard/api/resource/tenant%2Fuser", escapedPath)
}

func newTestCephClient(
	t *testing.T,
	rawURL string,
	notFollowRedirects bool,
	configure func(*web.RequestConfig),
	allowedRedirectOrigins ...string,
) *cephClient {
	t.Helper()
	cfg := web.RequestConfig{
		URL: rawURL,
	}
	if configure != nil {
		configure(&cfg)
	}
	client, err := newCephClient(&http.Client{}, cfg, notFollowRedirects, allowedRedirectOrigins)
	require.NoError(t, err)
	return client
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func requireURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func delayedHTTPResponse(req *http.Request, delay time.Duration, status int, body string) (*http.Response, error) {
	select {
	case <-time.After(delay):
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }
