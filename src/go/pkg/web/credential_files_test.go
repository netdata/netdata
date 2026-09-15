// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile"
	"github.com/netdata/netdata/go/plugins/pkg/credentialfiletest"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBearerReaderRequired(t *testing.T) {
	_, err := WrapHTTPClient(
		&http.Client{},
		nil,
	).NewRequest(context.Background(), RequestConfig{URL: "http://localhost", BearerTokenFile: "/var/run/secrets/test-token"})
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotErrorIs(t, err, fs.ErrNotExist)
}

func TestBearerRequestReadsCurrentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	files := credentialfiletest.New(t)
	client := WrapHTTPClient(&http.Client{}, files)
	cfg := RequestConfig{URL: "http://localhost", BearerTokenFile: path}
	for _, token := range []string{"synthetic-first", "synthetic-rotated"} {
		require.NoError(t, os.WriteFile(path, []byte(token), 0600))
		req, err := client.NewRequest(context.Background(), cfg)
		require.NoError(t, err)
		require.Equal(t, "Bearer "+token, req.Header.Get("Authorization"))
	}
	require.NoError(t, os.Remove(path))
	_, err := client.NewRequest(context.Background(), cfg)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

type trackedCredentialReader struct {
	credentialfile.FileReader
	closed bool
}

func (r *trackedCredentialReader) Close() error {
	r.closed = true
	return r.FileReader.Close()
}

func TestHTTPClientBorrowsReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("synthetic-token"), 0600))
	files := &trackedCredentialReader{FileReader: credentialfiletest.New(t)}
	initCtx, cancel := context.WithCancel(context.Background())
	client, err := NewHTTPClient(initCtx, ClientConfig{}, files)
	require.NoError(t, err)
	cancel()
	client.CloseIdleConnections()
	require.False(t, files.closed)
	req, err := client.NewRequest(context.Background(), RequestConfig{URL: "http://localhost", BearerTokenFile: path})
	require.NoError(t, err)
	require.NoError(t, req.Context().Err())
	require.Equal(t, "Bearer synthetic-token", req.Header.Get("Authorization"))
}

func TestWrappedHTTPClientPreservesRequestAndRedirectPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "configured-header", r.Header.Get("Authorization"))
		http.Redirect(w, r, "/target", http.StatusFound)
	}))
	defer server.Close()
	raw := server.Client()
	raw.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client := WrapHTTPClient(raw, credentialfiletest.New(t))
	require.Same(t, raw, client.Client)
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("synthetic-token"), 0600))
	cfg := RequestConfig{
		URL:             server.URL,
		BearerTokenFile: path,
		Headers:         map[string]string{"Authorization": "configured-header"},
	}
	req, err := client.NewRequestWithPath(context.Background(), cfg, "/start")
	require.NoError(t, err)
	require.Equal(t, server.URL, cfg.URL)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer CloseBody(resp)
	require.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestBearerReadUsesRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WrapHTTPClient(
		&http.Client{},
		credentialfiletest.New(t),
	).NewRequestWithPath(ctx, RequestConfig{URL: "http://localhost", BearerTokenFile: "synthetic-token"}, "metrics")
	require.ErrorIs(t, err, context.Canceled)
}
