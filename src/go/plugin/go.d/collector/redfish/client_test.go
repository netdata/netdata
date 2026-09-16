// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const asyncTestTimeout = 10 * time.Second

func TestProtocolClientBasicCheckAndCollect(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{requireBasic: true})
	defer server.Close()

	cfg := testConfig(server.URL, "basic")
	client := newTestProtocolClient(t, cfg)
	require.NoError(t, client.Check(context.Background()))

	result, err := client.Collect(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	assert.Equal(t, "success", result.Metrics.Status)
	assert.Equal(t, "present", result.Metrics.SelectedSystem)
	assert.Len(t, result.Inventory, 4)
	assert.Positive(t, result.Metrics.HTTPRequests["started"])

	coverage := New(redfishruntime.New())
	managed, ok := metrix.AsCycleManagedStore(coverage.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	coverage.metrics.observe("endpoint-key", "endpoint-job", result.Metrics)
	coverage.hardware.observe(result.Hardware)
	require.NoError(t, cycle.CommitCycleSuccess())
	collecttest.AssertChartCoverage(t, coverage, collecttest.ChartCoverageExpectation{})
}

func TestFairnessOrderRotatesByAttemptedWork(t *testing.T) {
	client := &protocolClient{}

	require.Equal(t, []int{0, 1, 2, 3}, client.fairnessOrder("work", 4))
	client.advanceFairnessCursor("work", 4, 2)
	require.Equal(t, []int{2, 3, 0, 1}, client.fairnessOrder("work", 4))
	client.advanceFairnessCursor("work", 4, 1)
	require.Equal(t, []int{3, 0, 1, 2}, client.fairnessOrder("work", 4))

	require.Nil(t, client.fairnessOrder("empty", 0))
	client.advanceFairnessCursor("empty", 0, 1)
}

type sensitiveNetError struct {
	timeout   bool
	temporary bool
}

func (e sensitiveNetError) Error() string   { return "proxy-user:test-password@private-proxy" }
func (e sensitiveNetError) Timeout() bool   { return e.timeout }
func (e sensitiveNetError) Temporary() bool { return e.temporary }

type requestRecordingTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	path []string
}

func (t *requestRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.path = append(t.path, req.URL.Path)
	t.mu.Unlock()
	return t.base.RoundTrip(req)
}

func (t *requestRecordingTransport) paths() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.path...)
}

func (t *requestRecordingTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func TestSanitizeTransportErrorPreservesPolicyWithoutLeakingDetails(t *testing.T) {
	t.Parallel()

	retryable := sanitizeTransportError(sensitiveNetError{temporary: true})
	assert.NotContains(t, retryable.Error(), "test-password")
	assert.Equal(t, "transport", classifyError(retryable))
	assert.True(t, retryableTransport(retryable))

	timeout := sanitizeTransportError(sensitiveNetError{timeout: true})
	assert.NotContains(t, timeout.Error(), "test-password")
	assert.Equal(t, "timeout", classifyError(timeout))
	assert.False(t, retryableTransport(timeout))
}

func TestProtocolClientSessionLifecycle(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{supportSession: true})
	defer server.Close()

	cfg := testConfig(server.URL, "session")
	client := newTestProtocolClient(t, cfg)
	require.NoError(t, client.Check(context.Background()))

	result, err := client.Collect(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	require.NoError(t, client.Close(context.Background()))
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	assert.Equal(t, int64(1), server.sessionDeletes.Load())
}

func TestProtocolClientReusesSessionAcrossRepeatedCheck(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{supportSession: true})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(context.Background()))
	require.NoError(t, client.Check(context.Background()))
	require.NoError(t, client.Close(context.Background()))
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	assert.Equal(t, int64(1), server.sessionDeletes.Load())
}

func TestProtocolClientCleansMalformedCreatedSession(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:       true,
		malformedSessionBody: true,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.Error(t, client.Check(context.Background()))
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	assert.Equal(t, int64(1), server.sessionDeletes.Load())
}

func TestProtocolClientDoesNotCreateAnotherSessionUntilPendingCleanupSucceeds(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:       true,
		malformedSessionBody: true,
		sessionDeleteStatus:  http.StatusInternalServerError,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.Error(t, client.Check(context.Background()))
	require.ErrorContains(t, client.Check(context.Background()), "retire unactivated Redfish session")
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	assert.Equal(t, int64(2), server.sessionDeletes.Load())
	require.Error(t, client.Close(context.Background()))
}

func TestProtocolClientRetriesSessionCreationAfterPendingSessionIsAlreadyGone(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:              true,
		malformedSessionBody:        true,
		sessionDeleteStatusSequence: []int{http.StatusInternalServerError, http.StatusNotFound, http.StatusNoContent},
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.Error(t, client.Check(context.Background()))
	err := client.Check(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "retire unactivated Redfish session")
	assert.Equal(t, int64(2), server.sessionCreates.Load())
	assert.Equal(t, int64(3), server.sessionDeletes.Load())
}

func TestProtocolClientTriesEveryAdvertisedSessionPath(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:   true,
		badDirectSession: true,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(context.Background()))
	require.NoError(t, client.Close(context.Background()))
	assert.Equal(t, int64(1), server.unsupportedSessionCreates.Load())
	assert.Equal(t, int64(1), server.sessionCreates.Load())
}

