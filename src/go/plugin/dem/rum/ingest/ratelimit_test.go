// SPDX-License-Identifier: GPL-3.0-or-later
package ingest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIPLimitBoundsActiveKeys(t *testing.T) {
	now := time.Unix(1000, 0)
	limit := newLimiter(1, 2, 2)
	assert.True(t, limit.allow("first", now))
	assert.True(t, limit.allow("second", now))
	assert.False(t, limit.allow("third", now), "new clients cannot allocate beyond the IP tracking budget")
	assert.True(t, limit.allow("first", now), "admitted clients retain their remaining tokens")
	assert.False(t, limit.allow("first", now))
	assert.True(t, limit.allow("third", now.Add(3*time.Second)), "fully replenished idle buckets may be reclaimed")
}
