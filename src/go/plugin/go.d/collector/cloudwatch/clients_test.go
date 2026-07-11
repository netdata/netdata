// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cache is generic over the client type; these tests exercise it with a
// trivial T=string fake so the memoize/distinct-key/reset/error semantics are
// verified independently of the CloudWatch and RGTA client surfaces.

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
	assert.Equal(t, 2, builds, "a distinct account rebuilds")

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
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cache.forTargetRegion(context.Background(), target, "us-east-1")
		}()
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
