// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfiletest"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/require"
)

func TestBearerReaderRequired(t *testing.T) {
	_, err := NewHTTPRequest(context.Background(), RequestConfig{URL: "http://localhost", BearerTokenFile: "/var/run/secrets/test-token"}, nil)
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotErrorIs(t, err, fs.ErrNotExist)
}

func TestBearerRequestReadsCurrentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	files := credentialfiletest.New(t)
	cfg := RequestConfig{URL: "http://localhost", BearerTokenFile: path}
	for _, token := range []string{"synthetic-first", "synthetic-rotated"} {
		require.NoError(t, os.WriteFile(path, []byte(token), 0600))
		req, err := NewHTTPRequest(context.Background(), cfg, files)
		require.NoError(t, err)
		require.Equal(t, "Bearer "+token, req.Header.Get("Authorization"))
	}
	require.NoError(t, os.Remove(path))
	_, err := NewHTTPRequest(context.Background(), cfg, files)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestBearerReadUsesRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewHTTPRequestWithPath(ctx, RequestConfig{URL: "http://localhost", BearerTokenFile: "synthetic-token"}, "metrics", credentialfiletest.New(t))
	require.ErrorIs(t, err, context.Canceled)
}
