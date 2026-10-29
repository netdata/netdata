//go:build !windows

// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnixRPC(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "rpc.sock")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(
			w,
			`<?xml version="1.0"?><methodResponse><params><param><value><array><data></data></array></value></param></params></methodResponse>`,
		)
	}))
	require.NoError(t, server.Listener.Close())
	server.Listener = listener
	server.Start()
	defer server.Close()
	u := &url.URL{
		Scheme: "unix",
		Path:   socket,
	}
	client, err := newSupervisorRPCClient(context.Background(), u, web.ClientConfig{})
	require.NoError(t, err)
	defer client.closeIdleConnections()
	info, err := client.getAllProcessInfo()
	require.NoError(t, err)
	assert.Empty(t, info)
	_, err = newSupervisorRPCClient(context.Background(), u, web.ClientConfig{
		ForceHTTP2: true,
	})
	assert.ErrorContains(t, err, "unexpected HTTPConfig client transport")
}
