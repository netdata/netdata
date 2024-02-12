// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"

	"github.com/netdata/go.d.plugin/agent/confgroup"
)

func newRunningJobsCache() *runningJobsCache {
	return &runningJobsCache{}
}

func newRetryingJobsCache() *retryingJobsCache {
	return &retryingJobsCache{}
}

type (
	runningJobsCache  map[string]bool
	retryingJobsCache map[uint64]retryTask

	retryTask struct {
		cancel  context.CancelFunc
		timeout int
		retries int
	}
)

func (c runningJobsCache) put(cfg confgroup.Config) {
	c[cfg.FullName()] = true
}
func (c runningJobsCache) remove(cfg confgroup.Config) {
	delete(c, cfg.FullName())
}
func (c runningJobsCache) has(cfg confgroup.Config) bool {
	return c[cfg.FullName()]
}

func (c retryingJobsCache) put(cfg confgroup.Config, retry retryTask) {
	c[cfg.Hash()] = retry
}
func (c retryingJobsCache) remove(cfg confgroup.Config) {
	delete(c, cfg.Hash())
}
func (c retryingJobsCache) lookup(cfg confgroup.Config) (retryTask, bool) {
	v, ok := c[cfg.Hash()]
	return v, ok
}
