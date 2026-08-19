// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cache is generic over the client type; these tests exercise it with a
// trivial T=string fake so the memoize/distinct-key/reset/error semantics are
// verified independently of the CloudWatch and RGTA client surfaces.

type observedDoneContext struct {
	context.Context
	once    sync.Once
	entered chan<- struct{}
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { c.entered <- struct{}{} })
	return c.Context.Done()
}

func TestClientCache_MemoizesPerTargetRegion(t *testing.T) {
	var builds int
	cache := newClientCache(func(_ context.Context, target, region string) (string, error) {
		builds++
		return target + "/" + region, nil
	})

	c1, err := cache.forTargetRegion(context.Background(), "a1", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "a1/us-east-1", c1)

	c2, err := cache.forTargetRegion(context.Background(), "a1", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, c1, c2)
	assert.Equal(t, 1, builds, "same (target, region) is built once")

	_, err = cache.forTargetRegion(context.Background(), "a2", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, 2, builds, "a distinct target rebuilds")

	cache.reset()
	_, err = cache.forTargetRegion(context.Background(), "a1", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, 3, builds, "reset clears the cache")
}

func TestClientCache_BuildErrorNotCached(t *testing.T) {
	var builds int
	cache := newClientCache(func(_ context.Context, _, _ string) (string, error) {
		builds++
		return "", errors.New("boom")
	})

	_, err := cache.forTargetRegion(context.Background(), "a1", "r1")
	require.Error(t, err)
	_, err = cache.forTargetRegion(context.Background(), "a1", "r1")
	require.Error(t, err)
	assert.Equal(t, 2, builds, "a failed build is retried, not cached")
}

func TestClientCache_DistinctKeysBuildConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	cache := newClientCache(func(_ context.Context, target, region string) (string, error) {
		started <- struct{}{}
		<-release
		return target + "/" + region, nil
	})

	var wg sync.WaitGroup
	for _, target := range []string{"first", "second"} {
		wg.Go(func() {
			_, _ = cache.forTargetRegion(context.Background(), target, "us-east-1")
		})
	}

	allStarted := true
	for range 2 {
		select {
		case <-started:
		case <-time.After(250 * time.Millisecond):
			allStarted = false
		}
	}
	close(release)
	wg.Wait()
	assert.True(t, allStarted, "one slow client build must not serialize a distinct target/region")
}

func TestClientCache_CoalescesConcurrentSameKey(t *testing.T) {
	tests := map[string]struct {
		buildErr error
	}{
		"shared success": {},
		"shared error":   {buildErr: errors.New("build failed")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const callers = 8
			var builds atomic.Int32
			started := make(chan struct{})
			release := make(chan struct{})
			cache := newClientCache(func(_ context.Context, target, region string) (string, error) {
				builds.Add(1)
				close(started)
				<-release
				return target + "/" + region, tc.buildErr
			})

			results := make(chan string, callers)
			errs := make(chan error, callers)
			var wg sync.WaitGroup
			wg.Go(func() {
				result, err := cache.forTargetRegion(context.Background(), "base", "us-east-1")
				results <- result
				errs <- err
			})
			<-started

			entered := make(chan struct{}, callers-1)
			for range callers - 1 {
				wg.Go(func() {
					ctx := &observedDoneContext{Context: context.Background(), entered: entered}
					result, err := cache.forTargetRegion(ctx, "base", "us-east-1")
					results <- result
					errs <- err
				})
			}
			for range callers - 1 {
				<-entered
			}
			close(release)
			wg.Wait()
			close(results)
			close(errs)

			assert.Equal(t, int32(1), builds.Load())
			for result := range results {
				assert.Equal(t, "base/us-east-1", result)
			}
			for err := range errs {
				if tc.buildErr == nil {
					assert.NoError(t, err)
				} else {
					assert.ErrorIs(t, err, tc.buildErr)
				}
			}
		})
	}
}

func TestClientCache_SameKeyWaiterCancellationDoesNotCancelBuild(t *testing.T) {
	var builds atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	cache := newClientCache(func(_ context.Context, target, region string) (string, error) {
		builds.Add(1)
		close(started)
		<-release
		return target + "/" + region, nil
	})

	first := make(chan error, 1)
	go func() {
		_, err := cache.forTargetRegion(context.Background(), "base", "us-east-1")
		first <- err
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cache.forTargetRegion(ctx, "base", "us-east-1")
	assert.ErrorIs(t, err, context.Canceled)

	close(release)
	require.NoError(t, <-first)
	result, err := cache.forTargetRegion(context.Background(), "base", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "base/us-east-1", result)
	assert.Equal(t, int32(1), builds.Load())
}
