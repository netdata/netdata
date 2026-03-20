// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"fmt"
	"net/http"

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
