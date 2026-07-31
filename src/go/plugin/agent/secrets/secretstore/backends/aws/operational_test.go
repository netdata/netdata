// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ dyncfg.Testable = (*store)(nil)

func TestECSCredentialsEndpoint(t *testing.T) {
	tests := map[string]struct {
		relativeURI string
		want        string
		wantErr     bool
	}{
		"path": {
			relativeURI: "/v2/credentials/test-id",
			want:        ecsMetadataEndpoint + "/v2/credentials/test-id",
		},
		"path and query": {
			relativeURI: "/v2/credentials/test-id?version=1",
			want:        ecsMetadataEndpoint + "/v2/credentials/test-id?version=1",
		},
		"missing leading slash": {
			relativeURI: "v2/credentials/test-id",
			wantErr:     true,
		},
		"absolute URI": {
			relativeURI: "http://metadata.example/credentials",
			wantErr:     true,
		},
		"authority-shaped value": {
			relativeURI: "@metadata.example/credentials",
			wantErr:     true,
		},
		"fragment": {
			relativeURI: "/v2/credentials/test-id#fragment",
			wantErr:     true,
		},
		"invalid escape": {
			relativeURI: "/v2/credentials/%zz",
			wantErr:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ecsCredentialsEndpoint(tc.relativeURI)

			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestECSCredentialsResponses(t *testing.T) {
	readErr := errors.New("response read failed")
	tests := map[string]struct {
		status          int
		body            string
		reader          io.Reader
		wantErrContains string
		wantTooLarge    bool
	}{
		"tokenless credentials succeed": {
			status: http.StatusOK,
			body:   `{"AccessKeyId":"test-access","SecretAccessKey":"test-secret"}`,
		},
		"session token is accepted": {
			status: http.StatusOK,
			body:   `{"AccessKeyId":"test-access","SecretAccessKey":"test-secret","Token":"test-token"}`,
		},
		"missing required key fails": {
			status:          http.StatusOK,
			body:            `{"AccessKeyId":"test-access"}`,
			wantErrContains: "missing required fields",
		},
		"malformed JSON fails": {
			status:          http.StatusOK,
			body:            `{"AccessKeyId":`,
			wantErrContains: "parsing ECS credentials response",
		},
		"oversized success response fails": {
			status:          http.StatusOK,
			body:            strings.Repeat("x", responseBodyLimit+1),
			wantErrContains: "reading ECS credentials response",
			wantTooLarge:    true,
		},
		"status wins over oversized diagnostic body": {
			status:          http.StatusForbidden,
			body:            strings.Repeat("x", responseBodyLimit+1),
			wantErrContains: "ECS credentials returned HTTP 403",
		},
		"read failure fails": {
			status:          http.StatusOK,
			reader:          errorReader{err: readErr},
			wantErrContains: "reading ECS credentials response",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reader := tc.reader
			if reader == nil {
				reader = strings.NewReader(tc.body)
			}
			body := &trackingReadCloser{Reader: reader}
			s := publishedWithMetadataTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "169.254.170.2", req.URL.Host)
				assert.Equal(t, "/v2/credentials/test-id", req.URL.Path)
				return &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}, nil
			}))

			creds, err := s.ecsCredentials(t.Context(), "/v2/credentials/test-id")

			assert.True(t, body.closed.Load())
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				require.NotNil(t, creds)
				assert.Equal(t, "test-access", creds.accessKeyID)
				assert.Equal(t, "test-secret", creds.secretAccessKey)
				return
			}
			require.ErrorContains(t, err, tc.wantErrContains)
			if tc.wantTooLarge {
				require.ErrorIs(t, err, httpx.ErrResponseTooLarge)
			}
		})
	}
}

