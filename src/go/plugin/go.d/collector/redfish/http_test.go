// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeTransportErrorPreservesPolicyWithoutLeakingDetails(t *testing.T) {
	t.Parallel()

	retryable := sanitizeTransportError(sensitiveNetError{
		temporary: true,
	})
	assert.NotContains(t, retryable.Error(), "test-password")
	assert.Equal(t, "transport", classifyError(retryable))

	timeout := sanitizeTransportError(sensitiveNetError{
		timeout: true,
	})
	assert.NotContains(t, timeout.Error(), "test-password")
	assert.Equal(t, "timeout", classifyError(timeout))
}

func TestProtocolClientRejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	var foreignRequests atomic.Int64
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		foreignRequests.Add(1)
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

	for _, method := range []string{"none", "basic"} {
		t.Run(method, func(t *testing.T) {
			client := newTestProtocolClient(t, testConfig(server.URL, method))
			err := client.Check(context.Background())
			require.ErrorContains(t, err, "crosses the configured origin")
			_, err = client.Collect(context.Background())
			require.ErrorContains(t, err, "crosses the configured origin")
		})
	}
	assert.Zero(t, foreignRequests.Load())
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
	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("$top") == "1" {
			http.Redirect(w, r, "/redfish/v1/Systems?$top=2", http.StatusTemporaryRedirect)
			return
		}
		redirected.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	target, err := client.resolveURI(client.root, "/redfish/v1/Systems?$top=1", true)
	require.NoError(t, err)

	_, err = client.get(context.Background(), target, nil)
	require.ErrorContains(t, err, "changed an authorized query")
	assert.Zero(t, redirected.Load())
}

func TestProtocolClientRejectsRedirectThatRemovesAuthorizedQuery(t *testing.T) {
	t.Parallel()

	var redirected atomic.Int64
	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			http.Redirect(w, r, "/redfish/v1/Systems", http.StatusTemporaryRedirect)
			return
		}
		redirected.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	target, err := client.resolveURI(client.root, "/redfish/v1/Systems?$top=1", true)
	require.NoError(t, err)

	_, err = client.get(context.Background(), target, nil)
	require.ErrorContains(t, err, "changed an authorized query")
	assert.Zero(t, redirected.Load())
}

func TestProtocolClientResponseGuards(t *testing.T) {
	t.Parallel()

	tests := map[string]http.HandlerFunc{
		"invalid service root": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
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

type sensitiveNetError struct {
	timeout   bool
	temporary bool
}

func (e sensitiveNetError) Error() string { return "proxy-user:test-password@private-proxy" }

func (e sensitiveNetError) Timeout() bool { return e.timeout }

func (e sensitiveNetError) Temporary() bool { return e.temporary }

func TestClassifiedErrorsDoNotDependOnMessageText(t *testing.T) {
	for _, test := range []struct {
		err   error
		class string
	}{
		{classifiedError{"limit", "response too large"}, "limit"},
		{classifiedError{"tls", "peer verification failed"}, "tls"},
		{errors.New("resource label mentions certificate and limit"), "protocol"},
	} {
		assert.Equal(t, test.class, classifyError(test.err))
	}
}

func TestProtocolClientCheckAcceptsJSONWithIncompatibleContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("OData-Version", "4.0")
		writeJSONBody(
			w,
			sourceTestResource("/redfish/v1/", "ServiceRoot", "Service", map[string]any{"RedfishVersion": "1.20.0"}),
		)
	}))
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	require.NoError(t, client.Check(t.Context()))
}

func TestSDKAcquisitionPreservesCounterTokensAndResponseTime(t *testing.T) {
	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/Metrics":
			_, _ = w.Write(
				[]byte(
					`{"@odata.id":"/redfish/v1/Metrics","@odata.type":"#PortMetrics.v1_0_0.PortMetrics","RXBytes":18446744073709551001,"TXBytes":0,"RXFrames":"invalid optional property"}`,
				),
			)
		case "/redfish/v1/Manager":
			writeJSON(
				w,
				sourceTestResource(
					r.URL.Path,
					"Manager",
					"Clock",
					map[string]any{"DateTime": time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)},
				),
			)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	node, err := client.fetchGraphNode(t.Context(), "port_metrics", "/redfish/v1/Metrics", graphRelationship{}, nil)
	require.NoError(t, err)
	assert.Equal(t, json.Number("18446744073709551001"), node.Data["RXBytes"])
	assert.Equal(t, json.Number("0"), node.Data["TXBytes"])
	node, err = client.fetchGraphNode(t.Context(), "manager", "/redfish/v1/Manager", graphRelationship{}, nil)
	require.NoError(t, err)
	clock, present, diagnostic := managerClockValue(node)
	require.True(t, present)
	require.True(t, clock.Valid, diagnostic)
	assert.InDelta(t, 60, clock.Value, 5)
}

