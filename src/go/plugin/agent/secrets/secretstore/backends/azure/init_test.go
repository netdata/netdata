// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreInit(t *testing.T) {
	tests := map[string]struct {
		cfg             Config
		wantTimeout     time.Duration
		wantErrContains string
	}{
		"service principal": {
			cfg: Config{
				AzureADAuthConfig: cloudauth.AzureADAuthConfig{
					Mode: cloudauth.AzureADAuthModeServicePrincipal,
					ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
						TenantID:     "tenant-id",
						ClientID:     "client-id",
						ClientSecret: "client-secret",
					},
				},
				Timeout: confopt.Duration(7 * time.Second),
			},
			wantTimeout: 7 * time.Second,
		},
		"default": {
			cfg: Config{
				AzureADAuthConfig: cloudauth.AzureADAuthConfig{
					Mode: cloudauth.AzureADAuthModeDefault,
				},
			},
			wantTimeout: defaultTimeout.Duration(),
		},
		"service principal validation": {
			cfg: Config{
				AzureADAuthConfig: cloudauth.AzureADAuthConfig{
					Mode: cloudauth.AzureADAuthModeServicePrincipal,
					ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
						TenantID: "tenant-id",
						ClientID: "client-id",
					},
				},
			},
			wantErrContains: "mode_service_principal.client_secret is required",
		},
		"negative timeout": {
			cfg: Config{
				AzureADAuthConfig: cloudauth.AzureADAuthConfig{
					Mode: cloudauth.AzureADAuthModeDefault,
				},
				Timeout: confopt.Duration(-time.Second),
			},
			wantErrContains: "timeout cannot be negative",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &store{
				Config: tc.cfg,
			}

			err := s.init(context.Background())
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, s.published)
			assert.NotNil(t, s.published.tokenProvider)
			assert.Equal(t, tc.wantTimeout, s.runtime.apiClient.Timeout)
			assert.Equal(t, tc.wantTimeout, s.runtime.imdsClient.Timeout)
		})
	}
}

func TestStoreAuthTimeout(t *testing.T) {
	tests := map[string]struct {
		mode        string
		apiTimeout  time.Duration
		imdsTimeout time.Duration
		want        time.Duration
	}{
		"service principal uses api client timeout": {
			mode:       cloudauth.AzureADAuthModeServicePrincipal,
			apiTimeout: 10 * time.Second,
			want:       10 * time.Second,
		},
		"managed identity uses imds timeout": {
			mode:        cloudauth.AzureADAuthModeManagedIdentity,
			imdsTimeout: 2 * time.Second,
			want:        2 * time.Second,
		},
		"default prefers api client timeout": {
			mode:        cloudauth.AzureADAuthModeDefault,
			apiTimeout:  10 * time.Second,
			imdsTimeout: 2 * time.Second,
			want:        10 * time.Second,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &store{
				Config: Config{
					AzureADAuthConfig: cloudauth.AzureADAuthConfig{
						Mode: tc.mode,
					},
				},
				runtime: &runtime{
					apiClient:  &http.Client{Timeout: tc.apiTimeout},
					imdsClient: &http.Client{Timeout: tc.imdsTimeout},
				},
			}

			assert.Equal(t, tc.want, s.authTimeout())
		})
	}
}

