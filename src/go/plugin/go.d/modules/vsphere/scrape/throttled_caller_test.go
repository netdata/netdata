// SPDX-License-Identifier: GPL-3.0-or-later

package scrape

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_throttledCaller(t *testing.T) {
	var current int64
	var maxv int64
	var total int64
	var mux sync.Mutex
	limit := 5
	n := 10000
	tc := newThrottledCaller(limit)

	for i := 0; i < n; i++ {
		job := func() {
			atomic.AddInt64(&total, 1)
			atomic.AddInt64(&current, 1)
			time.Sleep(100 * time.Microsecond)

			mux.Lock()
			defer mux.Unlock()
			if atomic.LoadInt64(&current) > maxv {
				maxv = atomic.LoadInt64(&current)
			}
			atomic.AddInt64(&current, -1)
		}
		tc.call(job)
	}
	tc.wait()

	assert.Equal(t, int64(n), total)
	assert.Equal(t, maxv, int64(limit))
}
