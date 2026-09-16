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

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestWithoutTokenDoesNotReadFiles(t *testing.T) {
	readFile := func(context.Context, string) ([]byte, error) {
		t.Fatal("request without a token file must not read credentials")
		return nil, nil
	}
	req, err := newHTTPRequest(
		context.Background(),
		RequestConfig{URL: "http://localhost", Username: "user", Password: "password"},
		readFile,
	)
	require.NoError(t, err)
	user, password, ok := req.BasicAuth()
	require.True(t, ok)
	require.Equal(t, "user", user)
	require.Equal(t, "password", password)
}

func TestBearerRequestReadsCurrentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	readFile := testutil.New().Read
	cfg := RequestConfig{URL: "http://localhost", BearerTokenFile: path}
	for _, token := range []string{"synthetic-first", "synthetic-rotated"} {
		require.NoError(t, os.WriteFile(path, []byte(token), 0600))
		req, err := newHTTPRequest(context.Background(), cfg, readFile)
		require.NoError(t, err)
		require.Equal(t, "Bearer "+token, req.Header.Get("Authorization"))
	}
	require.NoError(t, os.Remove(path))
	_, err := newHTTPRequest(context.Background(), cfg, readFile)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestRequestPreservesHeaderAndRedirectPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "configured-header", r.Header.Get("Authorization"))
		http.Redirect(w, r, "/target", http.StatusFound)
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("synthetic-token"), 0600))
	cfg := RequestConfig{
		URL:             server.URL,
		BearerTokenFile: path,
		Headers:         map[string]string{"Authorization": "configured-header"},
	}
	req, err := newHTTPRequestWithPath(context.Background(), cfg, "/start", testutil.New().Read)
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
	_, err := NewHTTPRequestWithPath(
		ctx,
		RequestConfig{URL: "http://localhost", BearerTokenFile: "synthetic-token"},
		"metrics",
	)
	require.ErrorIs(t, err, context.Canceled)
}