func TestIMDSCredentialsRequestPath(t *testing.T) {
	tests := map[string]struct {
		token           string
		role            string
		credentials     string
		wantRole        string
		wantRequests    int64
		wantErrContains string
	}{
		"legal role is used unchanged": {
			token:        "imds-token",
			role:         "team,prod\n",
			credentials:  `{"AccessKeyId":"test-access","SecretAccessKey":"test-secret"}`,
			wantRole:     "team,prod",
			wantRequests: 3,
		},
		"maximum length role succeeds": {
			token:        "imds-token",
			role:         strings.Repeat("a", 64),
			credentials:  `{"AccessKeyId":"test-access","SecretAccessKey":"test-secret","Token":"test-token"}`,
			wantRole:     strings.Repeat("a", 64),
			wantRequests: 3,
		},
		"empty token fails": {
			token:           " \n",
			wantRequests:    1,
			wantErrContains: "empty token",
		},
		"empty role fails": {
			token:           "imds-token",
			role:            " \n",
			wantRequests:    2,
			wantErrContains: "invalid role name",
		},
		"path separator in role fails": {
			token:           "imds-token",
			role:            "team/prod",
			wantRequests:    2,
			wantErrContains: "invalid role name",
		},
		"multi-line role fails": {
			token:           "imds-token",
			role:            "team\nprod",
			wantRequests:    2,
			wantErrContains: "invalid role name",
		},
		"overlong role fails": {
			token:           "imds-token",
			role:            strings.Repeat("a", 65),
			wantRequests:    2,
			wantErrContains: "invalid role name",
		},
		"missing credential key fails": {
			token:           "imds-token",
			role:            "test-role",
			credentials:     `{"AccessKeyId":"test-access"}`,
			wantRole:        "test-role",
			wantRequests:    3,
			wantErrContains: "missing required fields",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int64
			s := publishedWithMetadataTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				request := requests.Add(1)
				switch request {
				case 1:
					assert.Equal(t, http.MethodPut, req.Method)
					assert.Equal(t, "/latest/api/token", req.URL.Path)
					assert.Equal(t, "21600", req.Header.Get("X-aws-ec2-metadata-token-ttl-seconds"))
					return awsHTTPResponse(http.StatusOK, tc.token), nil
				case 2:
					assert.Equal(t, http.MethodGet, req.Method)
					assert.Equal(t, "/latest/meta-data/iam/security-credentials/", req.URL.Path)
					assert.Equal(t, "imds-token", req.Header.Get("X-aws-ec2-metadata-token"))
					return awsHTTPResponse(http.StatusOK, tc.role), nil
				case 3:
					assert.Equal(t, http.MethodGet, req.Method)
					assert.Equal(t, "/latest/meta-data/iam/security-credentials/"+tc.wantRole, req.URL.Path)
					assert.Equal(t, "imds-token", req.Header.Get("X-aws-ec2-metadata-token"))
					return awsHTTPResponse(http.StatusOK, tc.credentials), nil
				default:
					return nil, fmt.Errorf("unexpected IMDS request %d", request)
				}
			}))

			creds, err := s.imdsCredentials(t.Context())

			assert.Equal(t, tc.wantRequests, requests.Load())
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, creds)
			assert.Equal(t, "test-access", creds.accessKeyID)
			assert.Equal(t, "test-secret", creds.secretAccessKey)
		})
	}
}

func TestStoreTestModesAndCleanup(t *testing.T) {
	tests := map[string]struct {
		mode         string
		setupEnv     func(t *testing.T)
		wantRequests int64
		wantErr      bool
	}{
		"environment credentials are operational": {
			mode: "env",
			setupEnv: func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "test-access")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
			},
		},
		"missing environment credentials fail safely": {
			mode:    "env",
			wantErr: true,
		},
		"ECS credential source is operational": {
			mode: "ecs",
			setupEnv: func(t *testing.T) {
				t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/test-id")
			},
			wantRequests: 1,
		},
		"IMDS credential source is operational": {
			mode:         "imds",
			wantRequests: 3,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AWS_ACCESS_KEY_ID", "")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "")
			t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
			if tc.setupEnv != nil {
				tc.setupEnv(t)
			}

			s := newAWSOperationalStore(t, Config{AuthMode: tc.mode, Region: "us-east-1"})
			metadataTransport := &trackingTransport{roundTrip: func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/v2/credentials/test-id":
					return awsHTTPResponse(http.StatusOK, `{"AccessKeyId":"test-access","SecretAccessKey":"test-secret"}`), nil
				case "/latest/api/token":
					return awsHTTPResponse(http.StatusOK, "imds-token"), nil
				case "/latest/meta-data/iam/security-credentials/":
					return awsHTTPResponse(http.StatusOK, "test-role"), nil
				case "/latest/meta-data/iam/security-credentials/test-role":
					return awsHTTPResponse(http.StatusOK, `{"AccessKeyId":"test-access","SecretAccessKey":"test-secret"}`), nil
				default:
					return nil, fmt.Errorf("unexpected metadata path %q", req.URL.Path)
				}
			}}
			apiTransport := &trackingTransport{roundTrip: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("unexpected AWS API request")
			}}
			s.runtime.imdsClient.Transport = metadataTransport
			s.runtime.apiClient.Transport = apiTransport

			err := s.Test(t.Context())

			assert.Equal(t, tc.wantRequests, metadataTransport.requests.Load())
			assert.EqualValues(t, 1, metadataTransport.closeIdleCalls.Load())
			assert.Zero(t, apiTransport.requests.Load())
			assert.Zero(t, apiTransport.closeIdleCalls.Load())
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			requireAWSPublicError(t, err)
			assert.NotContains(t, err.Error(), "AWS_ACCESS_KEY_ID")
		})
	}
}