func TestSDKCollectionHonorsRequestConcurrency(t *testing.T) {
	for _, serial := range []bool{false, true} {
		t.Run(fmt.Sprintf("serial=%v", serial), func(t *testing.T) {
			var active, peak atomic.Int64
			const root = "/redfish/v1/"
			const chassis = root + "Chassis/1"
			const sensors = chassis + "/Sensors"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case root:
					writeJSON(w, sourceTestResource(root, "ServiceRoot", "Root", map[string]any{
						"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(root + "Chassis"),
						"ProtocolFeaturesSupported": map[string]any{"MultipleHTTPRequests": !serial},
					}))
				case root + "Chassis":
					writeJSON(w, sourceTestCollection(r.URL.Path, "Chassis", chassis))
				case chassis:
					writeJSON(
						w,
						sourceTestResource(
							chassis,
							"Chassis",
							"Chassis",
							map[string]any{"Sensors": sourceTestLink(sensors)},
						),
					)
				case sensors:
					writeJSON(
						w,
						sourceTestCollection(sensors, "Sensor", sensors+"/1", sensors+"/2", sensors+"/3", sensors+"/4"),
					)
				default:
					n := active.Add(1)
					defer active.Add(-1)
					for previous := peak.Load(); n > previous; previous = peak.Load() {
						if peak.CompareAndSwap(previous, n) {
							break
						}
					}
					// Keep responses overlapping long enough to observe the configured ceiling.
					time.Sleep(10 * time.Millisecond)
					writeJSON(
						w,
						sourceTestResource(
							r.URL.Path,
							"Sensor",
							"Sensor",
							map[string]any{"Reading": 0, "ReadingType": "Temperature", "ReadingUnits": "Cel"},
						),
					)
				}
			}))
			defer server.Close()
			cfg := testConfig(server.URL, "none")
			cfg.MaxConcurrentRequests = 2
			client := newTestProtocolClient(t, cfg)
			result, err := client.Collect(t.Context())
			require.NoError(t, err)
			require.True(t, result.Complete)
			if serial {
				assert.Equal(t, int64(1), peak.Load())
			} else {
				assert.Equal(t, int64(2), peak.Load())
			}
		})
	}
}

func TestSDKValidatesSessionLocationBeforeOriginIsDiscarded(t *testing.T) {
	for _, style := range []string{"relative", "absolute", "foreign"} {
		t.Run(style, func(t *testing.T) {
			var endpoint string
			var deleted atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					serveServiceRoot(w, true)
				case http.MethodPost:
					location := "/redfish/v1/SessionService/Sessions/1"
					if style == "absolute" {
						location = endpoint + location
					}
					if style == "foreign" {
						location = "https://foreign.invalid" + location
					}
					w.Header().Set("X-Auth-Token", "test-token")
					w.Header().Set("Location", location)
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{}`))
				case http.MethodDelete:
					assert.Equal(t, "/redfish/v1/SessionService/Sessions/1", r.URL.Path)
					assert.Equal(t, "test-token", r.Header.Get("X-Auth-Token"))
					deleted.Add(1)
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			endpoint = server.URL
			client := newTestProtocolClient(t, testConfig(endpoint, "session"))
			err := client.initializeAuthentication(t.Context(), nil)
			if style == "foreign" {
				require.ErrorContains(t, err, "invalid Location")
			} else {
				require.NoError(t, err)
			}
			client.Close()
			if style == "foreign" {
				assert.Zero(t, deleted.Load())
			} else {
				assert.Equal(t, int64(1), deleted.Load())
			}
		})
	}
}
