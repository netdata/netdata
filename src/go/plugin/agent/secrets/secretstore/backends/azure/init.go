// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

const azureKeyVaultScope = "https://vault.azure.net/.default"

func (s *store) init(_ context.Context) error {
	switch {
	case s.Config.Timeout.Duration() < 0:
		return fmt.Errorf("timeout cannot be negative")
	case s.Config.Timeout.Duration() == 0:
		s.Config.Timeout = defaultTimeout
	}

	if err := s.Config.ValidateWithPath(""); err != nil {
		return err
	}
	s.runtime = &runtime{
		apiClient:  httpx.APIClient(s.Config.Timeout.Duration()),
		imdsClient: httpx.NoProxyClient(s.Config.Timeout.Duration()),
	}

	cred, err := s.Config.NewCredentialWithOptions(s.credentialOptions())
	if err != nil {
		return fmt.Errorf("creating azure credential for kind %q: %w", secretstore.KindAzureKV, err)
	}
	cred = credentialWithTimeout{
		cred:    cred,
		timeout: s.authTimeout(),
	}

	tokenProvider, err := cloudauth.NewTokenProvider(
		cred,
		[]string{azureKeyVaultScope},
		cloudauth.DefaultTokenRefreshMargin,
	)
	if err != nil {
		return fmt.Errorf("creating azure token provider for kind %q: %w", secretstore.KindAzureKV, err)
	}

	s.published = &publishedStore{
		runtime:       s.runtime,
		tokenProvider: tokenProvider,
	}
	return nil
}

func (s *store) authTimeout() time.Duration {
	switch s.Config.NormalizedMode() {
	case cloudauth.AzureADAuthModeServicePrincipal:
		if s.runtime.apiClient != nil {
			return s.runtime.apiClient.Timeout
		}
	case cloudauth.AzureADAuthModeManagedIdentity:
		if s.runtime.imdsClient != nil {
			return s.runtime.imdsClient.Timeout
		}
	case cloudauth.AzureADAuthModeDefault:
		if s.runtime.apiClient != nil && s.runtime.apiClient.Timeout > 0 {
			return s.runtime.apiClient.Timeout
		}
		if s.runtime.imdsClient != nil {
			return s.runtime.imdsClient.Timeout
		}
	}

	return 0
}

func (s *store) credentialOptions() *cloudauth.AzureADCredentialOptions {
	opts := &cloudauth.AzureADCredentialOptions{}

	switch s.Config.NormalizedMode() {
	case cloudauth.AzureADAuthModeServicePrincipal:
		if s.runtime.apiClient != nil && s.runtime.apiClient.Transport != nil {
			opts.ClientOptions.Transport = transportAdapter{s.runtime.apiClient.Transport}
		}
	case cloudauth.AzureADAuthModeManagedIdentity:
		if s.runtime.imdsClient != nil && s.runtime.imdsClient.Transport != nil {
			opts.ClientOptions.Transport = transportAdapter{s.runtime.imdsClient.Transport}
		}
	case cloudauth.AzureADAuthModeDefault:
		opts.ClientOptions.Transport = routingTransportAdapter{
			defaultRoundTripper: roundTripperForClient(s.runtime.apiClient),
			noProxyRoundTripper: roundTripperForClient(s.runtime.imdsClient),
		}
	}

	return opts
}

type transportAdapter struct {
	roundTripper httpRoundTripper
}

type httpRoundTripper interface {
	RoundTrip(*http.Request) (*http.Response, error)
}

func (t transportAdapter) Do(req *http.Request) (*http.Response, error) {
	return t.roundTripper.RoundTrip(req)
}

type routingTransportAdapter struct {
	defaultRoundTripper httpRoundTripper
	noProxyRoundTripper httpRoundTripper
}

func (t routingTransportAdapter) Do(req *http.Request) (*http.Response, error) {
	switch {
	case shouldUseNoProxyTransport(req) && t.noProxyRoundTripper != nil:
		return t.noProxyRoundTripper.RoundTrip(req)
	case t.defaultRoundTripper != nil:
		return t.defaultRoundTripper.RoundTrip(req)
	case t.noProxyRoundTripper != nil:
		return t.noProxyRoundTripper.RoundTrip(req)
	default:
		return http.DefaultTransport.RoundTrip(req)
	}
}

func roundTripperForClient(client *http.Client) httpRoundTripper {
	if client != nil && client.Transport != nil {
		return client.Transport
	}
	if rt, ok := http.DefaultTransport.(httpRoundTripper); ok {
		return rt
	}
	return nil
}

// Managed identity endpoints are local or link-local and must bypass proxies.
func shouldUseNoProxyTransport(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}

	host := req.URL.Hostname()
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

type credentialWithTimeout struct {
	cred    azcore.TokenCredential
	timeout time.Duration
}

func (c credentialWithTimeout) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if c.timeout <= 0 {
		return c.cred.GetToken(ctx, opts)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	return c.cred.GetToken(ctx, opts)
}