func TestStoreTestUsesProductionMetadataTransport(t *testing.T) {
	tests := map[string]struct {
		timeout      confopt.Duration
		handler      func(t *testing.T, w http.ResponseWriter, req *http.Request)
		ctx          func(t *testing.T) context.Context
		wantRequests int64
		wantErr      bool
		wantErrIs    error
	}{
		"success keeps the fixed metadata host": {
			handler: func(t *testing.T, w http.ResponseWriter, req *http.Request) {
				assert.Equal(t, "169.254.170.2", req.Host)
				assert.Equal(t, "/v2/credentials/test-id", req.URL.Path)
				_, _ = io.WriteString(w, `{"AccessKeyId":"test-access","SecretAccessKey":"test-secret"}`)
			},
			wantRequests: 1,
		},
		"redirect is rejected at the first response": {
			handler: func(_ *testing.T, w http.ResponseWriter, req *http.Request) {
				http.Redirect(w, req, "/redirected", http.StatusTemporaryRedirect)
			},
			wantRequests: 1,
			wantErr:      true,
		},
		"caller cancellation stops before metadata I/O": {
			handler: func(t *testing.T, _ http.ResponseWriter, _ *http.Request) {
				require.FailNow(t, "test failed", "canceled Test reached the metadata server")
			},
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			wantErr:   true,
			wantErrIs: context.Canceled,
		},
		"configured timeout interrupts a stalled body": {
			timeout: confopt.Duration(30 * time.Millisecond),
			handler: func(_ *testing.T, w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"AccessKeyId":`)
				w.(http.Flusher).Flush()
				<-req.Context().Done()
			},
			wantRequests: 1,
			wantErr:      true,
			wantErrIs:    context.DeadlineExceeded,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests.Add(1)
				tc.handler(t, w, req)
			}))
			defer srv.Close()

			t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/test-id")
			s := newAWSOperationalStore(t, Config{
				AuthMode: "ecs",
				Region:   "us-east-1",
				Timeout:  tc.timeout,
			})
			transport, ok := s.runtime.imdsClient.Transport.(*http.Transport)
			require.True(t, ok)
			target := srv.Listener.Addr().String()
			transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, target)
			}

			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(t)
			}
			err := s.Test(ctx)

			assert.Equal(t, tc.wantRequests, requests.Load())
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			requireAWSPublicError(t, err)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, err, tc.wantErrIs)
			}
		})
	}
}

func TestStoreTestRejectsInvalidState(t *testing.T) {
	tests := map[string]func() error{
		"nil Store": func() error {
			var s *store
			return s.Test(t.Context())
		},
		"nil context": func() error {
			s := newAWSOperationalStore(t, Config{AuthMode: "env", Region: "us-east-1"})
			return s.Test(nil)
		},
		"uninitialized Store": func() error {
			return (&store{}).Test(t.Context())
		},
		"published runtime without metadata client": func() error {
			s := newAWSOperationalStore(t, Config{AuthMode: "env", Region: "us-east-1"})
			s.published.runtime = &runtime{}
			return s.Test(t.Context())
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.EqualError(t, test(), "invalid AWS operational test")
		})
	}
}

func TestSecretStoreTestUsesAWSOperationalCapability(t *testing.T) {
	tests := map[string]struct {
		accessKey  string
		secretKey  string
		wantTested bool
		wantErr    bool
	}{
		"valid environment source is operational": {
			accessKey:  "test-access",
			secretKey:  "test-secret",
			wantTested: true,
		},
		"missing environment source is an operational failure": {
			wantTested: true,
			wantErr:    true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AWS_ACCESS_KEY_ID", tc.accessKey)
			t.Setenv("AWS_SECRET_ACCESS_KEY", tc.secretKey)

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
				"kind":            string(secretstore.KindAWSSM),
				"auth_mode":       "env",
				"region":          "us-east-1",
				"__source__":      confgroup.TypeDyncfg,
				"__source_type__": confgroup.TypeDyncfg,
			}

			tested, err := authority.Test(t.Context(), catalog, config)

			assert.Equal(t, tc.wantTested, tested)
			assert.Equal(t, secretstore.SecretStoreCensus{}, authority.Census())
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			requireAWSPublicError(t, err)
		})
	}
}

func newAWSOperationalStore(t *testing.T, config Config) *store {
	t.Helper()
	s := &store{Config: config}
	require.NoError(t, s.Init(t.Context()))
	return s
}

func publishedWithMetadataTransport(t *testing.T, transport http.RoundTripper) *publishedStore {
	t.Helper()
	return &publishedStore{
		runtime: &runtime{
			imdsClient: &http.Client{Transport: transport},
		},
		mode:        "ecs",
		regionValue: "us-east-1",
	}
}

func awsHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func requireAWSPublicError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	message, ok := dyncfg.PublicMessage(err)
	require.True(t, ok)
	assert.Equal(t, publicErrCredentials, message)
	assert.Equal(t, publicErrCredentials, err.Error())
}

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

type trackingTransport struct {
	roundTrip      roundTripFunc
	requests       atomic.Int64
	closeIdleCalls atomic.Int64
}

func (t *trackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.requests.Add(1)
	return t.roundTrip(req)
}

func (t *trackingTransport) CloseIdleConnections() {
	t.closeIdleCalls.Add(1)
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