func TestSessionServiceAcceptsRedirectAndResolvesSessionsLink(t *testing.T) {
	t.Parallel()

	const finalURI = "/redfish/v1/redirected/session-service/"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/SessionService":
			http.Redirect(w, r, finalURI, http.StatusTemporaryRedirect)
		case finalURI:
			writeJSON(w, map[string]any{
				"@odata.id":   finalURI,
				"@odata.type": "#SessionService.v1_2_0.SessionService",
				"Id":          "SessionService",
				"Name":        "Session Service",
				"Sessions":    map[string]any{"@odata.id": finalURI + "sessions"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	sessionsURI, err := client.readSessionServiceSessions(
		context.Background(), "/redfish/v1/SessionService", nil,
	)
	require.NoError(t, err)
	assert.Equal(t, server.URL+finalURI+"sessions", sessionsURI)
}

func TestProtocolClientUsesLegacySessionPathAfterAdvertisedMethodNotAllowed(t *testing.T) {
	t.Parallel()

	var advertisedCreates, legacyCreates, deletes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/redfish/v1/SessionService/Sessions":
			advertisedCreates.Add(1)
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		case r.Method == http.MethodPost && r.URL.Path == legacySessionsURI:
			legacyCreates.Add(1)
			w.Header().Set("X-Auth-Token", "legacy-token")
			w.Header().Set("Location", legacySessionsURI+"/1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			writeJSONBody(w, map[string]any{
				"@odata.id":   legacySessionsURI + "/1",
				"@odata.type": "#Session.v1_6_0.Session",
				"Id":          "1",
				"Name":        "Legacy Session",
			})
		case r.Method == http.MethodDelete && r.URL.Path == legacySessionsURI+"/1":
			deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	root := &serviceRootDocument{}
	root.Links.Sessions.ODataID = "/redfish/v1/SessionService/Sessions"
	require.NoError(t, client.initializeSession(context.Background(), root, nil))
	require.NoError(t, client.Close(context.Background()))
	assert.Equal(t, int64(1), advertisedCreates.Load())
	assert.Equal(t, int64(1), legacyCreates.Load())
	assert.Equal(t, int64(1), deletes.Load())
}

func TestProtocolClientCleanupUsesIndependentContext(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{supportSession: true})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	require.NoError(t, client.Close(ctx))
	assert.Equal(t, int64(1), server.sessionDeletes.Load())
}

func TestProtocolClientRecreatesExpiredSessionOnce(t *testing.T) {
	t.Parallel()

	var expireOnce atomic.Bool
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:    true,
		expireSessionOnce: &expireOnce,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(context.Background()))
	expireOnce.Store(true)

	result, err := client.Collect(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	require.NoError(t, client.Close(context.Background()))
	assert.Equal(t, int64(2), server.sessionCreates.Load())
	assert.Equal(t, int64(2), server.sessionDeletes.Load())
}

func TestProtocolClientSessionRecoveryAccountsForEveryWireOperation(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{supportSession: true})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(context.Background()))
	auth := client.currentAuth(true)
	stats := &wireStats{failures: make(map[string]int)}

	_, err := client.refreshSession(context.Background(), auth.token, stats)
	require.NoError(t, err)
	assert.Equal(
		t,
		4,
		stats.started,
		"ServiceRoot GET, SessionService GET, Session POST, and old Session DELETE",
	)
	assert.Equal(t, 4, stats.successful)
	assert.Zero(t, stats.failed)
	require.NoError(t, client.Close(context.Background()))
}

func TestProtocolClientAutoFallbackClassification(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionStatus int
		wantErr       bool
		wantBasic     bool
	}{
		"unsupported": {
			sessionStatus: http.StatusNotImplemented,
			wantBasic:     true,
		},
		"unauthorized": {
			sessionStatus: http.StatusUnauthorized,
			wantErr:       true,
		},
		"malformed success": {
			sessionStatus: http.StatusCreated,
			wantErr:       true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := newRedfishTestServer(t, redfishTestServerConfig{
				sessionStatus: tc.sessionStatus,
				requireBasic:  tc.wantBasic,
			})
			defer server.Close()

			client := newTestProtocolClient(t, testConfig(server.URL, "auto"))
			err := client.Check(context.Background())
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantBasic, server.basicRequests.Load() > 0)
		})
	}
}

func TestProtocolClientRejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "must not be reached", http.StatusInternalServerError)
	}))
	defer foreign.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			http.Redirect(w, r, foreign.URL+"/redfish/v1/", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	err := client.Check(context.Background())
	require.ErrorContains(t, err, "crosses the configured origin")
}

func TestProtocolClientURIProfiles(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	for name, raw := range map[string]string{
		"fragment":             "/redfish/v1/Systems/1#/Status",
		"encoded dot":          "/redfish/v1/%2e%2e/admin",
		"encoded slash":        "/redfish/v1/Systems%2f1",
		"encoded unreserved":   "/redfish/v1/Systems/%41",
		"backslash":            `/redfish/v1\\admin`,
		"path relative":        "Systems/1",
		"ordinary query":       "/redfish/v1/Systems?$top=1",
		"cross origin":         "https://example.com/redfish/v1/Systems",
		"outside Redfish path": "/admin",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.resolveURI(client.root, raw, false)
			require.Error(t, err)
		})
	}
	opaque, err := client.resolveURI(client.root, "/redfish/v1/Systems?$skip=1", true)
	require.NoError(t, err)
	assert.Equal(t, "$skip=1", opaque.RawQuery)

	provenance, err := resolveRedfishURI(
		client.origin,
		client.root,
		"/redfish/v1/Chassis/1/Sensors/1#/Reading",
		uriProvenance,
	)
	require.NoError(t, err)
	assert.Equal(t, "/redfish/v1/Chassis/1/Sensors/1#/Reading", canonicalProvenanceURI(provenance))
	provenance, err = resolveRedfishURI(
		client.origin,
		provenance,
		"#/ReadingCelsius",
		uriProvenance,
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		"/redfish/v1/Chassis/1/Sensors/1#/ReadingCelsius",
		canonicalProvenanceURI(provenance),
	)
	_, err = resolveRedfishURI(client.origin, client.root, "#/Status", uriResource)
	require.Error(t, err)
	_, err = resolveRedfishURI(
		client.origin,
		client.root,
		"/redfish/v1/Chassis/1/Sensors/1#not-a-pointer",
		uriProvenance,
	)
	require.Error(t, err)

	root, origin, err := normalizeServiceRoot("https://bmc.example.test.")
	require.NoError(t, err)
	target, err := resolveRedfishURI(
		origin,
		root,
		"https://BMC.EXAMPLE.TEST./redfish/v1/Systems/1",
		uriResource,
	)
	require.NoError(t, err)
	assert.Equal(t, "https://bmc.example.test/redfish/v1/Systems/1", target.String())
}

