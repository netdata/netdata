// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cache is generic over the client type; these tests exercise it with a
// trivial T=string fake so the memoize/distinct-key/reset/error semantics are
// verified independently of the CloudWatch and RGTA client surfaces.

func TestClientCache_MemoizesPerAccountRegion(t *testing.T) {
	var builds int
	cache := newClientCache(func(_ context.Context, account, region string) (string, error) {
		builds++
		return account + "/" + region, nil
	})

	c1, err := cache.forAccountRegion(context.Background(), "a1", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "a1/us-east-1", c1)

	c2, err := cache.forAccountRegion(context.Background(), "a1", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, c1, c2)
	assert.Equal(t, 1, builds, "same (account, region) is built once")

	_, err = cache.forAccountRegion(context.Background(), "a2", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, 2, builds, "a distinct account rebuilds")

	cache.reset()
	_, err = cache.forAccountRegion(context.Background(), "a1", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, 3, builds, "reset clears the cache")
}

func TestClientCache_BuildErrorNotCached(t *testing.T) {
	var builds int
	cache := newClientCache(func(_ context.Context, _, _ string) (string, error) {
		builds++
		return "", errors.New("boom")
	})

	_, err := cache.forAccountRegion(context.Background(), "a1", "r1")
	require.Error(t, err)
	_, err = cache.forAccountRegion(context.Background(), "a1", "r1")
	require.Error(t, err)
	assert.Equal(t, 2, builds, "a failed build is retried, not cached")
}
