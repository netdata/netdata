// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile"
	"github.com/netdata/netdata/go/plugins/pkg/credentialfiletest"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/stretchr/testify/require"
)

func newLocalDiscoverer(t *testing.T, cfg Config) (*Discoverer, error) {
	t.Helper()
	return newDiscoverer(cfg, func() credentialfile.FileReader { return credentialfiletest.New(t) })
}

type closeReader struct {
	credentialfile.FileReader
	closes int
	reads  int
}

func (r *closeReader) Read(ctx context.Context, path string) ([]byte, error) {
	if r.closes > 0 {
		return nil, errors.New("reader is closed")
	}
	r.reads++
	return r.FileReader.Read(ctx, path)
}

func (r *closeReader) Close() error { r.closes++; return nil }

func TestCredentialReaderLifecycle(t *testing.T) {
	for _, mode := range []string{"constructor error", "unsupported test", "canceled discovery"} {
		t.Run(mode, func(t *testing.T) {
			var readers []*closeReader
			cfg := Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://example.invalid", Method: "POST"}}}
			if mode == "constructor error" {
				cfg.TLSConfig = tlscfg.TLSConfig{TLSCA: t.TempDir() + "/missing"}
			}
			d, err := newDiscoverer(cfg, func() credentialfile.FileReader {
				r := &closeReader{FileReader: credentialfiletest.New(t)}
				readers = append(readers, r)
				return r
			})
			require.Equal(t, 1, readers[0].closes)
			if mode == "constructor error" {
				require.Error(t, err)
				require.Len(t, readers, 1)
				return
			}
			require.NoError(t, err)
			require.Zero(t, readers[1].closes)
			if mode == "unsupported test" {
				require.Error(t, d.Test(t.Context()))
			} else {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				d.Discover(ctx, make(chan []model.TargetGroup))
			}
			require.Equal(t, 1, readers[1].closes)
		})
	}
}

func TestHTTPClientUsesRuntimeReaderAfterConstruction(t *testing.T) {
	path := t.TempDir() + "/token"
	require.NoError(t, os.WriteFile(path, []byte("runtime-token"), 0o600))
	var readers []*closeReader
	cfg := Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "https://example.invalid", BearerTokenFile: path}}}
	d, err := newDiscoverer(cfg, func() credentialfile.FileReader {
		r := &closeReader{FileReader: credentialfiletest.New(t)}
		readers = append(readers, r)
		return r
	})
	require.NoError(t, err)
	defer d.files.Close()
	defer d.client.CloseIdleConnections()
	require.Len(t, readers, 2)
	require.Equal(t, 1, readers[0].closes)
	req, err := d.client.NewRequest(t.Context(), d.request)
	require.NoError(t, err)
	require.Equal(t, "Bearer runtime-token", req.Header.Get("Authorization"))
	require.Zero(t, readers[0].reads)
	require.Equal(t, 1, readers[1].reads)
}