func TestProtocolClientFollowsSameOriginGetRedirect(t *testing.T) {
	t.Parallel()

	var rootHits atomic.Int64
	server := newRedfishTestServer(t, redfishTestServerConfig{
		rootRedirect: "/redfish/v1/redirected",
		rootHits:     &rootHits,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	require.NoError(t, client.Check(context.Background()))
	assert.Positive(t, rootHits.Load())
}

func TestProtocolClientRejectsRedirectThatChangesAuthorizedQuery(t *testing.T) {
	t.Parallel()

	var redirected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("$top") == "1" {
			http.Redirect(w, r, "/redfish/v1/Systems?$top=2", http.StatusTemporaryRedirect)
			return
		}
		redirected.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	target, err := client.resolveURI(client.root, "/redfish/v1/Systems?$top=1", true)
	require.NoError(t, err)

	_, err = client.do(
		withOperationBudget(context.Background()),
		protocolRequest{method: http.MethodGet, target: target, auth: client.currentAuth(false)},
		nil,
		false,
		http.StatusOK,
	)
	require.ErrorContains(t, err, "changed an authorized query")
	assert.Zero(t, redirected.Load())
}

func TestProtocolClientRejectsRedirectThatRemovesAuthorizedQuery(t *testing.T) {
	t.Parallel()

	var redirected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			http.Redirect(w, r, "/redfish/v1/Systems", http.StatusTemporaryRedirect)
			return
		}
		redirected.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	target, err := client.resolveURI(client.root, "/redfish/v1/Systems?$top=1", true)
	require.NoError(t, err)

	_, err = client.do(
		withOperationBudget(context.Background()),
		protocolRequest{method: http.MethodGet, target: target, auth: client.currentAuth(false)},
		nil,
		false,
		http.StatusOK,
	)
	require.ErrorContains(t, err, "changed an authorized query")
	assert.Zero(t, redirected.Load())
}

func TestSuccessfulSessionResponseReportsCompatibilityHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("session request method = %q, want %q", r.Method, http.MethodPost)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Auth-Token", "test-token")
		w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
		w.WriteHeader(http.StatusCreated)
		writeJSONBody(w, map[string]any{
			"@odata.id":   "/redfish/v1/SessionService/Sessions/1",
			"@odata.type": "#Session.v1_6_0.Session",
			"Id":          "1",
			"Name":        "Session 1",
		})
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	root := &serviceRootDocument{}
	root.Links.Sessions.ODataID = "/redfish/v1/SessionService/Sessions"
	stats := &wireStats{failures: make(map[string]int)}

	require.NoError(t, client.initializeSession(withOperationBudget(context.Background()), root, stats))
	assert.Equal(t, []string{
		"Redfish compatibility: response OData-Version header is missing",
	}, responseCompatibilityDiagnostics(stats))
}

func TestCreateSessionRejectsOversizeAuthenticationHeaders(t *testing.T) {
	t.Parallel()

	for name, headers := range map[string]map[string]string{
		"token": {
			"X-Auth-Token": strings.Repeat("x", maxSessionTokenBytes+1),
			"Location":     "/redfish/v1/SessionService/Sessions/1",
		},
		"location": {
			"X-Auth-Token": "token",
			"Location":     strings.Repeat("x", maxURIBytes+1),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for key, value := range headers {
					w.Header().Set(key, value)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				writeJSONBody(w, map[string]any{
					"@odata.id":   "/redfish/v1/SessionService/Sessions/1",
					"@odata.type": "#Session.v1_6_0.Session",
					"Id":          "1",
					"Name":        "Session 1",
				})
			}))
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, "session"))
			target, err := client.resolveURI(client.root, "/redfish/v1/SessionService/Sessions", false)
			require.NoError(t, err)
			err = client.createSession(context.Background(), target, nil)
			require.ErrorContains(t, err, "oversized X-Auth-Token or Location")
			assert.Empty(t, client.currentAuth(true).token)
		})
	}
}

func TestFetchCollectionValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		handler http.HandlerFunc
		wantErr string
	}{
		"pagination loop": {
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":              "/redfish/v1/Test",
					"@odata.type":            "#TestCollection.TestCollection",
					"Members@odata.count":    1,
					"Members":                []any{},
					"Members@odata.nextLink": r.URL.RequestURI(),
				})
			},
			wantErr: "pagination loop",
		},
		"count changes": {
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") == "2" {
					writeJSON(w, map[string]any{
						"@odata.id":           "/redfish/v1/Test",
						"@odata.type":         "#TestCollection.TestCollection",
						"Members@odata.count": 2,
						"Members":             []any{map[string]any{"@odata.id": "/redfish/v1/Systems/1"}},
					})
					return
				}
				writeJSON(w, map[string]any{
					"@odata.id":              "/redfish/v1/Test",
					"@odata.type":            "#TestCollection.TestCollection",
					"Members@odata.count":    1,
					"Members":                []any{},
					"Members@odata.nextLink": "/redfish/v1/Test?page=2",
				})
			},
			wantErr: "@odata.count changed",
		},
		"duplicate identity": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 2,
					"Members": []any{
						map[string]any{"@odata.id": "/redfish/v1/Systems/1"},
						map[string]any{"@odata.id": "/redfish/v1/Systems/1"},
					},
				})
			},
			wantErr: "duplicate member identity",
		},
		"final count mismatch": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 2,
					"Members":             []any{map[string]any{"@odata.id": "/redfish/v1/Systems/1"}},
				})
			},
			wantErr: "advertised 2",
		},
		"members absent": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 0,
				})
			},
			wantErr: "no Members array",
		},
		"members null": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 0,
					"Members":             nil,
				})
			},
			wantErr: "no Members array",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			client := newTestProtocolClient(t, testConfig(server.URL, "none"))
			_, complete, err := client.fetchCollection(context.Background(), "/redfish/v1/Test", nil)
			assert.False(t, complete)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestFetchCollectionAcceptsSameOriginRedirectIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/Test":
			http.Redirect(w, r, "/redfish/v1/RedirectedTest", http.StatusTemporaryRedirect)
		case "/redfish/v1/RedirectedTest":
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/RedirectedTest",
				"@odata.type":         "#TestCollection.TestCollection",
				"Members@odata.count": 0,
				"Members":             []any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	members, complete, err := client.fetchCollection(context.Background(), "/redfish/v1/Test", nil)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Empty(t, members)
}

func TestFetchCollectionResumesPageRejectedByMemberBudget(t *testing.T) {
	t.Parallel()

	var firstPageRequests atomic.Int64
	var secondPageRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		memberID := "1"
		next := "/redfish/v1/Test?page=2"
		if r.URL.Query().Get("page") == "2" {
			secondPageRequests.Add(1)
			memberID = "2"
			next = ""
		} else {
			firstPageRequests.Add(1)
		}
		writeJSON(w, map[string]any{
			"@odata.id":              "/redfish/v1/Test",
			"@odata.type":            "#TestCollection.TestCollection",
			"Members@odata.count":    2,
			"Members@odata.nextLink": next,
			"Members": []any{
				map[string]any{"@odata.id": "/redfish/v1/Systems/" + memberID},
			},
		})
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	first := withOperationBudget(context.Background())
	require.NoError(t, consumeCollectionMemberBudget(first, maxCollectionMembers-1))
	members, complete, err := client.fetchCollection(first, "/redfish/v1/Test", nil)
	require.ErrorContains(t, err, "collection member work")
	assert.False(t, complete)
	require.Len(t, members, 1)
	assert.Equal(t, "/redfish/v1/Systems/1", members[0].ODataID)

	members, complete, err = client.fetchCollection(
		withOperationBudget(context.Background()),
		"/redfish/v1/Test",
		nil,
	)
	require.NoError(t, err)
	assert.True(t, complete)
	require.Len(t, members, 2)
	assert.Equal(t, "/redfish/v1/Systems/1", members[0].ODataID)
	assert.Equal(t, "/redfish/v1/Systems/2", members[1].ODataID)
	assert.Equal(t, int64(1), firstPageRequests.Load())
	assert.Equal(t, int64(2), secondPageRequests.Load())
}

func TestProtocolClientWorkBudgetStopsBeforeRequest(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	ctx := withOperationBudget(context.Background())
	operationBudgetFrom(ctx).requests.Store(maxCycleRequests)

	_, err := client.doOnce(
		ctx,
		protocolRequest{method: http.MethodGet, target: client.root, auth: requestAuth{}},
		nil,
	)
	require.ErrorContains(t, err, "request work")
	assert.Zero(t, requests.Load())
}

func TestFetchCollectionContinuesAfterInvalidMember(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Test",
				"@odata.type":         "#TestCollection.TestCollection",
				"Members@odata.count": 3,
				"Members": []any{
					map[string]any{"@odata.id": "/redfish/v1/Test/1"},
					map[string]any{"Name": "missing identity"},
				},
				"Members@odata.nextLink": "/redfish/v1/Test?page=2",
			})
		case "2":
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Test",
				"@odata.type":         "#TestCollection.TestCollection",
				"Members@odata.count": 3,
				"Members": []any{
					map[string]any{"@odata.id": "/redfish/v1/Test/2"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	members, complete, err := client.fetchCollection(
		context.Background(),
		"/redfish/v1/Test",
		nil,
	)
	require.ErrorContains(t, err, "collection member has no @odata.id")
	require.False(t, complete)
	require.Equal(t, []redfishLink{
		{ODataID: "/redfish/v1/Test/1"},
		{ODataID: "/redfish/v1/Test/2"},
	}, members)
}

func TestFetchCollectionContinuesAfterDuplicateMember(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"@odata.id":           "/redfish/v1/Test",
			"@odata.type":         "#TestCollection.TestCollection",
			"Members@odata.count": 2,
			"Members": []any{
				map[string]any{"@odata.id": "/redfish/v1/Test/1"},
				map[string]any{"@odata.id": "/redfish/v1/Test/1"},
				map[string]any{"@odata.id": "/redfish/v1/Test/2"},
			},
		})
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	members, complete, err := client.fetchCollection(
		context.Background(),
		"/redfish/v1/Test",
		nil,
	)
	require.ErrorContains(t, err, "duplicate member identity")
	require.False(t, complete)
	require.Equal(t, []redfishLink{
		{ODataID: "/redfish/v1/Test/1"},
		{ODataID: "/redfish/v1/Test/2"},
	}, members)
}

