// SPDX-License-Identifier: GPL-3.0-or-later

package profilecatalog

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCached_cachesSuccessfulLoads(t *testing.T) {
	calls := 0
	c := NewCached(func() (string, error) {
		calls++
		return "value", nil
	})
	c.CacheEnabled = func() bool { return true }

	first, err := c.Get()
	require.NoError(t, err)
	second, err := c.Get()
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	assert.Equal(t, "value", first)
	assert.Equal(t, "value", second)
}

func TestCached_retriesAfterFailure(t *testing.T) {
	calls := 0
	c := NewCached(func() (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("boom")
		}
		return "value", nil
	})
	c.CacheEnabled = func() bool { return true }

	_, err := c.Get()
	require.Error(t, err)

	got, err := c.Get()
	require.NoError(t, err)

	assert.Equal(t, 2, calls, "a failed load must not be cached")
	assert.Equal(t, "value", got)
}

func TestCached_doesNotCacheWhenDisabled(t *testing.T) {
	calls := 0
	c := NewCached(func() (string, error) {
		calls++
		return "value", nil
	})
	c.CacheEnabled = func() bool { return false }

	_, err := c.Get()
	require.NoError(t, err)
	_, err = c.Get()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
}

func TestCached_reset(t *testing.T) {
	calls := 0
	c := NewCached(func() (string, error) {
		calls++
		return "value", nil
	})
	c.CacheEnabled = func() bool { return true }

	_, err := c.Get()
	require.NoError(t, err)
	c.Reset()
	_, err = c.Get()
	require.NoError(t, err)

	assert.Equal(t, 2, calls, "Reset must force a reload")
}

// TestCached_defaultDisabledUnderTest documents why the caching tests above must
// override CacheEnabled: the process-default gate is off under the test binary.
func TestCached_defaultDisabledUnderTest(t *testing.T) {
	c := NewCached(func() (string, error) { return "value", nil })
	assert.False(t, c.CacheEnabled(), "caching is disabled under `go test` (executable.Name == test)")
}
