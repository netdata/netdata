// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

const azureKeyVaultScope = "https://vault.azure.net/.default"

func (s *store) init(_ context.Context) error {
	if err := s.Config.ValidateWithPath(""); err != nil {
		return err
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
		provider:      s.provider,
		tokenProvider: tokenProvider,
	}
	return nil
}

func (s *store) authTimeout() time.Duration {
	switch s.Config.NormalizedMode() {
	case cloudauth.AzureADAuthModeServicePrincipal:
		if s.provider.apiClient != nil {
			return s.provider.apiClient.Timeout
		}
	case cloudauth.AzureADAuthModeManagedIdentity:
		if s.provider.imdsClient != nil {
			return s.provider.imdsClient.Timeout
		}
	case cloudauth.AzureADAuthModeDefault:
		if s.provider.apiClient != nil && s.provider.apiClient.Timeout > 0 {
			return s.provider.apiClient.Timeout
		}
		if s.provider.imdsClient != nil {
			return s.provider.imdsClient.Timeout
		}
	}

	return 0
}

func (s *store) credentialOptions() *cloudauth.AzureADCredentialOptions {
	opts := &cloudauth.AzureADCredentialOptions{}

	switch s.Config.NormalizedMode() {
	case cloudauth.AzureADAuthModeServicePrincipal:
		if s.provider.apiClient != nil && s.provider.apiClient.Transport != nil {
			opts.ClientOptions.Transport = transportAdapter{s.provider.apiClient.Transport}
		}
	case cloudauth.AzureADAuthModeManagedIdentity:
		if s.provider.imdsClient != nil && s.provider.imdsClient.Transport != nil {
			opts.ClientOptions.Transport = transportAdapter{s.provider.imdsClient.Transport}
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
