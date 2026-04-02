// SPDX-License-Identifier: GPL-3.0-or-later

package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultInsecureClient_PreservesDefaultTransportBehavior(t *testing.T) {
	client := VaultInsecureClient(5 * time.Second)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, defaultTransport.Proxy)

	assert.Equal(t, 5*time.Second, client.Timeout)
	require.NotNil(t, client.CheckRedirect)

	assert.NotSame(t, defaultTransport, transport)
	assert.NotNil(t, transport.Proxy)
	assert.Equal(t, defaultTransport.MaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, defaultTransport.IdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, defaultTransport.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)

	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestVaultClient_DoesNotFollowTemporaryRedirects(t *testing.T) {
	tokenSeen := make(chan string, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenSeen <- r.Header.Get("X-Vault-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	client := VaultClient(5 * time.Second)
	req, err := http.NewRequest(http.MethodGet, redirect.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Vault-Token", "vault-token")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	select {
	case seen := <-tokenSeen:
		t.Fatalf("unexpected redirected request carrying token %q", seen)
	default:
	}
}

func TestNoProxyClient_PreservesDefaultTransportBehavior(t *testing.T) {
	client := NoProxyClient(2 * time.Second)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)

	assert.Equal(t, 2*time.Second, client.Timeout)
	assert.NotSame(t, defaultTransport, transport)
	assert.Nil(t, transport.Proxy)
	assert.Equal(t, defaultTransport.MaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, defaultTransport.IdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, defaultTransport.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)
}