func TestProtocolClientUsesAdvertisedExpandedCollectionMembers(t *testing.T) {
	t.Parallel()

	var collectionRequests, memberRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/Systems":
			collectionRequests.Add(1)
			if got := r.URL.Query().Get("$expand"); got != "." {
				t.Errorf("$expand = %q, want %q", got, ".")
				http.Error(w, "unexpected expansion", http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Systems",
				"@odata.type":         "#ComputerSystemCollection.ComputerSystemCollection",
				"Members@odata.count": 1,
				"Members": []any{map[string]any{
					"@odata.id":   "/redfish/v1/Systems/1",
					"@odata.type": "#ComputerSystem.v1_20_0.ComputerSystem",
					"Id":          "1",
					"Name":        "System 1",
					"ExactTotal":  json.Number("9007199254740993"),
				}},
			})
		case "/redfish/v1/Systems/1":
			memberRequests.Add(1)
			http.Error(w, "expanded member should avoid this request", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	client.setExpansionValue(".")
	members, complete, err := client.fetchCollectionMembers(
		context.Background(),
		"/redfish/v1/Systems",
		"system",
		nil,
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, members, 1)
	assert.NotNil(t, members[0].Data)
	assert.NotEmpty(t, members[0].Raw)
	assert.Equal(t, json.Number("9007199254740993"), members[0].Data["ExactTotal"])
	assert.Equal(t, int64(1), collectionRequests.Load())
	assert.Zero(t, memberRequests.Load())
}

func TestProtocolClientDisablesRejectedExpansionAndFallsBackToLinks(t *testing.T) {
	t.Parallel()

	var expanded, ordinary atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1/Systems" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Has("$expand") {
			expanded.Add(1)
		} else {
			ordinary.Add(1)
		}
		serveCollection(w, "/redfish/v1/Systems", "/redfish/v1/Systems/1")
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	client.setExpansionValue(".")
	for range 2 {
		members, complete, err := client.fetchCollectionMembers(
			context.Background(),
			"/redfish/v1/Systems",
			"system",
			nil,
		)
		require.NoError(t, err)
		require.True(t, complete)
		require.Len(t, members, 1)
		assert.Nil(t, members[0].Data)
	}
	assert.Equal(t, int64(1), expanded.Load())
	assert.Equal(t, int64(2), ordinary.Load())
}

func TestCollectionExpansionFailureClassification(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		err  error
		want bool
	}{
		"bad request": {
			err:  statusError{status: http.StatusBadRequest, class: "protocol"},
			want: true,
		},
		"not implemented": {
			err:  statusError{status: http.StatusNotImplemented, class: "protocol"},
			want: true,
		},
		"service unavailable": {
			err: statusError{status: http.StatusServiceUnavailable, class: "protocol"},
		},
		"authentication": {
			err: statusError{status: http.StatusUnauthorized, class: "auth"},
		},
		"transport": {
			err: transportError{temporary: true},
		},
		"timeout": {
			err: transportError{timeout: true},
		},
		"caller deadline": {
			err: context.DeadlineExceeded,
		},
		"structural": {
			err:  errors.New("expanded collection member has no schema type"),
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, persistentCollectionExpansionFailure(test.err))
		})
	}
}

func TestProtocolClientResumesCollectionAtOpaqueNextPage(t *testing.T) {
	t.Parallel()

	var firstPageRequests, secondPageRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1/Test" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			if secondPageRequests.Add(1) == 1 {
				<-r.Context().Done()
				return
			}
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Test",
				"@odata.type":         "#ResourceCollection.ResourceCollection",
				"Members@odata.count": 2,
				"Members":             []any{map[string]any{"@odata.id": "/redfish/v1/Test/2"}},
			})
			return
		}
		firstPageRequests.Add(1)
		writeJSON(w, map[string]any{
			"@odata.id":              "/redfish/v1/Test",
			"@odata.type":            "#ResourceCollection.ResourceCollection",
			"Members@odata.count":    2,
			"Members":                []any{map[string]any{"@odata.id": "/redfish/v1/Test/1"}},
			"Members@odata.nextLink": "/redfish/v1/Test?page=2",
		})
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	members, complete, err := client.fetchCollection(ctx, "/redfish/v1/Test", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, complete)
	require.Len(t, members, 1)

	members, complete, err = client.fetchCollection(context.Background(), "/redfish/v1/Test", nil)
	require.NoError(t, err)
	assert.True(t, complete)
	require.Len(t, members, 2)
	assert.Equal(t, "/redfish/v1/Test/1", members[0].ODataID)
	assert.Equal(t, "/redfish/v1/Test/2", members[1].ODataID)
	assert.Equal(t, int64(1), firstPageRequests.Load())
	assert.Equal(t, int64(2), secondPageRequests.Load())
}

func TestProtocolClientRotatesMemberWorkAfterCancellation(t *testing.T) {
	t.Parallel()

	blocked := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/Systems/1" {
			select {
			case blocked <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			<-r.Context().Done()
			return
		}
		serveBaseResource(w, r.URL.Path, "#ComputerSystem.v1_20_0.ComputerSystem", r.URL.Path)
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	recorder := &requestRecordingTransport{base: client.http.Transport}
	client.http.Transport = recorder
	members := []collectionMember{
		{Ref: redfishLink{ODataID: "/redfish/v1/Systems/1"}},
		{Ref: redfishLink{ODataID: "/redfish/v1/Systems/2"}},
		{Ref: redfishLink{ODataID: "/redfish/v1/Systems/3"}},
	}
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := client.fetchBaseMembers(ctx, "base\x00system", "system", members, nil, true)
			result <- err
		}()
		select {
		case <-blocked:
		case <-time.After(asyncTestTimeout):
			cancel()
			t.Fatal("timed out waiting for blocked Redfish member request")
		}
		cancel()
		select {
		case err := <-result:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(asyncTestTimeout):
			t.Fatal("timed out waiting for canceled Redfish member collection")
		}
	}
	requests := recorder.paths()
	require.GreaterOrEqual(t, len(requests), 4)
	assert.Equal(t, []string{
		"/redfish/v1/Systems/1",
		"/redfish/v1/Systems/2",
		"/redfish/v1/Systems/3",
		"/redfish/v1/Systems/1",
	}, requests[:4])
}

