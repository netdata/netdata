// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolClientSessionLifecycle(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession: true,
	})
	defer server.Close()

	cfg := testConfig(server.URL, "session")
	client := newTestProtocolClient(t, cfg)
	require.NoError(t, client.Check(context.Background()))

	result, err := client.Collect(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	require.NoError(t, client.Close(context.Background()))
	assert.Zero(t, server.activeSessions.Load())
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	assert.Equal(t, int64(1), server.sessionDeletes.Load())
}

func TestProtocolClientCheckOnlyReadsServiceRoot(t *testing.T) {
	for _, method := range []string{"auto", "session", "basic", "none"} {
		t.Run(method, func(t *testing.T) {
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.RequestURI())
				assert.Empty(t, r.Header.Get("X-Auth-Token"))
				if method == "basic" {
					user, password, ok := r.BasicAuth()
					assert.True(t, ok)
					assert.Equal(t, "user", user)
					assert.Equal(t, "test-password", password)
				} else {
					assert.Empty(t, r.Header.Get("Authorization"))
				}
				serveServiceRoot(w, true)
			}))
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, method))
			require.NoError(t, client.Check(context.Background()))
			require.NoError(t, client.Check(context.Background()))
			require.NoError(t, client.Close(context.Background()))
			assert.Equal(t, []string{"GET /redfish/v1/", "GET /redfish/v1/"}, requests)
			assert.False(t, client.authenticationInitialized())
		})
	}
}

func TestProtocolClientCheckDoesNotRefreshAnExpiredSession(t *testing.T) {
	var expire atomic.Bool
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:    true,
		expireSessionOnce: &expire,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	_, err := client.Collect(context.Background())
	require.NoError(t, err)
	expire.Store(true)
	require.NoError(t, client.Check(context.Background()))
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	assert.Zero(t, server.sessionDeletes.Load())
	assert.True(t, expire.Load(), "Check must not use an active session token")
	expire.Store(false)
	require.NoError(t, client.Close(context.Background()))
}

func TestProtocolClientSessionCredentialsValidatedOnlyDuringCollect(t *testing.T) {
	server := newRedfishTestServer(t, redfishTestServerConfig{
		sessionStatus: http.StatusUnauthorized,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(context.Background()))
	assert.Zero(t, server.sessionCreates.Load())
	result, err := client.Collect(context.Background())
	require.Error(t, err)
	assert.Equal(t, "unavailable", result.Metrics.Status)
	assert.Empty(t, result.Hardware)
	assert.Zero(t, server.basicRequests.Load(), "session mode must not fall back to Basic")
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	require.NoError(t, client.Close(context.Background()))
}

func TestProtocolClientCleansMalformedCreatedSession(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:       true,
		malformedSessionBody: true,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.Error(t, client.initializeAuthentication(context.Background(), nil))
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
	require.Error(t, client.initializeAuthentication(context.Background(), nil))
	require.ErrorContains(
		t,
		client.initializeAuthentication(context.Background(), nil),
		"retire unactivated Redfish session",
	)
	assert.Equal(t, int64(1), server.sessionCreates.Load())
	assert.Equal(t, int64(2), server.sessionDeletes.Load())
	require.Error(t, client.Close(context.Background()))
}

func TestProtocolClientRetriesSessionCreationAfterPendingSessionIsAlreadyGone(t *testing.T) {
	t.Parallel()
	for name, firstStatus := range map[string]int{"cleanup failed": http.StatusInternalServerError, "session expired": http.StatusUnauthorized} {
		t.Run(name, func(t *testing.T) {
			server := newRedfishTestServer(t, redfishTestServerConfig{
				supportSession:              true,
				malformedSessionBody:        true,
				sessionDeleteStatusSequence: []int{firstStatus, http.StatusNotFound, http.StatusNoContent},
			})
			t.Cleanup(server.Close)
			client := newTestProtocolClient(t, testConfig(server.URL, "session"))
			require.Error(t, client.initializeAuthentication(t.Context(), nil))
			err := client.initializeAuthentication(t.Context(), nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "retire unactivated Redfish session")
			assert.Equal(t, int64(2), server.sessionCreates.Load())
			assert.Equal(t, int64(3), server.sessionDeletes.Load())
			assert.Zero(t, server.activeSessions.Load())
		})
	}
}

func TestProtocolClientTriesEveryAdvertisedSessionPath(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:   true,
		badDirectSession: true,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.initializeAuthentication(context.Background(), nil))
	require.NoError(t, client.Close(context.Background()))
	assert.Zero(t, server.activeSessions.Load())
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

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession: true,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.initializeAuthentication(context.Background(), nil))
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
	_, err := client.Collect(context.Background())
	require.NoError(t, err)
	expireOnce.Store(true)

	result, err := client.Collect(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	require.NoError(t, client.Close(context.Background()))
	assert.Zero(t, server.activeSessions.Load())
	assert.Equal(t, int64(2), server.sessionCreates.Load())
	assert.Equal(t, int64(2), server.sessionDeletes.Load())
}

func TestProtocolClientRecoversFromRepeatedSessionExpiration(t *testing.T) {
	var expire atomic.Bool
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:              true,
		expireSessionOnce:           &expire,
		sessionDeleteStatusSequence: []int{http.StatusUnauthorized, http.StatusUnauthorized, http.StatusNoContent},
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(context.Background()))
	_, err := client.Collect(context.Background())
	require.NoError(t, err)
	for range 2 {
		expire.Store(true)
		result, err := client.Collect(context.Background())
		require.NoError(t, err)
		assert.True(t, result.Complete)
	}
	require.NoError(t, client.Close(context.Background()))
	assert.Zero(t, server.activeSessions.Load())
	assert.Equal(t, int64(3), server.sessionCreates.Load())
	assert.Equal(t, int64(3), server.sessionDeletes.Load())
}

func TestProtocolClientSessionRecoveryAccountsForEveryWireOperation(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession: true,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.initializeAuthentication(context.Background(), nil))
	auth := client.currentAuth(true)
	stats := &wireStats{
		failures: make(map[string]int),
	}

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
			_, err := client.Collect(context.Background())
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantBasic, server.basicRequests.Load() > 0)
		})
	}
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
	stats := &wireStats{
		failures: make(map[string]int),
	}

	require.NoError(t, client.initializeSession(context.Background(), root, stats))
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
