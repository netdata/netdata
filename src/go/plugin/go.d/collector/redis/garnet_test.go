// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCollector_GarnetInfoRequests(t *testing.T) {
	mock := newGarnetMock(dataGarnetInfoCommandstats)
	collr := newTestCollector(t, mock)
	// Make the refresh interval longer than any test run so the "at most one
	// request per interval" assertion does not depend on wall-clock time.
	collr.garnetKeyspace.refreshEvery = time.Hour

	ctx := context.Background()
	for range 3 {
		assert.NotEmpty(t, collr.Collect(ctx))
	}

	// keyspace is expensive on Garnet (full store scan): at most one request
	// within the refresh interval; commandstats is requested every cycle.
	assert.Equal(t, map[string]int{"all": 3, "keyspace": 1, "commandstats": 3}, mock.infoCalls)
}

func TestCollector_appendGarnetInfoSections(t *testing.T) {
	trim := func(data []byte) []byte { return []byte(strings.TrimRight(string(data), "\n")) }
	collr := newTestCollector(t, &mockRedisClient{
		info: map[string][]byte{
			"all":          trim(dataGarnetInfoAll),
			"keyspace":     trim(dataGarnetInfoKeyspace),
			"commandstats": trim(dataGarnetInfoCommandstats),
		},
	})
	collr.server = "garnet"

	// base INFO and extra sections without trailing newlines: the join must
	// still put every section on its own lines (exactly one newline boundary,
	// whatever line endings the sections use)
	info := collr.appendGarnetInfoSections(context.Background(), string(trim(dataGarnetInfoAll)))
	assert.Contains(t, info, "\n# Keyspace")
	assert.NotContains(t, info, "\n\n# Keyspace")
	assert.Contains(t, info, "\n# Commandstats")
	assert.NotContains(t, info, "\n\n# Commandstats")

	mx := make(map[string]int64)
	collr.collectInfo(mx, info)
	assert.Equal(t, int64(64), mx["arch_bits"])    // base section parsed
	assert.Equal(t, int64(6), mx["db0_keys"])      // keyspace section parsed
	assert.Equal(t, int64(1), mx["cmd_get_calls"]) // commandstats section parsed
}

func TestCollector_GarnetKeyspaceErrorClearsCache(t *testing.T) {
	mock := newGarnetMock(dataGarnetInfoCommandstats)
	collr := newTestCollector(t, mock)
	// interval 0: refresh on every cycle, so the test does not depend on timing
	collr.garnetKeyspace.refreshEvery = 0

	ctx := context.Background()
	mx := collr.Collect(ctx)
	assert.Equal(t, int64(6), mx["db0_keys"])

	delete(mock.info, "keyspace")
	mx = collr.Collect(ctx)
	assert.NotContains(t, mx, "db0_keys")

	mx = collr.Collect(ctx)
	assert.NotContains(t, mx, "db0_keys")
}
