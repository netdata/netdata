// SPDX-License-Identifier: GPL-3.0-or-later

package netlistensd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

var _ dyncfg.Testable = (*Discoverer)(nil)

func TestDiscovererTest(t *testing.T) {
	t.Run("runs and parses exactly one snapshot without runtime side effects", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)

		var calls int
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			calls++
			return []byte("TCP|127.0.0.1|19999|/usr/bin/example\n"), nil
		})
		d.successRuns = 3
		d.timeoutRuns = 2
		d.cache[42] = &cacheItem{}

		require.NoError(t, d.Test(t.Context()))
		require.Equal(t, 1, calls)
		require.EqualValues(t, 3, d.successRuns)
		require.EqualValues(t, 2, d.timeoutRuns)
		require.Len(t, d.cache, 1)
		require.Contains(t, d.cache, uint64(42))
	})

	t.Run("accepts an empty snapshot", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)

		var calls int
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			calls++
			return nil, nil
		})

		require.NoError(t, d.Test(t.Context()))
		require.Equal(t, 1, calls)
	})

	t.Run("sanitizes helper failures", func(t *testing.T) {
		privateErr := errors.New("helper failed at [PRIVATE_ENDPOINT]")
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			return nil, privateErr
		})

		err = d.Test(t.Context())

		require.ErrorIs(t, err, privateErr)
		require.Equal(t, "cannot inspect local network listeners", err.Error())
		require.NotContains(t, err.Error(), "[PRIVATE_ENDPOINT]")
	})

	t.Run("sanitizes invalid helper output", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			return []byte("invalid [REDACTED_SECRET]\n"), nil
		})

		err = d.Test(t.Context())

		require.Error(t, err)
		require.Equal(t, "local listener inspection returned invalid data", err.Error())
		require.NotContains(t, err.Error(), "[REDACTED_SECRET]")
	})

	t.Run("rejects a snapshot truncated by the scanner", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			line := "TCP|127.0.0.1|19999|/" + strings.Repeat("x", bufio.MaxScanTokenSize)
			return []byte(line + "\n"), nil
		})

		err = d.Test(t.Context())

		require.Error(t, err)
		require.Equal(t, "local listener inspection returned invalid data", err.Error())
	})

	t.Run("classifies a missing helper as a public unavailability error", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			return nil, fmt.Errorf("%w ('/plugins.d/local-listeners')", errLocalListenersNotInstalled)
		})

		err = d.Test(t.Context())

		require.ErrorIs(t, err, errLocalListenersNotInstalled)
		require.Equal(t, "local network listener inspection is not available on this system", err.Error())
	})

	t.Run("classifies configured helper timeout", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			return nil, context.DeadlineExceeded
		})

		err = d.Test(t.Context())

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, "local listener inspection did not complete before the timeout", err.Error())
	})

	t.Run("preserves preexisting caller cancellation cause after entering helper path", func(t *testing.T) {
		cancelCause := errors.New("test attempt superseded")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cancelCause)

		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		var calls int
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			calls++
			return nil, nil
		})

		err = d.Test(ctx)

		require.ErrorIs(t, err, cancelCause)
		_, public := dyncfg.PublicMessage(err)
		require.False(t, public)
		require.Equal(t, 1, calls)
	})

	t.Run("preserves inflight caller cancellation cause", func(t *testing.T) {
		cancelCause := errors.New("test attempt superseded")
		ctx, cancel := context.WithCancelCause(t.Context())

		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(ctx context.Context) ([]byte, error) {
			cancel(cancelCause)
			<-ctx.Done()
			return nil, context.Cause(ctx)
		})

		err = d.Test(ctx)

		require.ErrorIs(t, err, cancelCause)
		_, public := dyncfg.PublicMessage(err)
		require.False(t, public)
	})

	t.Run("parser failure wins over cancellation racing the fast parse", func(t *testing.T) {
		cancelCause := errors.New("test attempt superseded")
		ctx, cancel := context.WithCancelCause(t.Context())

		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			cancel(cancelCause)
			return []byte("invalid [REDACTED_SECRET]\n"), nil
		})

		err = d.Test(ctx)

		require.Equal(t, "local listener inspection returned invalid data", err.Error())
		require.NotContains(t, err.Error(), "[REDACTED_SECRET]")
		require.NotErrorIs(t, err, cancelCause)
	})

	t.Run("does not classify the caller deadline as the configured helper timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()

		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(ctx context.Context) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})

		err = d.Test(ctx)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		_, public := dyncfg.PublicMessage(err)
		require.False(t, public)
	})
}