func TestProtocolClientDoesNotSleepAfterFinalRetryableResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer server.Close()
	cfg := testConfig(server.URL, "none")
	cfg.Retries = new(0)
	client := newTestProtocolClient(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.do(
		ctx,
		protocolRequest{method: http.MethodGet, target: client.root, auth: requestAuth{}},
		nil,
		true,
		http.StatusOK,
	)
	var status statusError
	require.ErrorAs(t, err, &status)
	assert.Equal(t, http.StatusTooManyRequests, status.status)
}

func TestRetryAfterBoundsUntrustedHeaderValues(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{
		"oversized token":      strings.Repeat("9", maxRetryAfterBytes+1),
		"oversized whitespace": strings.Repeat(" ", maxRetryAfterBytes) + "1",
		"uint overflow":        strings.Repeat("9", maxRetryAfterBytes),
	} {
		t.Run(name, func(t *testing.T) {
			header := make(http.Header)
			header.Set("Retry-After", value)
			assert.Zero(t, retryAfter(header))
		})
	}

	header := make(http.Header)
	header.Set("Retry-After", "9223372036854775807")
	assert.Equal(t, maxRetryAfter, retryAfter(header))
}

func TestProtocolClientRetryBudgetDoesNotOverflow(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	defer server.Close()

	cfg := testConfig(server.URL, "none")
	cfg.Retries = new(int)
	*cfg.Retries = math.MaxInt
	client := newTestProtocolClient(t, cfg)

	response, err := client.do(
		context.Background(),
		protocolRequest{method: http.MethodGet, target: client.root, auth: requestAuth{}},
		nil,
		true,
		http.StatusOK,
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	response.finish(nil)
	assert.Equal(t, int64(2), requests.Load())
}

func TestCollectionProgressRetentionBoundsEmptyKeyChurn(t *testing.T) {
	client := &protocolClient{}
	budget := retainedStateBudget{entries: 2, members: 2}
	progress := collectionProgress{
		SeenPages:   make(map[string]struct{}),
		SeenMembers: make(map[string]struct{}),
	}

	require.True(t, client.saveCollectionProgressWithinBudget("first", progress, budget))
	require.True(t, client.saveCollectionProgressWithinBudget("second", progress, budget))
	assert.False(t, client.saveCollectionProgressWithinBudget("third", progress, budget))
	assert.Len(t, client.collectionProgress, 2)

	progress.Members = []collectionMember{{}, {}, {}}
	assert.False(t, client.saveCollectionProgressWithinBudget("first", progress, budget))
	assert.NotContains(t, client.collectionProgress, "first")
	assert.Contains(t, client.collectionProgress, "second")

	pageOnly := collectionProgress{
		SeenPages: map[string]struct{}{"page-1": {}, "page-2": {}, "page-3": {}},
	}
	assert.False(t, (&protocolClient{}).saveCollectionProgressWithinBudget(
		"invalid-pages",
		pageOnly,
		retainedStateBudget{entries: 1, members: 2},
	))

	withBody := collectionProgress{
		Members: []collectionMember{{
			Data: map[string]any{"large": strings.Repeat("x", 1024)},
			Raw:  []byte(strings.Repeat("x", 1024)),
		}},
	}
	bodyClient := &protocolClient{}
	require.True(t, bodyClient.saveCollectionProgressWithinBudget(
		"body",
		withBody,
		retainedStateBudget{entries: 1, members: 1},
	))
	assert.Nil(t, bodyClient.collectionProgress["body"].Members[0].Data)
	assert.Nil(t, bodyClient.collectionProgress["body"].Members[0].Raw)
}

func TestProtocolClientResponseGuards(t *testing.T) {
	t.Parallel()

	tests := map[string]http.HandlerFunc{
		"wrong content type": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`{}`))
		},
		"trailing json": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{} {}`))
		},
		"oversize body": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(make([]byte, maxResponseBodyBytes+1))
		},
	}

	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(handler)
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, "none"))
			require.Error(t, client.Check(context.Background()))
		})
	}
}

func TestResponseMetadataClassifiesCompatibilityHeaders(t *testing.T) {
	tests := map[string]struct {
		header          http.Header
		wantContentType string
		wantOData       string
	}{
		"valid": {
			header: http.Header{
				"Content-Type":  []string{"application/json; charset=utf-8"},
				"Odata-Version": []string{"4.0"},
			},
			wantContentType: "valid",
			wantOData:       "valid",
		},
		"missing": {
			header:          http.Header{},
			wantContentType: "missing",
			wantOData:       "missing",
		},
		"wrong media type": {
			header:          http.Header{"Content-Type": []string{"text/plain"}, "Odata-Version": []string{"3.0"}},
			wantContentType: "invalid",
			wantOData:       "invalid",
		},
		"wrong charset": {
			header:          http.Header{"Content-Type": []string{"application/json; charset=iso-8859-1"}},
			wantContentType: "invalid",
			wantOData:       "missing",
		},
		"malformed media type": {
			header:          http.Header{"Content-Type": []string{"application/json; charset"}},
			wantContentType: "invalid",
			wantOData:       "missing",
		},
		"oversize headers": {
			header: http.Header{
				"Content-Type":  []string{strings.Repeat(" ", maxContentTypeBytes) + "application/json"},
				"Odata-Version": []string{strings.Repeat(" ", maxODataVersionBytes) + "4.0"},
			},
			wantContentType: "invalid",
			wantOData:       "invalid",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := responseContentTypeState(test.header); got != test.wantContentType {
				t.Errorf("content type state = %q, want %q", got, test.wantContentType)
			}
			if got := responseODataVersionState(test.header); got != test.wantOData {
				t.Errorf("OData version state = %q, want %q", got, test.wantOData)
			}
		})
	}
}

func TestDecodeJSONToleratesCompatibilityHeadersAfterStructuralChecks(t *testing.T) {
	response := &responseData{
		header: http.Header{
			"Content-Type":  []string{"text/plain"},
			"Odata-Version": []string{"3.0"},
		},
		body: []byte(`{"@odata.id":"/redfish/v1/Managers/1"}`),
	}
	var decoded map[string]any
	require.NoError(t, decodeJSON(response, &decoded))
	assert.Equal(t, "/redfish/v1/Managers/1", decoded["@odata.id"])
}

func TestProtocolClientRetainsLastCompleteMembershipWithoutReplayingCurrentState(t *testing.T) {
	t.Parallel()

	var failSystems atomic.Bool
	server := newRedfishTestServer(t, redfishTestServerConfig{failSystems: &failSystems})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	first, err := client.Collect(context.Background())
	require.NoError(t, err)
	require.True(t, first.Complete)

	failSystems.Store(true)
	second, err := client.Collect(context.Background())
	require.Error(t, err)
	assert.False(t, second.Complete)
	var system map[string]any
	for _, row := range second.Inventory {
		if row["resource_kind"] == "system" {
			system = row
			break
		}
	}
	require.NotNil(t, system)
	assert.Equal(t, "unknown", system["acquisition_state"])
	assert.Equal(t, false, system["membership_complete"])
	assert.Nil(t, system["health"])
	assert.Nil(t, system["state"])
}

type redfishTestServerConfig struct {
	requireBasic                bool
	supportSession              bool
	sessionStatus               int
	rootRedirect                string
	rootHits                    *atomic.Int64
	failSystems                 *atomic.Bool
	expireSessionOnce           *atomic.Bool
	malformedSessionBody        bool
	badDirectSession            bool
	sessionDeleteStatus         int
	sessionDeleteStatusSequence []int
}

type redfishTestServer struct {
	*httptest.Server
	sessionCreates            atomic.Int64
	sessionDeletes            atomic.Int64
	basicRequests             atomic.Int64
	unsupportedSessionCreates atomic.Int64
}

func newRedfishTestServer(t *testing.T, cfg redfishTestServerConfig) *redfishTestServer {
	t.Helper()

	state := &redfishTestServer{}
	var mu sync.Mutex
	validTokens := map[string]bool{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get("X-Auth-Token"); token != "" && cfg.expireSessionOnce != nil &&
			cfg.expireSessionOnce.CompareAndSwap(true, false) {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		if cfg.rootHits != nil && r.URL.Path == "/redfish/v1/" {
			cfg.rootHits.Add(1)
		}
		if cfg.rootRedirect != "" && r.URL.Path == "/redfish/v1/" {
			http.Redirect(w, r, cfg.rootRedirect, http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path == "/redfish/v1/redirected" {
			serveServiceRootAt(w, r.URL.Path, cfg.supportSession || cfg.sessionStatus != 0)
			return
		}

		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == http.MethodPost {
			state.sessionCreates.Add(1)
			status := cfg.sessionStatus
			if status == 0 && cfg.supportSession {
				status = http.StatusCreated
			}
			if status == 0 {
				status = http.StatusNotFound
			}
			if status != http.StatusCreated {
				http.Error(w, http.StatusText(status), status)
				return
			}
			if !cfg.supportSession {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{}`))
				return
			}
			var got map[string]string
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode session request body: %v", err)
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			want := map[string]string{"UserName": "user", "Password": "test-password"}
			if !maps.Equal(got, want) {
				t.Errorf("session request body = %v, want %v", got, want)
				http.Error(w, "unexpected request body", http.StatusBadRequest)
				return
			}
			token := fmt.Sprintf("test-token-%d", state.sessionCreates.Load())
			mu.Lock()
			validTokens[token] = true
			mu.Unlock()
			w.Header().Set("X-Auth-Token", token)
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if cfg.malformedSessionBody {
				writeJSONBody(w, map[string]any{
					"@odata.id":   "/redfish/v1/SessionService/Sessions/1",
					"@odata.type": "#Session.v1_6_0.Session",
					"Id":          "1",
				})
				return
			}
			writeJSONBody(w, map[string]any{
				"@odata.id":   "/redfish/v1/SessionService/Sessions/1",
				"@odata.type": "#Session.v1_6_0.Session",
				"Id":          "1",
				"Name":        "Session",
			})
			return
		}
		if r.URL.Path == "/redfish/v1/UnsupportedSessions" && r.Method == http.MethodPost {
			state.unsupportedSessionCreates.Add(1)
			http.Error(w, "unsupported", http.StatusNotImplemented)
			return
		}
		if r.URL.Path == "/redfish/v1/SessionService/Sessions/1" && r.Method == http.MethodDelete {
			attempt := state.sessionDeletes.Add(1)
			if index := int(attempt - 1); index < len(cfg.sessionDeleteStatusSequence) {
				status := cfg.sessionDeleteStatusSequence[index]
				if status != http.StatusNoContent {
					http.Error(w, http.StatusText(status), status)
					return
				}
				w.WriteHeader(status)
				return
			}
			if cfg.sessionDeleteStatus != 0 {
				http.Error(w, http.StatusText(cfg.sessionDeleteStatus), cfg.sessionDeleteStatus)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/redfish/v1/SessionService" {
			writeJSON(w, map[string]any{
				"@odata.id":   r.URL.Path,
				"@odata.type": "#SessionService.v1_2_0.SessionService",
				"Id":          "SessionService",
				"Name":        "Session Service",
				"Sessions":    map[string]any{"@odata.id": "/redfish/v1/SessionService/Sessions"},
			})
			return
		}

		if r.URL.Path != "/redfish/v1/" {
			authenticated := false
			if user, password, ok := r.BasicAuth(); ok && user == "user" && password == "test-password" {
				state.basicRequests.Add(1)
				authenticated = true
			}
			mu.Lock()
			authenticated = authenticated || validTokens[r.Header.Get("X-Auth-Token")]
			mu.Unlock()
			if cfg.requireBasic || cfg.supportSession {
				if !authenticated {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}

		switch r.URL.Path {
		case "/redfish/v1/":
			serveServiceRootConfigured(
				w,
				"/redfish/v1/",
				cfg.supportSession || cfg.sessionStatus != 0,
				cfg.badDirectSession,
			)
		case "/redfish/v1/Systems":
			if cfg.failSystems != nil && cfg.failSystems.Load() {
				http.Error(w, "temporary failure", http.StatusServiceUnavailable)
				return
			}
			serveCollection(w, r.URL.Path, "/redfish/v1/Systems/1")
		case "/redfish/v1/Chassis":
			serveCollection(w, r.URL.Path, "/redfish/v1/Chassis/1")
		case "/redfish/v1/Managers":
			serveCollection(w, r.URL.Path, "/redfish/v1/Managers/1")
		case "/redfish/v1/Systems/1":
			serveBaseResource(w, r.URL.Path, "#ComputerSystem.v1_21_0.ComputerSystem", "System")
		case "/redfish/v1/Chassis/1":
			serveBaseResource(w, r.URL.Path, "#Chassis.v1_27_0.Chassis", "Chassis")
		case "/redfish/v1/Managers/1":
			serveBaseResource(w, r.URL.Path, "#Manager.v1_20_0.Manager", "Manager")
		default:
			http.NotFound(w, r)
		}
	})
	state.Server = httptest.NewServer(handler)
	return state
}

