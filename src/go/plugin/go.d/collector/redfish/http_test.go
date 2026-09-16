// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
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
	assert.True(t, retryableTransport(retryable))

	timeout := sanitizeTransportError(sensitiveNetError{
		timeout: true,
	})
	assert.NotContains(t, timeout.Error(), "test-password")
	assert.Equal(t, "timeout", classifyError(timeout))
	assert.False(t, retryableTransport(timeout))
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
		context.Background(),
		protocolRequest{
			method: http.MethodGet,
			target: target,
			auth:   client.currentAuth(false),
		},
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
		context.Background(),
		protocolRequest{
			method: http.MethodGet,
			target: target,
			auth:   client.currentAuth(false),
		},
		nil,
		false,
		http.StatusOK,
	)
	require.ErrorContains(t, err, "changed an authorized query")
	assert.Zero(t, redirected.Load())
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
		protocolRequest{
			method: http.MethodGet,
			target: client.root,
			auth:   requestAuth{},
		},
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
		protocolRequest{
			method: http.MethodGet,
			target: client.root,
			auth:   requestAuth{},
		},
		nil,
		true,
		http.StatusOK,
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	response.finish(nil)
	assert.Equal(t, int64(2), requests.Load())
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
	require.Equal(
		t,
		[]string{"Redfish compatibility: response Content-Type header is invalid"},
		client.takeCompatibilityDiagnostics(),
	)
}
