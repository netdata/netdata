// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish"
	"github.com/stretchr/testify/require"
)

func TestProbeTransportErrorsDoNotDiscloseUnderlyingText(t *testing.T) {
	const secret = "test-proxy-password"
	err := sanitizeProbeTransportError(errors.New("proxy authentication failed for " + secret))
	require.ErrorContains(t, err, "transport error")
	require.NotContains(t, err.Error(), secret)
}

func TestProbeStatusErrorsDoNotDiscloseServerReasonPhrase(t *testing.T) {
	const secret = "test-server-secret"
	err := probeHTTPStatusError("ServiceRoot", &http.Response{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 " + secret,
	})
	require.EqualError(t, err, "ServiceRoot returned HTTP 401")
	require.NotContains(t, err.Error(), secret)
}

func TestValidateServiceRoot(t *testing.T) {
	responseURL, err := url.Parse("https://192.0.2.1/redfish/v1/")
	require.NoError(t, err)
	valid := []byte(`{
		"@odata.id":"/redfish/v1/",
		"@odata.type":"#ServiceRoot.v1_16_0.ServiceRoot",
		"Id":"RootService",
		"Name":"Root Service",
		"RedfishVersion":"1.20.0",
		"Systems":{"@odata.id":"/redfish/v1/Systems"}
	}`)
	require.NoError(t, redfish.ValidateDiscoveryServiceRoot(valid, responseURL, responseURL.String()))

	for name, test := range map[string]struct {
		payload string
		wantErr string
	}{
		"missing identity": {
			payload: `{"@odata.type":"#ServiceRoot.v1_16_0.ServiceRoot","Id":"RootService","Name":"Root Service","RedfishVersion":"1.20.0","Systems":{"@odata.id":"/redfish/v1/Systems"}}`,
			wantErr: "@odata.id does not identify",
		},
		"cross origin": {
			payload: `{"@odata.id":"/redfish/v1/","@odata.type":"#ServiceRoot.v1_16_0.ServiceRoot","Id":"RootService","Name":"Root Service","RedfishVersion":"1.20.0","Systems":{"@odata.id":"https://example.com/redfish/v1/Systems"}}`,
			wantErr: "no valid Systems, Chassis, or Managers link",
		},
		"no structural link": {
			payload: `{"@odata.id":"/redfish/v1/","@odata.type":"#ServiceRoot.v1_16_0.ServiceRoot","Id":"RootService","Name":"Root Service","RedfishVersion":"1.20.0"}`,
			wantErr: "no valid Systems, Chassis, or Managers link",
		},
		"trailing JSON": {
			payload: `{"@odata.id":"/redfish/v1/","@odata.type":"#ServiceRoot.v1_16_0.ServiceRoot","Id":"RootService","Name":"Root Service","RedfishVersion":"1.20.0","Systems":{"@odata.id":"/redfish/v1/Systems"}} {}`,
			wantErr: "trailing JSON data",
		},
		"missing type": {
			payload: `{"@odata.id":"/redfish/v1/","Id":"RootService","Name":"Root Service","RedfishVersion":"1.20.0","Systems":{"@odata.id":"/redfish/v1/Systems"}}`,
			wantErr: "no valid @odata.type",
		},
		"wrong type": {
			payload: `{"@odata.id":"/redfish/v1/","@odata.type":"#Manager.v1_0_0.Manager","Id":"RootService","Name":"Root Service","RedfishVersion":"1.20.0","Systems":{"@odata.id":"/redfish/v1/Systems"}}`,
			wantErr: "unexpected @odata.type",
		},
		"missing version": {
			payload: `{"@odata.id":"/redfish/v1/","@odata.type":"#ServiceRoot.v1_16_0.ServiceRoot","Id":"RootService","Name":"Root Service","Systems":{"@odata.id":"/redfish/v1/Systems"}}`,
			wantErr: "no valid RedfishVersion",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := redfish.ValidateDiscoveryServiceRoot([]byte(test.payload), responseURL, responseURL.String())
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestProbeCandidateIsUnauthenticated(t *testing.T) {
	type requestHeaders struct {
		authorization string
		token         string
		odataVersion  string
	}
	headers := make(chan requestHeaders, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- requestHeaders{
			authorization: r.Header.Get("Authorization"),
			token:         r.Header.Get("X-Auth-Token"),
			odataVersion:  r.Header.Get("OData-Version"),
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"@odata.id":"/redfish/v1/",
			"@odata.type":"#ServiceRoot.v1_16_0.ServiceRoot",
			"Id":"RootService",
			"Name":"Root Service",
			"RedfishVersion":"1.20.0",
			"Managers":{"@odata.id":"/redfish/v1/Managers"}
		}`))
	}))
	defer server.Close()

	candidate := endpointCandidate{
		url: server.URL + "/redfish/v1/",
		profile: ProfileConfig{ProbeConfig: map[string]any{
			"auth_method": "auto",
			"username":    "not-sent",
			"password":    "not-sent",
			"timeout":     "2s",
		}},
	}
	require.NoError(t, probeCandidate(context.Background(), candidate))
	got := <-headers
	require.Empty(t, got.authorization)
	require.Empty(t, got.token)
	require.Equal(t, "4.0", got.odataVersion)
}

func TestProbeCandidateRejectsUnsafeSameOriginRedirectWithoutAuthorization(t *testing.T) {
	var redirected atomic.Bool
	var location atomic.Pointer[string]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			target := location.Load()
			if target == nil {
				http.Error(w, "redirect target is not initialized", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Location", *target)
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		redirected.Store(true)
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Errorf("redirected request Authorization header = %q, want empty", authorization)
			http.Error(w, "unexpected authorization", http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	redirectTarget := "//chosen-user:chosen-password@" + strings.TrimPrefix(server.URL, "http://") + "/redfish/v1/"
	location.Store(&redirectTarget)

	err := probeCandidate(context.Background(), endpointCandidate{
		url: server.URL + "/redfish/v1/",
		profile: ProfileConfig{ProbeConfig: map[string]any{
			"auth_method": "none",
		}},
	})
	require.ErrorContains(t, err, "user-info")
	require.False(t, redirected.Load())
}

func TestProbeCandidateRejectsAsyncAndCrossOriginRedirect(t *testing.T) {
	async := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer async.Close()
	require.ErrorContains(t, probeCandidate(context.Background(), endpointCandidate{
		url: async.URL + "/redfish/v1/",
		profile: ProfileConfig{ProbeConfig: map[string]any{
			"auth_method": "none",
		}},
	}), "HTTP 202")

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "https://example.com/redfish/v1/", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	require.Error(t, probeCandidate(context.Background(), endpointCandidate{
		url: redirect.URL + "/redfish/v1/",
		profile: ProfileConfig{ProbeConfig: map[string]any{
			"auth_method": "none",
		}},
	}))
}

func TestDiscoveryTransportPreservesAmbientAndExplicitProxyPolicy(t *testing.T) {
	ambient, err := redfish.NewDiscoveryHTTPClient(
		map[string]any{"auth_method": "none"},
		"http://192.0.2.2/redfish/v1/",
	)
	require.NoError(t, err)
	transport, ok := ambient.Transport.(*http.Transport)
	require.True(t, ok)
	require.Equal(
		t,
		reflect.ValueOf(http.ProxyFromEnvironment).Pointer(),
		reflect.ValueOf(transport.Proxy).Pointer(),
		"omitted proxy_url must retain the standard HTTP_PROXY/HTTPS_PROXY/NO_PROXY policy",
	)

	explicit, err := redfish.NewDiscoveryHTTPClient(
		map[string]any{
			"auth_method": "none",
			"proxy_url":   "http://127.0.0.1:28080",
		},
		"http://192.0.2.1/redfish/v1/",
	)
	require.NoError(t, err)
	explicitTransport, ok := explicit.Transport.(*http.Transport)
	require.True(t, ok)
	request, err := http.NewRequest(http.MethodGet, "http://192.0.2.1/redfish/v1/", nil)
	require.NoError(t, err)
	proxy, err := explicitTransport.Proxy(request)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:28080", proxy.String())

	const secretProxy = "http://user:secret@[invalid"
	_, err = redfish.NewDiscoveryHTTPClient(
		map[string]any{"auth_method": "none", "proxy_url": secretProxy},
		"http://192.0.2.1/redfish/v1/",
	)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
}
