// SPDX-License-Identifier: GPL-3.0-or-later

package scrape

import "sync"

type throttledCaller struct {
	limit chan struct{}
	wg    sync.WaitGroup
}

func newThrottledCaller(limit int) *throttledCaller {
	if limit <= 0 {
		panic("limit must be > 0")
	}
	return &throttledCaller{limit: make(chan struct{}, limit)}
}

func (t *throttledCaller) call(job func()) {
	t.wg.Go(func() {
		t.limit <- struct{}{}
		defer func() {
			<-t.limit
		}()
		job()
	})
}

func (t *throttledCaller) wait() {
	t.wg.Wait()
}