func serveServiceRoot(w http.ResponseWriter, session bool) {
	serveServiceRootConfigured(w, "/redfish/v1/", session, false)
}

func serveServiceRootAt(w http.ResponseWriter, resourceURI string, session bool) {
	serveServiceRootConfigured(w, resourceURI, session, false)
}

func serveServiceRootConfigured(
	w http.ResponseWriter,
	resourceURI string,
	session bool,
	badDirectSession bool,
) {
	document := map[string]any{
		"@odata.id":      resourceURI,
		"@odata.type":    "#ServiceRoot.v1_16_0.ServiceRoot",
		"Id":             "RootService",
		"Name":           "Root Service",
		"RedfishVersion": "1.20.0",
		"Systems":        map[string]any{"@odata.id": "/redfish/v1/Systems"},
		"Chassis":        map[string]any{"@odata.id": "/redfish/v1/Chassis"},
		"Managers":       map[string]any{"@odata.id": "/redfish/v1/Managers"},
	}
	if session {
		document["SessionService"] = map[string]any{"@odata.id": "/redfish/v1/SessionService"}
	}
	if badDirectSession {
		document["Links"] = map[string]any{
			"Sessions": map[string]any{"@odata.id": "/redfish/v1/UnsupportedSessions"},
		}
	}
	writeJSON(w, document)
}

