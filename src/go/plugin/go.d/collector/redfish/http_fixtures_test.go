// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type redfishTestServerConfig struct {
	requireBasic         bool
	supportSession       bool
	sessionStatus        int
	rootRedirect         string
	rootHits             *atomic.Int64
	failSystems          *atomic.Bool
	expireSessionOnce    *atomic.Bool
	malformedSessionBody bool
	sessionDeleteStatus  int
	handleRequest        func(http.ResponseWriter, *http.Request) bool
}

type redfishTestServer struct {
	*httptest.Server
	sessionCreates atomic.Int64
	sessionDeletes atomic.Int64
	activeSessions atomic.Int64
	basicRequests  atomic.Int64
}

func newRedfishTestServer(t *testing.T, cfg redfishTestServerConfig) *redfishTestServer {
	t.Helper()

	state := &redfishTestServer{}
	var mu sync.Mutex
	sessions := map[string]string{}
	issuedSessions := map[string]string{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.handleRequest != nil && cfg.handleRequest(w, r) {
			return
		}
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
			sessionID := state.sessionCreates.Add(1)
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
			token := fmt.Sprintf("test-token-%d", sessionID)
			mu.Lock()
			sessionURI := fmt.Sprintf("/redfish/v1/SessionService/Sessions/%d", sessionID)
			sessions[token] = sessionURI
			issuedSessions[token] = sessionURI
			state.activeSessions.Add(1)
			mu.Unlock()
			w.Header().Set("X-Auth-Token", token)
			w.Header().Set("Location", sessionURI)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if cfg.malformedSessionBody {
				writeJSONBody(w, map[string]any{
					"@odata.id":   sessionURI,
					"@odata.type": "#Session.v1_6_0.Session",
					"Id":          "1",
				})
				return
			}
			writeJSONBody(w, map[string]any{
				"@odata.id":   sessionURI,
				"@odata.type": "#Session.v1_6_0.Session",
				"Id":          "1",
				"Name":        "Session",
			})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/redfish/v1/SessionService/Sessions/") && r.Method == http.MethodDelete {
			token := r.Header.Get("X-Auth-Token")
			mu.Lock()
			defer mu.Unlock()
			if token == "" || issuedSessions[token] != r.URL.Path {
				t.Errorf("session cleanup used the wrong token/session pair")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			state.sessionDeletes.Add(1)
			status := cfg.sessionDeleteStatus
			if status == 0 {
				status = http.StatusNoContent
			}
			switch status {
			case http.StatusOK, http.StatusNoContent, http.StatusUnauthorized, http.StatusNotFound, http.StatusGone:
				// Unauthorized/gone injection models a session already expired on the BMC.
				if _, active := sessions[token]; active {
					delete(sessions, token)
					state.activeSessions.Add(-1)
				}
			}
			w.WriteHeader(status)
			return
		}

		if r.URL.Path != "/redfish/v1/" {
			authenticated := false
			if user, password, ok := r.BasicAuth(); ok && user == "user" && password == "test-password" {
				state.basicRequests.Add(1)
				authenticated = true
			}
			mu.Lock()
			authenticated = authenticated || sessions[r.Header.Get("X-Auth-Token")] != ""
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
			serveServiceRootAt(
				w,
				"/redfish/v1/",
				cfg.supportSession || cfg.sessionStatus != 0,
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
	serveServiceRootAt(w, "/redfish/v1/", session)
}

func serveServiceRootAt(
	w http.ResponseWriter,
	resourceURI string,
	session bool,
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
		document["Links"] = map[string]any{
			"Sessions": map[string]any{"@odata.id": "/redfish/v1/SessionService/Sessions"},
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
	client, err := newHTTPClient(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	endpoint, err := newEndpointClient(cfg, client)
	require.NoError(t, err)
	return endpoint.(*protocolClient)
}

func fetchTestCollection(
	c *protocolClient,
	ctx context.Context,
	ref string,
	stats *wireStats,
) ([]redfishLink, bool, error) {
	members, complete, err := c.fetchCollectionMembers(ctx, ref, stats)
	result := make([]redfishLink, len(members))
	for i, member := range members {
		result[i] = member.Ref
	}
	return result, complete, err
}

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

// Resource unit tests supply only the resource under test; SDK connections also
// read the ServiceRoot. Lifecycle tests use their own full endpoint fixtures.
func newResourceTestServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			serveServiceRoot(w, false)
			return
		}
		handler.ServeHTTP(w, r)
	}))
}

func newTestResourceClient(t *testing.T, cfg Config) *protocolClient {
	t.Helper()
	c := newTestProtocolClient(t, cfg)
	require.NoError(t, c.initializeAuthentication(t.Context(), nil))
	return c
}
