// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestImportRejectsFIFOWithoutBlocking(t *testing.T) {
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "fifo")
	path := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, unix.Mkfifo(path, 0600))
	result := make(chan error, 1)
	go func() { result <- s.Import(context.Background(), m, path) }()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "regular")
	case <-time.After(time.Second):
		writer, err := os.OpenFile(path, os.O_RDWR|unix.O_NONBLOCK, 0)
		require.NoError(t, err)
		<-result
		require.NoError(t, writer.Close())
		t.Fatal("archive open blocked on a FIFO")
	}
}

func TestImportFollowsRegularArchiveSymlink(t *testing.T) {
	s, _ := testStore(t)
	m, archive := bundleFixture(t, "symlink")
	dir := t.TempDir()
	target, link := filepath.Join(dir, "archive"), filepath.Join(dir, "link")
	require.NoError(t, os.WriteFile(target, archive, 0600))
	require.NoError(t, os.Symlink(target, link))
	require.NoError(t, s.Import(context.Background(), m, link))
	require.NoError(t, s.Available(m))
}

func TestPublicationReportsGenerationReadFailure(t *testing.T) {
	s, _ := testStore(t)
	m, archive := bundleFixture(t, "publication")
	err := s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
		name := filepath.Join(s.path, "generations", m.Archive.SHA256)
		if err := os.Mkdir(name, 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(name, "prepared.json"), []byte("broken"), 0600); err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(archive)), nil
	})
	require.ErrorContains(t, err, "invalid character")
	require.NotContains(t, err.Error(), "appeared during preparation")
}

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
			err := s.Download(context.Background(), m)
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
	err := s.Download(context.Background(), m)
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
	err := s.Download(context.Background(), m)
	require.ErrorIs(t, err, errDownloadRedirect)
	require.ErrorContains(t, err, "unsafe runtime download redirect")
	require.NotContains(t, err.Error(), "private-password")
	require.NotContains(t, err.Error(), "private-query")
}
