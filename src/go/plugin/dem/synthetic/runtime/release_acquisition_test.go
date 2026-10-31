// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"

	"github.com/stretchr/testify/require"
)

func TestReleaseAcquisitionAuthenticatesRemoteAndOffline(t *testing.T) {
	release, key := releaseFixture(t)
	keys := testTrust(t, key)
	signed, err := pgptest.Release(release, key)
	require.NoError(t, err)
	var calls atomic.Int32
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write(signed)
	})
	got, err := FetchRelease(context.Background(), server.URL, keys)
	require.NoError(t, err)
	require.Equal(t, release, got)
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, signed, 0600))
	got, err = ReadRelease(path, keys)
	require.NoError(t, err)
	require.Equal(t, release, got)
	_, err = FetchRelease(context.Background(), server.URL, nil)
	require.ErrorContains(t, err, "no runtime release verification key")
	require.Equal(t, int32(1), calls.Load(), "missing trust must fail before network access")
	wrong := pgptest.Key(t, 9)
	_, err = ReadRelease(path, testTrust(t, wrong))
	require.ErrorContains(t, err, "not signed by a trusted key")
}

func TestReleaseAcquisitionBoundsAndTransport(t *testing.T) {
	_, key := releaseFixture(t)
	keys := testTrust(t, key)
	for _, address := range []string{"http://example.invalid/release", "https://user:secret@example.invalid/release", "https://example.invalid/release#fragment"} {
		_, err := FetchRelease(context.Background(), address, keys)
		require.ErrorContains(t, err, "requires HTTPS")
		require.NotContains(t, err.Error(), "secret")
	}
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "http://example.invalid/release", http.StatusFound)
		case "/failure":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = io.WriteString(w, strings.Repeat(" ", ReleaseEnvelopeLimit+1))
		}
	})
	_, err := FetchRelease(context.Background(), server.URL, keys)
	require.ErrorContains(t, err, "exceeds protocol size limit")
	_, err = FetchRelease(context.Background(), server.URL+"/redirect", keys)
	require.Error(t, err)
	_, err = FetchRelease(context.Background(), server.URL+"/failure", keys)
	require.ErrorContains(t, err, "HTTP 503")
	path := filepath.Join(t.TempDir(), "oversized.json")
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte{' '}, ReleaseEnvelopeLimit+1), 0600))
	_, err = ReadRelease(path, keys)
	require.ErrorContains(t, err, "exceeds protocol size limit")
}