func TestCredentialWithTimeout(t *testing.T) {
	errMissingDeadline := errors.New("missing deadline")

	tests := map[string]struct {
		timeout      time.Duration
		buildTokenFn func(t *testing.T) (func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error), func(t *testing.T))
		wantErr      bool
		wantErrIs    error
		wantToken    string
	}{
		"zero timeout passes through": {
			timeout: 0,
			buildTokenFn: func(*testing.T) (func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error), func(t *testing.T)) {
				return func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
					return azcore.AccessToken{Token: "ok"}, nil
				}, nil
			},
			wantToken: "ok",
		},
		"timeout cancels token request": {
			timeout: 20 * time.Millisecond,
			buildTokenFn: func(*testing.T) (func(ctx context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error), func(t *testing.T)) {
				var sawDeadline bool

				return func(ctx context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
						if _, ok := ctx.Deadline(); !ok {
							return azcore.AccessToken{}, errMissingDeadline
						}

						sawDeadline = true
						<-ctx.Done()
						return azcore.AccessToken{}, ctx.Err()
					}, func(t *testing.T) {
						assert.True(t, sawDeadline)
					}
			},
			wantErr:   true,
			wantErrIs: context.DeadlineExceeded,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			getToken, assertPostRun := tc.buildTokenFn(t)

			cred := credentialWithTimeout{
				cred:    fakeTokenCredential{getToken: getToken},
				timeout: tc.timeout,
			}

			type result struct {
				token azcore.AccessToken
				err   error
			}

			results := make(chan result, 1)
			go func() {
				token, err := cred.GetToken(context.Background(), policy.TokenRequestOptions{Scopes: []string{azureKeyVaultScope}})
				results <- result{token: token, err: err}
			}()

			var res result
			select {
			case res = <-results:
			case <-time.After(2 * time.Second):
				t.Fatal("GetToken did not return before the outer test timeout")
			}

			if tc.wantErr {
				require.Error(t, res.err)
				assert.ErrorIs(t, res.err, tc.wantErrIs)
				if assertPostRun != nil {
					assertPostRun(t)
				}
				return
			}

			require.NoError(t, res.err)
			assert.Equal(t, tc.wantToken, res.token.Token)
		})
	}
}

func TestDefaultCredentialTransportRouting(t *testing.T) {
	tests := map[string]struct {
		url              string
		wantDefaultCalls int
		wantNoProxyCalls int
	}{
		"aad host uses default transport": {
			url:              "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
			wantDefaultCalls: 1,
		},
		"imds uses no proxy transport": {
			url:              "http://169.254.169.254/metadata/identity/oauth2/token",
			wantNoProxyCalls: 1,
		},
		"localhost managed identity endpoint uses no proxy transport": {
			url:              "http://localhost/msi/token",
			wantNoProxyCalls: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var defaultCalls int
			var noProxyCalls int

			s := &store{
				Config: Config{
					AzureADAuthConfig: cloudauth.AzureADAuthConfig{
						Mode: cloudauth.AzureADAuthModeDefault,
					},
				},
				runtime: &runtime{
					apiClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						defaultCalls++
						return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
					})},
					imdsClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						noProxyCalls++
						return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
					})},
				},
			}

			transport, ok := s.credentialOptions().ClientOptions.Transport.(routingTransportAdapter)
			require.True(t, ok)

			req, err := http.NewRequest(http.MethodGet, tc.url, nil)
			require.NoError(t, err)

			_, err = transport.Do(req)
			require.NoError(t, err)

			assert.Equal(t, tc.wantDefaultCalls, defaultCalls)
			assert.Equal(t, tc.wantNoProxyCalls, noProxyCalls)
		})
	}
}

func TestInitBuildsStoreScopedRuntime(t *testing.T) {
	creator := New()

	first, ok := creator.Create().(*store)
	require.True(t, ok)
	second, ok := creator.Create().(*store)
	require.True(t, ok)

	assert.Equal(t, defaultTimeout, first.Config.Timeout)
	first.AzureADAuthConfig.Mode = cloudauth.AzureADAuthModeDefault
	second.AzureADAuthConfig.Mode = cloudauth.AzureADAuthModeDefault

	require.NoError(t, first.init(context.Background()))
	require.NoError(t, second.init(context.Background()))

	require.NotNil(t, first.runtime)
	require.NotNil(t, second.runtime)
	assert.NotSame(t, first.runtime, second.runtime)
	assert.NotSame(t, first.runtime.apiClient, second.runtime.apiClient)
	assert.NotSame(t, first.runtime.imdsClient, second.runtime.imdsClient)
	assert.Equal(t, defaultTimeout.Duration(), first.runtime.apiClient.Timeout)
	assert.Equal(t, defaultTimeout.Duration(), first.runtime.imdsClient.Timeout)
}
