// SPDX-License-Identifier: GPL-3.0-or-later

package profilecatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_lenAndEmpty(t *testing.T) {
	empty, err := Load[tProfile](nil, Options[tProfile]{Decode: decodeTest})
	require.NoError(t, err)
	assert.True(t, empty.Empty())
	assert.Equal(t, 0, empty.Len())

	specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{
		"a.yaml": "1", "b.yaml": "2",
	}}})
	cat, err := Load(specs, Options[tProfile]{Decode: decodeTest})
	require.NoError(t, err)
	assert.False(t, cat.Empty())
	assert.Equal(t, 2, cat.Len())
}

func TestCatalog_getAndHas(t *testing.T) {
	specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{"app.yaml": "one"}}})
	cat, err := Load(specs, Options[tProfile]{Decode: decodeTest})
	require.NoError(t, err)

	got, ok := cat.Get("app")
	require.True(t, ok)
	assert.Equal(t, "one", got.Content)
	assert.True(t, cat.Has("app"))

	_, ok = cat.Get("missing")
	assert.False(t, ok)
	assert.False(t, cat.Has("missing"))
}

func TestCatalog_sorted(t *testing.T) {
	specs := buildSpecs(t, []dirFiles{{isStock: true, files: map[string]string{
		"c.yaml": "3", "a.yaml": "1", "b.yaml": "2",
	}}})
	cat, err := Load(specs, Options[tProfile]{Decode: decodeTest})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, names(cat.Sorted()))
}

func TestCatalog_stockTracking(t *testing.T) {
	// stock: a, b ; user overrides b and adds c.
	specs := buildSpecs(t, []dirFiles{
		{isStock: true, files: map[string]string{"a.yaml": "sa", "b.yaml": "sb"}},
		{isStock: false, files: map[string]string{"b.yaml": "ub", "c.yaml": "uc"}},
	})
	cat, err := Load(specs, Options[tProfile]{Decode: decodeTest})
	require.NoError(t, err)

	// StockNames reports basenames that had a stock profile, even when overridden.
	assert.Equal(t, []string{"a", "b"}, cat.StockNames())
	assert.True(t, cat.HasStock("a"))
	assert.True(t, cat.HasStock("b"), "b had a stock profile even though a user overrode it")
	assert.False(t, cat.HasStock("c"))

	// EffectiveIsStock reflects the CURRENT winner: a is stock; b was overridden
	// by a user profile so it is no longer stock; c is user-only.
	assert.True(t, cat.EffectiveIsStock("a"))
	assert.False(t, cat.EffectiveIsStock("b"))
	assert.False(t, cat.EffectiveIsStock("c"))
	assert.False(t, cat.EffectiveIsStock("missing"))

	// The winning content for b is the user's.
	got, ok := cat.Get("b")
	require.True(t, ok)
	assert.Equal(t, "ub", got.Content)
}

func TestCatalog_zeroValueLookupIsSafe(t *testing.T) {
	var c Catalog[tProfile]
	_, ok := c.Get("x")
	assert.False(t, ok)
	assert.True(t, c.Empty())
	assert.Nil(t, c.InOrder())
	assert.Nil(t, c.Sorted())
	assert.Nil(t, c.StockNames())
	assert.False(t, c.HasStock("x"))
	assert.False(t, c.EffectiveIsStock("x"))
}