func TestDiscoverLocalListenersCancellationAndTimeoutClassification(t *testing.T) {
	t.Run("caller cancellation is a clean stop without runtime side effects", func(t *testing.T) {
		cancelCause := errors.New("discovery attempt superseded")
		ctx, cancel := context.WithCancelCause(t.Context())

		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(ctx context.Context) ([]byte, error) {
			cancel(cancelCause)
			<-ctx.Done()
			return nil, context.Cause(ctx)
		})
		d.cache[42] = &cacheItem{}

		require.NoError(t, d.discoverLocalListeners(ctx, make(chan []model.TargetGroup, 1)))
		require.Zero(t, d.successRuns)
		require.Zero(t, d.timeoutRuns)
		require.Len(t, d.cache, 1)
		require.Contains(t, d.cache, uint64(42))
	})

	t.Run("sixth configured timeout stops discovery before any success", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			return nil, context.DeadlineExceeded
		})

		for range 5 {
			require.NoError(t, d.discoverLocalListeners(t.Context(), make(chan []model.TargetGroup, 1)))
		}
		err = d.discoverLocalListeners(t.Context(), make(chan []model.TargetGroup, 1))

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Zero(t, d.successRuns)
		require.EqualValues(t, 6, d.timeoutRuns)
	})

	t.Run("a successful snapshot keeps later configured timeouts nonfatal", func(t *testing.T) {
		d, err := NewDiscoverer(Config{})
		require.NoError(t, err)
		var calls int
		d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
			calls++
			if calls == 1 {
				return nil, nil
			}
			return nil, context.DeadlineExceeded
		})

		require.NoError(t, d.discoverLocalListeners(t.Context(), make(chan []model.TargetGroup, 1)))
		for range 6 {
			require.NoError(t, d.discoverLocalListeners(t.Context(), make(chan []model.TargetGroup, 1)))
		}

		require.EqualValues(t, 1, d.successRuns)
		require.EqualValues(t, 6, d.timeoutRuns)
	})
}

func TestDiscoverStopsQuietlyWhenHelperIsNotInstalled(t *testing.T) {
	var buf safeBuffer

	d, err := NewDiscoverer(Config{})
	require.NoError(t, err)
	d.Logger = logger.NewWithWriter(&buf)
	d.ll = localListenersFunc(func(context.Context) ([]byte, error) {
		return nil, fmt.Errorf("%w ('/plugins.d/local-listeners')", errLocalListenersNotInstalled)
	})

	done := make(chan struct{})
	go func() { defer close(done); d.Discover(t.Context(), make(chan []model.TargetGroup, 1)) }()

	select {
	case <-done:
	case <-time.After(time.Second * 5):
		t.Fatal("Discover did not return after the helper was reported as not installed")
	}

	out := strings.ToLower(buf.String())
	require.Contains(t, out, "level=info")
	require.Contains(t, out, "discovery is disabled")
	require.NotContains(t, out, "level=error")
}

type safeBuffer struct {
	mux sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mux.Lock()
	defer b.mux.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mux.Lock()
	defer b.mux.Unlock()
	return b.buf.String()
}

type localListenersFunc func(context.Context) ([]byte, error)

func (fn localListenersFunc) discover(ctx context.Context) ([]byte, error) {
	return fn(ctx)
}
