// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/tickstate"
)

const (
	penaltyStep = 5
	maxPenalty  = 600
	infTries    = -1
)

// stopController coordinates stop request and completion acknowledgement.
// It is safe for repeated Stop calls and pre-start stop requests.
type stopController struct {
	stopCh    chan struct{}
	stoppedCh chan struct{}

	started atomic.Bool

	stopOnce    sync.Once
	stoppedOnce sync.Once
}

func newStopController() stopController {
	return stopController{
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

func (c *stopController) markStarted() {
	c.started.Store(true)
}

func (c *stopController) markStopped() {
	c.stoppedOnce.Do(func() { close(c.stoppedCh) })
}

func (c *stopController) requestStop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

func (c *stopController) stopAndWait() {
	c.requestStop()
	if !c.started.Load() {
		return
	}
	<-c.stoppedCh
}

func retryAutoDetection(autoDetectEvery, autoDetectTries int) bool {
	return autoDetectEvery > 0 && (autoDetectTries == infTries || autoDetectTries > 0)
}

func disableAutoDetection(autoDetectEvery *int) {
	*autoDetectEvery = 0
}

func consumeAutoDetectTry(autoDetectTries *int) {
	if *autoDetectTries != infTries {
		*autoDetectTries--
	}
}

func shouldCollectWithPenalty(clock, updateEvery, retries int) bool {
	return clock%(updateEvery+penaltyFromRetries(retries, updateEvery)) == 0
}

func penaltyFromRetries(retries, updateEvery int) int {
	v := retries / penaltyStep * penaltyStep * updateEvery / 2
	if v > maxPenalty {
		return maxPenalty
	}
	return v
}

func enqueueTickWithSkipLog(
	tick chan int,
	clock int,
	functionOnly bool,
	updateEvery int,
	retries int,
	skipTracker *tickstate.SkipTracker,
	log *logger.Logger,
) {
	select {
	case tick <- clock:
	default:
		if functionOnly || !shouldCollectWithPenalty(clock, updateEvery, retries) {
			return
		}

		skip := skipTracker.MarkSkipped()
		if skip.RunStarted.IsZero() {
			log.Infof("skipping data collection: waiting for first collection to start (interval %ds)", updateEvery)
			return
		}
		if skip.Count >= 2 {
			log.Warningf(
				"skipping data collection: previous run is still in progress for %s (skipped %d times in a row, interval %ds)",
				time.Since(skip.RunStarted),
				skip.Count,
				updateEvery,
			)
			return
		}
		log.Infof("skipping data collection: previous run is still in progress for %s (interval %ds)", time.Since(skip.RunStarted), updateEvery)
	}
}

func markRunStartWithResumeLog(skipTracker *tickstate.SkipTracker, log *logger.Logger) {
	resume := skipTracker.MarkRunStart(time.Now())
	if resume.Skipped == 0 {
		return
	}
	if resume.RunStopped.IsZero() || resume.RunStarted.IsZero() {
		log.Infof("data collection resumed (skipped %d times)", resume.Skipped)
		return
	}
	log.Infof("data collection resumed after %s (skipped %d times)", resume.RunStopped.Sub(resume.RunStarted), resume.Skipped)
}
