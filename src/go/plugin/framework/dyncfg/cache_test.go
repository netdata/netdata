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

func TestSeenCache_AddAndLookup(t *testing.T) {
	c := NewSeenCache[testConfig]()
	cfg := testConfig{uid: "uid1", key: "key1"}

	c.Add(cfg)

	got, ok := c.Lookup(cfg)
	require.True(t, ok)
	assert.Equal(t, cfg, got)
}

func TestSeenCache_LookupByUID(t *testing.T) {
	c := NewSeenCache[testConfig]()
	cfg := testConfig{uid: "uid1", key: "key1"}

	c.Add(cfg)

	got, ok := c.LookupByUID("uid1")
	require.True(t, ok)
	assert.Equal(t, cfg, got)

	_, ok = c.LookupByUID("nonexistent")
	assert.False(t, ok)
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

	got, ok := c.LookupByUID("uid1")
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

func TestExposedCache_Count(t *testing.T) {
	c := NewExposedCache[testConfig]()
	assert.Equal(t, 0, c.Count())

	c.Add(&Entry[testConfig]{Cfg: testConfig{key: "a"}})
	c.Add(&Entry[testConfig]{Cfg: testConfig{key: "b"}})
	assert.Equal(t, 2, c.Count())

	c.Remove(testConfig{key: "a"})
	assert.Equal(t, 1, c.Count())
}

func TestExposedCache_ForEach(t *testing.T) {
	c := NewExposedCache[testConfig]()
	c.Add(&Entry[testConfig]{Cfg: testConfig{key: "a"}, Status: StatusRunning})
	c.Add(&Entry[testConfig]{Cfg: testConfig{key: "b"}, Status: StatusDisabled})

	keys := make(map[string]Status)
	c.ForEach(func(key string, entry *Entry[testConfig]) bool {
		keys[key] = entry.Status
		return true
	})

	assert.Len(t, keys, 2)
	assert.Equal(t, StatusRunning, keys["a"])
	assert.Equal(t, StatusDisabled, keys["b"])
}

func TestExposedCache_ForEach_EarlyStop(t *testing.T) {
	c := NewExposedCache[testConfig]()
	c.Add(&Entry[testConfig]{Cfg: testConfig{key: "a"}})
	c.Add(&Entry[testConfig]{Cfg: testConfig{key: "b"}})
	c.Add(&Entry[testConfig]{Cfg: testConfig{key: "c"}})

	count := 0
	c.ForEach(func(_ string, _ *Entry[testConfig]) bool {
		count++
		return false // stop after first
	})

	assert.Equal(t, 1, count)
}
