// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ dyncfg.Testable = (*store)(nil)

func TestStoreTestUsesAuthenticatedSelfLookup(t *testing.T) {
	type requestRecord struct {
		method      string
		path        string
		tokenHeader string
		namespace   string
		body        string
		contentLen  int64
		transferEnc []string
	}
	requests := make(chan requestRecord, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		requests <- requestRecord{
			method:      req.Method,
			path:        req.URL.Path,
			tokenHeader: req.Header.Get("X-Vault-Token"),
			namespace:   req.Header.Get("X-Vault-Namespace"),
			body:        string(body),
			contentLen:  req.ContentLength,
			transferEnc: req.TransferEncoding,
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"accessor":"synthetic-accessor"}}`)
	}))
	defer srv.Close()

	s := newOperationalStore(t, Config{
		Mode:          "token",
		ModeToken:     &ModeTokenConfig{Token: "test-token"},
		Addr:          srv.URL + "/",
		Namespace:     "team-a",
		TLSSkipVerify: false,
	})

	require.NoError(t, s.Test(t.Context()))

	select {
	case got := <-requests:
		assert.Equal(t, http.MethodGet, got.method)
		assert.Equal(t, "/v1/auth/token/lookup-self", got.path)
		assert.Equal(t, "test-token", got.tokenHeader)
		assert.Equal(t, "team-a", got.namespace)
		assert.Empty(t, got.body)
		assert.Zero(t, got.contentLen)
		assert.Empty(t, got.transferEnc)
	default:
		require.FailNow(t, "test failed", "Vault authentication request was not observed")
	}
}

func TestStoreTestAuthenticationResponses(t *testing.T) {
	tests := map[string]struct {
		status          int
		body            string
		wantErr         bool
		wantUnsupported bool
		wantPublic      string
		wantCause       error
		privateValues   []string
	}{
		"self lookup succeeds": {
			status: http.StatusOK,
			body:   `{"data":{"accessor":"synthetic-accessor"}}`,
		},
		"valid token without self lookup permission is unsupported": {
			status:          http.StatusForbidden,
			body:            `{"errors":["permission denied"]}`,
			wantUnsupported: true,
		},
		"wrapped permission denial is unsupported": {
			status:          http.StatusForbidden,
			body:            "{\"errors\":[\"1 error occurred:\\n\\t* permission denied\\n\\n\"]}",
			wantUnsupported: true,
		},
		"invalid token fails": {
			status: http.StatusForbidden,
			body: "{\"errors\":[\"2 errors occurred:\\n\\t* permission denied\\n" +
				"\\t* invalid token\\n\\n\"]}",
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			wantCause:     errAuthenticationTokenInvalid,
			privateValues: []string{"invalid token", "permission denied"},
		},
		"malformed denial fails": {
			status:        http.StatusForbidden,
			body:          `{"errors":`,
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{`{"errors":`},
		},
		"ambiguous denial fails": {
			status:        http.StatusForbidden,
			body:          `{"errors":["backend unavailable"]}`,
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{"backend unavailable"},
		},
		"compound permission denial fails": {
			status:        http.StatusForbidden,
			body:          `{"errors":["permission denied","backend unavailable"]}`,
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{"permission denied", "backend unavailable"},
		},
		"negated permission denial fails": {
			status:        http.StatusForbidden,
			body:          `{"errors":["not permission denied"]}`,
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{"not permission denied"},
		},
		"qualified permission denial fails": {
			status:        http.StatusForbidden,
			body:          `{"errors":["permission denied: token expired"]}`,
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{"permission denied: token expired"},
		},
		"wrapped denial count mismatch fails": {
			status:        http.StatusForbidden,
			body:          "{\"errors\":[\"2 errors occurred:\\n\\t* permission denied\\n\\n\"]}",
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{"permission denied"},
		},
		"wrapped denial absurd count fails without amplified allocation": {
			status:        http.StatusForbidden,
			body:          "{\"errors\":[\"1000000000 errors occurred:\\n\\t* permission denied\\n\\n\"]}",
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{"permission denied"},
		},
		"wrapped denial grammar mismatch fails": {
			status:        http.StatusForbidden,
			body:          "{\"errors\":[\"1 errors occurred:\\n\\t* permission denied\\n\\n\"]}",
			wantErr:       true,
			wantPublic:    publicErrAuthentication,
			privateValues: []string{"permission denied"},
		},
		"unauthorized fails": {
			status:     http.StatusUnauthorized,
			body:       `{"errors":["unauthorized"]}`,
			wantErr:    true,
			wantPublic: publicErrAuthentication,
		},
		"server failure fails": {
			status:     http.StatusInternalServerError,
			body:       `{"errors":["private backend failure"]}`,
			wantErr:    true,
			wantPublic: publicErrAuthentication,
		},
		"redirect is not followed": {
			status:     http.StatusFound,
			wantErr:    true,
			wantPublic: publicErrAuthentication,
		},
		"oversized denial fails": {
			status: http.StatusForbidden,
			body: `{"errors":["permission denied"],"padding":"` +
				strings.Repeat("x", responseBodyLimit+1) + `"}`,
			wantErr:    true,
			wantPublic: publicErrAuthentication,
			wantCause:  errAuthenticationResponseTooLarge,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				if tc.status == http.StatusFound {
					w.Header().Set("Location", "/redirected")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			s := newOperationalStore(t, Config{
				Mode:      "token",
				ModeToken: &ModeTokenConfig{Token: "test-token"},
				Addr:      srv.URL,
			})

			err := s.Test(t.Context())

			assert.EqualValues(t, 1, requests.Load())
			if tc.wantUnsupported {
				require.ErrorIs(t, err, dyncfg.ErrTestUnsupported)
				_, public := dyncfg.PublicMessage(err)
				assert.False(t, public)
				return
			}
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			requireVaultPublicError(t, err, tc.wantPublic)
			if tc.wantCause != nil {
				require.ErrorIs(t, err, tc.wantCause)
			}
			for _, value := range tc.privateValues {
				assert.NotContains(t, err.Error(), value)
			}
		})
	}
}

func TestStoreTestDoesNotReplayProcessedRequest(t *testing.T) {
	var lookups atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/warm" {
			_, _ = io.WriteString(w, "warm")
			return
		}
		if req.URL.Path != "/v1/auth/token/lookup-self" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if lookups.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	s := newOperationalStore(t, Config{
		Mode:      "token",
		ModeToken: &ModeTokenConfig{Token: "test-token"},
		Addr:      srv.URL,
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/warm", nil)
	require.NoError(t, err)
	resp, err := s.runtime.httpClient.Do(req)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	err = s.Test(t.Context())

	requireVaultPublicError(t, err, publicErrEndpoint)
	assert.EqualValues(t, 1, lookups.Load())
}

func TestStoreTestTokenSources(t *testing.T) {
	tests := map[string]struct {
		config       func(t *testing.T, addr string) Config
		wantHeader   string
		wantErr      error
		wantPublic   string
		wantRequests int64
	}{
		"inline token": {
			config: func(_ *testing.T, addr string) Config {
				return Config{
					Mode:      "token",
					ModeToken: &ModeTokenConfig{Token: "test-inline-token"},
					Addr:      addr,
				}
			},
			wantHeader:   "test-inline-token",
			wantRequests: 1,
		},
		"token file": {
			config: func(t *testing.T, addr string) Config {
				path := filepath.Join(t.TempDir(), "vault.token")
				require.NoError(t, os.WriteFile(path, []byte("  file-token\n"), 0o600))
				return Config{
					Mode:          "token_file",
					ModeTokenFile: &ModeTokenFileConfig{Path: path},
					Addr:          addr,
				}
			},
			wantHeader:   "file-token",
			wantRequests: 1,
		},
		"missing token file": {
			config: func(t *testing.T, addr string) Config {
				return Config{
					Mode:          "token_file",
					ModeTokenFile: &ModeTokenFileConfig{Path: filepath.Join(t.TempDir(), "missing")},
					Addr:          addr,
				}
			},
			wantErr:    safefile.ErrFile,
			wantPublic: publicErrToken,
		},
		"empty token file": {
			config: func(t *testing.T, addr string) Config {
				path := filepath.Join(t.TempDir(), "vault.token")
				require.NoError(t, os.WriteFile(path, nil, 0o600))
				return Config{
					Mode:          "token_file",
					ModeTokenFile: &ModeTokenFileConfig{Path: path},
					Addr:          addr,
				}
			},
			wantPublic: publicErrToken,
		},
		"oversized token file": {
			config: func(t *testing.T, addr string) Config {
				path := filepath.Join(t.TempDir(), "vault.token")
				require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("x"), int(safefile.MaxSize)+1), 0o600))
				return Config{
					Mode:          "token_file",
					ModeTokenFile: &ModeTokenFileConfig{Path: path},
					Addr:          addr,
				}
			},
			wantErr:    safefile.ErrTooLarge,
			wantPublic: publicErrToken,
		},
		"token path is a directory": {
			config: func(t *testing.T, addr string) Config {
				return Config{
					Mode:          "token_file",
					ModeTokenFile: &ModeTokenFileConfig{Path: t.TempDir()},
					Addr:          addr,
				}
			},
			wantErr:    safefile.ErrNotRegular,
			wantPublic: publicErrToken,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int64
			var tokenHeader string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests.Add(1)
				tokenHeader = req.Header.Get("X-Vault-Token")
				_, _ = io.WriteString(w, `{"data":{}}`)
			}))
			defer srv.Close()

			s := newOperationalStore(t, tc.config(t, srv.URL))
			err := s.Test(t.Context())

			assert.Equal(t, tc.wantHeader, tokenHeader)
			assert.Equal(t, tc.wantRequests, requests.Load())
			if tc.wantPublic == "" {
				require.NoError(t, err)
				return
			}
			requireVaultPublicError(t, err, tc.wantPublic)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestStoreTestUsesConfiguredClientAndContext(t *testing.T) {
	tests := map[string]struct {
		configure func(t *testing.T, s *store)
		ctx       func(t *testing.T) context.Context
		timeout   confopt.Duration
		wantErr   error
	}{
		"secure client": {
			configure: func(t *testing.T, s *store) {
				s.runtime.httpClient.Transport = successfulVaultTransport(t)
				s.runtime.httpClientInsecure.Transport = failingVaultTransport(t)
			},
			ctx: func(t *testing.T) context.Context { return t.Context() },
		},
		"insecure client": {
			configure: func(t *testing.T, s *store) {
				s.Config.TLSSkipVerify = true
				s.published.tlsSkipVerify = true
				s.runtime.httpClient.Transport = failingVaultTransport(t)
				s.runtime.httpClientInsecure.Transport = successfulVaultTransport(t)
			},
			ctx: func(t *testing.T) context.Context { return t.Context() },
		},
		"caller cancellation": {
			configure: func(t *testing.T, s *store) {
				s.runtime.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
					<-req.Context().Done()
					return nil, req.Context().Err()
				})
			},
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
		"configured timeout": {
			configure: func(t *testing.T, s *store) {
				s.runtime.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
					<-req.Context().Done()
					return nil, req.Context().Err()
				})
			},
			ctx:     func(t *testing.T) context.Context { return t.Context() },
			timeout: confopt.Duration(20 * time.Millisecond),
			wantErr: context.DeadlineExceeded,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			timeout := tc.timeout
			if timeout == 0 {
				timeout = confopt.Duration(time.Second)
			}
			s := newOperationalStore(t, Config{
				Mode:      "token",
				ModeToken: &ModeTokenConfig{Token: "test-token"},
				Addr:      "https://vault.example",
				Timeout:   timeout,
			})
			tc.configure(t, s)

			err := s.Test(tc.ctx(t))

			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			requireVaultPublicError(t, err, publicErrEndpoint)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestStoreTestBodyReadHonorsContext(t *testing.T) {
	tests := map[string]struct {
		cancelAfterHeaders bool
		timeout            confopt.Duration
		wantErr            error
	}{
		"caller cancellation": {
			cancelAfterHeaders: true,
			timeout:            confopt.Duration(time.Second),
			wantErr:            context.Canceled,
		},
		"configured timeout": {
			timeout: confopt.Duration(100 * time.Millisecond),
			wantErr: context.DeadlineExceeded,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			headers := make(chan struct{}, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"errors":[`)
				w.(http.Flusher).Flush()
				headers <- struct{}{}
				<-req.Context().Done()
			}))
			defer srv.Close()

			s := newOperationalStore(t, Config{
				Mode:      "token",
				ModeToken: &ModeTokenConfig{Token: "test-token"},
				Addr:      srv.URL,
				Timeout:   tc.timeout,
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- s.Test(ctx)
			}()

			select {
			case <-headers:
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "Vault response headers were not observed")
			}
			if tc.cancelAfterHeaders {
				cancel()
			}

			select {
			case err := <-result:
				require.Error(t, err)
				_, public := dyncfg.PublicMessage(err)
				require.True(t, public)
				require.ErrorIs(t, err, tc.wantErr)
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "Vault body read did not stop")
			}
		})
	}
}

func TestStoreTestUsesConfiguredTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	tests := map[string]struct {
		skipVerify bool
		wantErr    bool
	}{
		"certificate verification is enabled": {
			wantErr: true,
		},
		"certificate verification can be skipped": {
			skipVerify: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := newOperationalStore(t, Config{
				Mode:          "token",
				ModeToken:     &ModeTokenConfig{Token: "test-token"},
				Addr:          srv.URL,
				TLSSkipVerify: tc.skipVerify,
			})

			err := s.Test(t.Context())

			if tc.wantErr {
				requireVaultPublicError(t, err, publicErrEndpoint)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestStoreTestClosesResponseAndOwnedIdleConnections(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader(`{"data":{}}`)}
	secure := &closeTrackingTransport{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     make(http.Header),
	}}
	insecure := &closeTrackingTransport{}
	s := newOperationalStore(t, Config{
		Mode:      "token",
		ModeToken: &ModeTokenConfig{Token: "test-token"},
		Addr:      "https://vault.example",
	})
	s.runtime.httpClient.Transport = secure
	s.runtime.httpClientInsecure.Transport = insecure

	require.NoError(t, s.Test(t.Context()))

	assert.True(t, body.closed.Load())
	assert.Zero(t, secure.closeIdleCalls.Load())
	assert.EqualValues(t, 1, insecure.closeIdleCalls.Load())
}

func TestSecretStoreTestUsesVaultOperationalCapability(t *testing.T) {
	tests := map[string]struct {
		status     int
		body       string
		wantTested bool
	}{
		"successful self lookup is operational": {
			status:     http.StatusOK,
			body:       `{"data":{}}`,
			wantTested: true,
		},
		"permission-only denial is validation-only": {
			status: http.StatusForbidden,
			body:   `{"errors":["permission denied"]}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			resolver, err := secretresolver.NewAtomicResolver(nil)
			require.NoError(t, err)
			authority, err := secretstore.NewSecretStore(resolver)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, authority.Close(context.Background()))
			})
			catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{New()})
			require.NoError(t, err)
			config := secretstore.Config{
				"name":            "main",
				"kind":            string(secretstore.KindVault),
				"mode":            "token",
				"mode_token":      map[string]any{"token": "test-token"},
				"addr":            srv.URL,
				"__source__":      confgroup.TypeDyncfg,
				"__source_type__": confgroup.TypeDyncfg,
			}

			tested, err := authority.Test(t.Context(), catalog, config)

			require.NoError(t, err)
			assert.Equal(t, tc.wantTested, tested)
			assert.Equal(t, secretstore.SecretStoreCensus{}, authority.Census())
		})
	}
}

func newOperationalStore(t *testing.T, config Config) *store {
	t.Helper()
	s := &store{Config: config}
	require.NoError(t, s.Init(t.Context()))
	return s
}

func requireVaultPublicError(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t, err)
	message, ok := dyncfg.PublicMessage(err)
	require.True(t, ok)
	assert.Equal(t, want, message)
	assert.Equal(t, want, err.Error())
}

func successfulVaultTransport(t *testing.T) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{}}`)),
			Header:     make(http.Header),
		}, nil
	})
}

func failingVaultTransport(t *testing.T) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(*http.Request) (*http.Response, error) {
		require.FailNow(t, "test failed", "unexpected Vault HTTP client was selected")
		return nil, errors.New("unexpected Vault HTTP client")
	})
}

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

type closeTrackingTransport struct {
	response       *http.Response
	closeIdleCalls atomic.Int64
}

func (t *closeTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	if t.response == nil {
		return nil, fmt.Errorf("unexpected request")
	}
	return t.response, nil
}

func (t *closeTrackingTransport) CloseIdleConnections() {
	t.closeIdleCalls.Add(1)
}
