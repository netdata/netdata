// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	uid        string
	key        string
	sourceType string
	priority   int
	source     string
	hash       uint64
}

func (c testConfig) UID() string             { return c.uid }
func (c testConfig) ExposedKey() string      { return c.key }
func (c testConfig) SourceType() string      { return c.sourceType }
func (c testConfig) SourceTypePriority() int { return c.priority }
func (c testConfig) Source() string          { return c.source }
func (c testConfig) Hash() uint64            { return c.hash }

func seenCacheCount(c *SeenCache[testConfig]) int {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return len(c.items)
}

func exposedCacheCount(c *ExposedCache[testConfig]) int {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return len(c.items)
}

func lookupSeenByUID(c *SeenCache[testConfig], uid string) (testConfig, bool) {
	return c.Lookup(testConfig{uid: uid})
}

func TestSeenCache_AddAndLookup(t *testing.T) {
	c := NewSeenCache[testConfig]()
	cfg := testConfig{uid: "uid1", key: "key1"}

	c.Add(cfg)

	got, ok := c.Lookup(cfg)
	require.True(t, ok)
	assert.Equal(t, cfg, got)
}

func TestSeenCache_Remove(t *testing.T) {
	c := NewSeenCache[testConfig]()
	cfg := testConfig{uid: "uid1", key: "key1"}

	c.Add(cfg)
	c.Remove(cfg)

	_, ok := c.Lookup(cfg)
	assert.False(t, ok)
}

func TestSeenCache_AddOverwrites(t *testing.T) {
	c := NewSeenCache[testConfig]()
	cfg1 := testConfig{uid: "uid1", key: "key1", hash: 100}
	cfg2 := testConfig{uid: "uid1", key: "key1", hash: 200}

	c.Add(cfg1)
	c.Add(cfg2)

	got, ok := c.Lookup(cfg2)
	require.True(t, ok)
	assert.Equal(t, uint64(200), got.Hash())
}

func TestSeenCache_RemoveNonexistent(t *testing.T) {
	c := NewSeenCache[testConfig]()
	cfg := testConfig{uid: "uid1"}

	// Should not panic.
	c.Remove(cfg)
}

func TestExposedCache_AddAndLookup(t *testing.T) {
	c := NewExposedCache[testConfig]()
	cfg := testConfig{uid: "uid1", key: "key1"}
	entry := &Entry[testConfig]{Cfg: cfg, Status: StatusAccepted}

	c.Add(entry)

	got, ok := c.LookupByKey("key1")
	require.True(t, ok)
	assert.Equal(t, StatusAccepted, got.Status)
	assert.Equal(t, cfg, got.Cfg)
}

func TestExposedCache_LookupByKey_NotFound(t *testing.T) {
	c := NewExposedCache[testConfig]()

	_, ok := c.LookupByKey("nonexistent")
	assert.False(t, ok)
}

func TestExposedCache_Remove(t *testing.T) {
	c := NewExposedCache[testConfig]()
	cfg := testConfig{uid: "uid1", key: "key1"}
	entry := &Entry[testConfig]{Cfg: cfg, Status: StatusAccepted}

	c.Add(entry)
	c.Remove(cfg)

	_, ok := c.LookupByKey("key1")
	assert.False(t, ok)
}

func TestExposedCache_AddOverwritesByKey(t *testing.T) {
	c := NewExposedCache[testConfig]()
	cfg1 := testConfig{uid: "uid1", key: "key1"}
	cfg2 := testConfig{uid: "uid2", key: "key1"} // same Key, different UID
	entry1 := &Entry[testConfig]{Cfg: cfg1, Status: StatusRunning}
	entry2 := &Entry[testConfig]{Cfg: cfg2, Status: StatusAccepted}

	c.Add(entry1)
	c.Add(entry2)

	got, ok := c.LookupByKey("key1")
	require.True(t, ok)
	assert.Equal(t, "uid2", got.Cfg.UID())
	assert.Equal(t, StatusAccepted, got.Status)
}

func TestExposedCache_PointerMutationVisible(t *testing.T) {
	c := NewExposedCache[testConfig]()
	cfg := testConfig{uid: "uid1", key: "key1"}
	entry := &Entry[testConfig]{Cfg: cfg, Status: StatusAccepted}

	c.Add(entry)

	got, ok := c.LookupByKey("key1")
	require.True(t, ok)

	// Mutating through the returned pointer updates the cache.
	got.Status = StatusRunning

	got2, _ := c.LookupByKey("key1")
	assert.Equal(t, StatusRunning, got2.Status)
}
