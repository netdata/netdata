// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func inactivityServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	previous := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	return server
}

func requireInactivity(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, errDownloadInactive)
	require.False(t, errors.Is(err, context.Canceled))
	var timeout net.Error
	require.ErrorAs(t, err, &timeout)
	require.True(t, timeout.Timeout())
	require.Contains(t, err.Error(), "network inactivity timeout")
}

func TestDownloadInactivityHeadersAndBody(t *testing.T) {
	for _, bodyStall := range []bool{false, true} {
		t.Run(map[bool]string{false: "headers", true: "body"}[bodyStall], func(t *testing.T) {
			stopped := make(chan struct{})
			server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				if bodyStall {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			source, err := openDownload(ctx, server.URL, 100*time.Millisecond)
			if bodyStall {
				require.NoError(t, err)
				defer source.Close()
				_, err = io.ReadAll(source)
			}
			requireInactivity(t, err)
			select {
			case <-stopped:
			case <-ctx.Done():
				t.Fatal("timeout did not interrupt HTTP I/O")
			}
		})
	}
}

func TestDownloadProgressHasNoOverallDeadline(t *testing.T) {
	const idle = 300 * time.Millisecond
	const count = 12
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < count; i++ {
			if _, err := io.WriteString(w, "x"); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-time.After(50 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	source, err := openDownload(ctx, server.URL, idle)
	require.NoError(t, err)
	defer source.Close()
	data, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("x", count), string(data))
	require.Greater(t, time.Since(started), idle)
}

func TestDownloadInactivityIgnoresLocalWorkBetweenReads(t *testing.T) {
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "xy") })
	source, err := openDownload(context.Background(), server.URL, 50*time.Millisecond)
	require.NoError(t, err)
	defer source.Close()
	first := make([]byte, 1)
	_, err = io.ReadFull(source, first)
	require.NoError(t, err)
	time.Sleep(150 * time.Millisecond) // Local archive processing has no network timer.
	rest, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, "xy", string(first)+string(rest))
}

func TestDownloadExplicitCancellationAndCloseInterruptRead(t *testing.T) {
	for _, closeBody := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "close"}[closeBody], func(t *testing.T) {
			stopped := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(stopped)
				// Keep a normal response EOF from racing the interrupted read.
				<-release
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source, err := openDownload(ctx, server.URL, time.Minute)
			require.NoError(t, err)
			defer source.Close()
			result := make(chan error, 1)
			go func() { _, err := io.ReadAll(source); result <- err }()
			if closeBody {
				require.NoError(t, source.Close())
			} else {
				cancel()
			}
			select {
			case err := <-result:
				require.Error(t, err)
				require.False(t, errors.Is(err, errDownloadInactive))
				if !closeBody {
					require.ErrorIs(t, err, context.Canceled)
				}
			case <-time.After(time.Second):
				t.Fatal("HTTP read did not stop")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("HTTP request did not close")
			}
		})
	}
}

func TestDownloadRedirectRefreshesInactivity(t *testing.T) {
	const idle = 300 * time.Millisecond
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(180 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/second", http.StatusFound)
			return
		}
		if r.URL.Path == "/second" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		io.WriteString(w, "done")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	source, err := openDownload(ctx, server.URL, idle)
	require.NoError(t, err)
	defer source.Close()
	data, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, "done", string(data))
	require.Greater(t, time.Since(started), idle)
}

func TestDownloadCloseCannotCancelFollowingRequest(t *testing.T) {
	const idle = time.Second
	server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	for i := 0; i < 5; i++ {
		source, err := openDownload(context.Background(), server.URL, idle)
		require.NoError(t, err)
		require.NoError(t, source.Close())
	}
	source, err := openDownload(context.Background(), server.URL, idle)
	require.NoError(t, err)
	defer source.Close()
	time.Sleep(idle + 100*time.Millisecond)
	data, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))
}

func TestDownloadExactByteEndingPreservesTimeoutAndCancellation(t *testing.T) {
	for _, voluntary := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "cancel"}[voluntary], func(t *testing.T) {
			payload := []byte("exact archive bytes")
			release := make(chan struct{})
			defer close(release)
			server := inactivityServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(payload)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				// Do not let handler completion race cancellation with a normal EOF.
				<-release
			})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			idle := 100 * time.Millisecond
			if voluntary {
				idle = time.Minute
			}
			source, err := openDownload(ctx, server.URL, idle)
			require.NoError(t, err)
			defer source.Close()
			file, err := os.CreateTemp(t.TempDir(), "archive")
			require.NoError(t, err)
			defer file.Close()
			archive := Archive{Bytes: int64(len(payload)), SHA256: fmt.Sprintf("%x", sha256.Sum256(payload))}
			done := make(chan error, 1)
			go func() { done <- saveArchive(source, file, archive) }()
			if voluntary {
				require.Eventually(t, func() bool { info, err := file.Stat(); return err == nil && info.Size() == int64(len(payload)) }, time.Second, time.Millisecond)
				cancel()
			}
			select {
			case err := <-done:
				if voluntary {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					requireInactivity(t, err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("archive ending read did not finish")
			}
		})
	}
}
