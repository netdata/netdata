// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package web

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/stretchr/testify/require"
)

// Use the actual helper to verify public constructors launch and reap one child
// per file operation on both success and failure.
func oneShotHelperFixture(t *testing.T) (dir, pidLog string) {
	t.Helper()
	helper := os.Getenv("NETDATA_TEST_ND_RUN")
	if helper == "" {
		t.Skip("set NETDATA_TEST_ND_RUN to a prebuilt nd-run to test one-shot file operations")
	}
	helper, err := filepath.Abs(helper)
	require.NoError(t, err)
	dir, err = os.MkdirTemp("/tmp", "netdata-oneshot-files-test-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	require.NoError(t, os.Chmod(dir, 0755))
	pidLog = filepath.Join(dir, "children")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\nprintf '%s\\n' \"$$\" >> " + quote(pidLog) + "\nexec " + quote(helper) + " \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nd-run"), []byte(script), 0755))
	oldBinDir := buildinfo.NetdataBinDir
	buildinfo.NetdataBinDir = dir
	t.Cleanup(func() { buildinfo.NetdataBinDir = oldBinDir })

	return dir, pidLog
}

func TestCOneShotTLSConstruction(t *testing.T) {
	dir, pidLog := oneShotHelperFixture(t)

	server := httptest.NewTLSServer(nil)
	defer server.Close()
	pair := server.TLS.Certificates[0]
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	require.NoError(t, err)
	key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	certPath, keyPath := filepath.Join(dir, "cert"), filepath.Join(dir, "key")
	require.NoError(t, os.WriteFile(certPath, cert, 0644))
	require.NoError(t, os.WriteFile(keyPath, key, 0644))
	cfg := tlscfg.TLSConfig{TLSCA: certPath, TLSCert: certPath, TLSKey: keyPath}

	constructors := map[string]func(tlscfg.TLSConfig) error{
		"tls": func(cfg tlscfg.TLSConfig) error {
			_, err := tlscfg.NewTLSConfig(context.Background(), cfg)
			return err
		},
		"http": func(cfg tlscfg.TLSConfig) error {
			client, err := NewHTTPClient(context.Background(), ClientConfig{TLSConfig: cfg})
			if client != nil {
				client.CloseIdleConnections()
			}
			return err
		},
	}
	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			for _, fail := range []bool{false, true} {
				require.NoError(t, os.WriteFile(pidLog, nil, 0600))
				input := cfg
				if fail {
					input.TLSKey = filepath.Join(dir, "missing-key")
				}
				err := construct(input)
				if fail {
					require.ErrorIs(t, err, os.ErrNotExist)
				} else {
					require.NoError(t, err)
				}
				logged, err := os.ReadFile(pidLog)
				require.NoError(t, err)
				pids := strings.Fields(string(logged))
				require.Len(t, pids, 3, "each TLS file must use a fresh child")
				for _, text := range pids {
					pid, err := strconv.Atoi(text)
					require.NoError(t, err)
					require.Eventually(
						t,
						func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH },
						5*time.Second,
						10*time.Millisecond,
						"constructor must reap each helper",
					)
				}
			}
		})
	}
}

// Public request helpers launch only for configured files and retire each
// operation's child. Rotation, permission changes and deletion stay visible.
func TestCImplicitBearerReads(t *testing.T) {
	dir, pidLog := oneShotHelperFixture(t)
	client, err := NewHTTPClient(context.Background(), ClientConfig{})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	_, err = NewHTTPRequest(context.Background(), RequestConfig{URL: "http://localhost"})
	require.NoError(t, err)
	_, err = os.Stat(pidLog)
	require.ErrorIs(t, err, os.ErrNotExist, "no-file operations must not start a helper")

	path := filepath.Join(dir, "token")
	cfg := RequestConfig{URL: "http://localhost", BearerTokenFile: path}
	for _, token := range []string{"synthetic-first", "synthetic-replaced"} {
		require.NoError(t, os.WriteFile(path+".new", []byte(token), 0644))
		require.NoError(t, os.Rename(path+".new", path))
		req, err := NewHTTPRequest(context.Background(), cfg)
		require.NoError(t, err)
		require.Equal(t, "Bearer "+token, req.Header.Get("Authorization"))
	}
	require.NoError(t, os.Chmod(path, 0000))
	_, err = NewHTTPRequest(context.Background(), cfg)
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoError(t, os.Remove(path))
	_, err = NewHTTPRequestWithPath(context.Background(), cfg, "metrics")
	require.ErrorIs(t, err, os.ErrNotExist)

	logged, err := os.ReadFile(pidLog)
	require.NoError(t, err)
	pids := strings.Fields(string(logged))
	require.Len(t, pids, 4, "each file operation must use a fresh helper")
	for _, text := range pids {
		pid, err := strconv.Atoi(text)
		require.NoError(t, err)
		require.Eventually(
			t,
			func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH },
			5*time.Second,
			10*time.Millisecond,
			"request helper must be closed and reaped",
		)
	}
}
