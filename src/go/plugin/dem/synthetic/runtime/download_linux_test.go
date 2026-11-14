// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

type downloadTransport func(*http.Request) (*http.Response, error)

func (f downloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadReportsCauseWithoutRequestCredentials(t *testing.T) {
	for name, tc := range map[string]struct {
		cause error
		want  string
	}{
		"DNS":        {&net.DNSError{Err: "no such host", Name: "runtime.invalid", IsNotFound: true}, "DNS name not found"},
		"TLS":        {&tls.CertificateVerificationError{Err: errors.New("untrusted certificate")}, "TLS certificate verification failed"},
		"nested URL": {&url.Error{Op: "Get", URL: "https://user:private-password@runtime.invalid/?token=private-query", Err: errors.New("opaque private-query")}, "HTTP request failed"},
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := testStore(t)
			m, _ := bundleFixture(t, "download-diagnostic")
			m.Archive.URL = "https://runtime.invalid/archive?token=private-query"
			previous := http.DefaultTransport
			http.DefaultTransport = downloadTransport(func(*http.Request) (*http.Response, error) { return nil, tc.cause })
			defer func() { http.DefaultTransport = previous }()
			err := s.download(context.Background(), m)
			require.ErrorIs(t, err, tc.cause)
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "private-query")
			require.NotContains(t, err.Error(), "private-password")
		})
	}
}

func TestDownloadDoesNotExposeMalformedRedirectCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://user:private-user@runtime.invalid/%ZZ?token=private-query")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = previous }()
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "redirect-diagnostic")
	m.Archive.URL = server.URL
	err := s.download(context.Background(), m)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-user")
	require.NotContains(t, err.Error(), "private-query")
}

func TestDownloadReportsRedirectPolicyFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://user:private-password@runtime.invalid/?token=private-query")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = previous }()
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "redirect-policy")
	m.Archive.URL = server.URL
	err := s.download(context.Background(), m)
	require.ErrorIs(t, err, errDownloadRedirect)
	require.ErrorContains(t, err, "unsafe runtime download redirect")
	require.NotContains(t, err.Error(), "private-password")
	require.NotContains(t, err.Error(), "private-query")
}