func serveCollection(w http.ResponseWriter, resourceURI string, members ...string) {
	values := make([]map[string]any, 0, len(members))
	for _, member := range members {
		values = append(values, map[string]any{"@odata.id": member})
	}
	collectionType := map[string]string{
		"/redfish/v1/Systems":  "ComputerSystemCollection",
		"/redfish/v1/Chassis":  "ChassisCollection",
		"/redfish/v1/Managers": "ManagerCollection",
	}[strings.TrimSuffix(resourceURI, "/")]
	if collectionType == "" {
		collectionType = "ResourceCollection"
	}
	writeJSON(w, map[string]any{
		"@odata.id":           resourceURI,
		"@odata.type":         "#" + collectionType + "." + collectionType,
		"Members@odata.count": len(values),
		"Members":             values,
	})
}

func serveBaseResource(w http.ResponseWriter, resourceURI, schemaType, name string) {
	writeJSON(w, map[string]any{
		"@odata.id":   resourceURI,
		"@odata.type": schemaType,
		"Id":          "1",
		"Name":        name,
		"Status":      map[string]any{"Health": "OK", "State": "Enabled"},
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	writeJSONBody(w, value)
}

func writeJSONBody(w http.ResponseWriter, value any) {
	_ = json.NewEncoder(w).Encode(value)
}

func testConfig(rawURL, auth string) Config {
	cfg := Config{
		URL:        rawURL,
		NodeMode:   "local",
		SystemURI:  "/redfish/v1/Systems/1",
		AuthMethod: auth,
		Username:   "user",
		Password:   "test-password",
	}
	if auth == "none" {
		cfg.Username = ""
		cfg.Password = ""
	}
	cfg.applyDefaults()
	return cfg
}

func newTestProtocolClient(t *testing.T, cfg Config) *protocolClient {
	t.Helper()
	client, err := newHTTPClient(cfg)
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	endpoint, err := newEndpointClient(cfg, client)
	require.NoError(t, err)
	return endpoint.(*protocolClient)
}
