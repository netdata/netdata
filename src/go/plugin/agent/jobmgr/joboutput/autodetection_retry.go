// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type autoDetectionRetryPlanner func(
	confgroup.Config,
	autoDetectionRetryToken,
) (jobmgr.WorkPlan, error)

type autoDetectionRetryToken struct {
	uid        string
	generation uint64
}

type autoDetectionRetryIndex struct {
	mu sync.Mutex

	entries          map[string]*autoDetectionRetry
	queue            autoDetectionRetryHeap
	commands         jobmgr.PreparedCommandPort
	plan             autoDetectionRetryPlanner
	failure          func(error)
	run              uint64
	logicalClock     int64
	lastProcessClock int
	clockInitialized bool
	generation       uint64
	bound            bool
	closed           bool
	failed           bool
	terminalErr      error
	wake             chan struct{}
	stop             chan struct{}
	done             chan struct{}
	failOnce         sync.Once
}

type autoDetectionRetry struct {
	config confgroup.Config
	token  autoDetectionRetryToken
	due    int64
	index  int
}

type autoDetectionRetryHeap []*autoDetectionRetry

func newAutoDetectionRetryIndex() *autoDetectionRetryIndex {
	return &autoDetectionRetryIndex{
		entries: make(map[string]*autoDetectionRetry),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (adri *autoDetectionRetryIndex) bind(
	commands jobmgr.PreparedCommandPort,
	plan autoDetectionRetryPlanner,
	run uint64,
	failure func(error),
) error {
	if adri == nil || commands == nil || plan == nil || run == 0 ||
		failure == nil {
		return errors.New(
			"job output: invalid autodetection retry binding",
		)
	}
	adri.mu.Lock()
	if adri.closed ||
		adri.bound {
		adri.mu.Unlock()
		return errors.New(
			"job output: autodetection retries already bound",
		)
	}
	adri.commands = commands
	adri.plan = plan
	adri.failure = failure
	adri.run = run
	adri.bound = true
	adri.mu.Unlock()
	go adri.runWorker()
	return nil
}

func (adri *autoDetectionRetryIndex) schedule(
	config confgroup.Config,
	after int,
) {
	if adri == nil ||
		config == nil ||
		config.FullName() == "" ||
		config.UID() == "" ||
		after <= 0 {
		return
	}
	cloned, err := config.Clone()
	if err != nil {
		return
	}
	adri.mu.Lock()
	if adri.closed || adri.failed {
		adri.mu.Unlock()
		return
	}
	if current := adri.entries[config.FullName()]; current != nil {
		if current.index >= 0 {
			heap.Remove(&adri.queue, current.index)
		}
		delete(adri.entries, config.FullName())
	}
	adri.generation++
	if adri.generation == 0 {
		adri.failed = true
		adri.mu.Unlock()
		adri.fail(errors.New(
			"job output: autodetection retry generation wrapped",
		))
		return
	}
	retry := &autoDetectionRetry{
		config: cloned,
		token: autoDetectionRetryToken{
			uid:        cloned.UID(),
			generation: adri.generation,
		},
		due:   adri.logicalClock + int64(after),
		index: -1,
	}
	adri.entries[cloned.FullName()] = retry
	heap.Push(&adri.queue, retry)
	due := retry.due <= adri.logicalClock
	adri.mu.Unlock()
	if due {
		adri.notify()
	}
}

func (adri *autoDetectionRetryIndex) cancel(id string) {
	if adri == nil || id == "" {
		return
	}
	adri.mu.Lock()
	defer adri.mu.Unlock()
	retry := adri.entries[id]
	if retry == nil {
		return
	}
	if retry.index >= 0 {
		heap.Remove(&adri.queue, retry.index)
	}
	delete(adri.entries, id)
}

func (adri *autoDetectionRetryIndex) cancelToken(
	id string,
	token autoDetectionRetryToken,
) {
	if adri == nil || id == "" || token.generation == 0 {
		return
	}
	adri.mu.Lock()
	defer adri.mu.Unlock()
	retry := adri.entries[id]
	if retry == nil || retry.token != token {
		return
	}
	if retry.index >= 0 {
		heap.Remove(&adri.queue, retry.index)
	}
	delete(adri.entries, id)
}

func (adri *autoDetectionRetryIndex) isCurrent(
	id string,
	token autoDetectionRetryToken,
) bool {
	if adri == nil || id == "" || token.generation == 0 {
		return false
	}
	adri.mu.Lock()
	defer adri.mu.Unlock()
	retry := adri.entries[id]
	return retry != nil && retry.token == token
}

func (adri *autoDetectionRetryIndex) stopWorker() {
	if adri == nil {
		return
	}
	adri.mu.Lock()
	if adri.closed {
		adri.mu.Unlock()
		return
	}
	adri.closed = true
	adri.entries = make(map[string]*autoDetectionRetry)
	adri.queue = nil
	close(adri.stop)
	adri.mu.Unlock()
	adri.notify()
}

func (adri *autoDetectionRetryIndex) wait(
	ctx context.Context,
) error {
	if adri == nil || ctx == nil {
		return errors.New(
			"job output: invalid autodetection retry wait",
		)
	}
	adri.mu.Lock()
	bound := adri.bound
	done := adri.done
	adri.mu.Unlock()
	if !bound {
		return nil
	}
	select {
	case <-done:
		return adri.terminalError()
	default:
	}
	select {
	case <-done:
		return adri.terminalError()
	case <-ctx.Done():
		select {
		case <-done:
			return adri.terminalError()
		default:
			return errors.Join(ctx.Err(), adri.terminalError())
		}
	}
}

func (adri *autoDetectionRetryIndex) joined() bool {
	if adri == nil {
		return true
	}
	adri.mu.Lock()
	bound := adri.bound
	done := adri.done
	adri.mu.Unlock()
	if !bound {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (adri *autoDetectionRetryIndex) terminalError() error {
	adri.mu.Lock()
	defer adri.mu.Unlock()
	return adri.terminalErr
}

func (adri *autoDetectionRetryIndex) advance(processClock int) {
	if adri == nil {
		return
	}
	adri.mu.Lock()
	if adri.closed || adri.failed {
		adri.mu.Unlock()
		return
	}
	if !adri.clockInitialized {
		adri.lastProcessClock = processClock
		adri.clockInitialized = true
		adri.mu.Unlock()
		return
	}
	if processClock < adri.lastProcessClock {
		adri.failed = true
		adri.mu.Unlock()
		adri.fail(errors.New(
			"job output: autodetection retry process clock regressed",
		))
		return
	}
	adri.logicalClock += int64(processClock - adri.lastProcessClock)
	adri.lastProcessClock = processClock
	due := len(adri.queue) > 0 &&
		adri.queue[0].due <= adri.logicalClock
	adri.mu.Unlock()
	if due {
		adri.notify()
	}
}

func (adri *autoDetectionRetryIndex) runWorker() {
	defer close(adri.done)
	for {
		select {
		case <-adri.stop:
			return
		case <-adri.wake:
		}
		for {
			retry := adri.takeDue()
			if retry == nil {
				break
			}
			if err := adri.dispatch(retry); err != nil {
				stopping, clean := err.(*lifecycle.StoppingRejection)
				if clean &&
					stopping.Generation == adri.run {
					return
				}
				adri.fail(err)
				return
			}
		}
	}
}

func (adri *autoDetectionRetryIndex) takeDue() *autoDetectionRetry {
	adri.mu.Lock()
	defer adri.mu.Unlock()
	if adri.closed || adri.failed ||
		len(adri.queue) == 0 ||
		adri.queue[0].due > adri.logicalClock {
		return nil
	}
	return heap.Pop(&adri.queue).(*autoDetectionRetry)
}

func (adri *autoDetectionRetryIndex) dispatch(
	retry *autoDetectionRetry,
) error {
	work, err := adri.plan(retry.config, retry.token)
	if err != nil {
		return err
	}
	return adri.commands.SubmitPrepared(
		context.Background(),
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-autodetection-retry-%d-%d",
				adri.run,
				retry.token.generation,
			),
			LaneKey: retry.config.FullName(),
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/jobs/autodetection-retry",
		},
		work,
	)
}

func (adri *autoDetectionRetryIndex) notify() {
	select {
	case adri.wake <- struct{}{}:
	default:
	}
}

func (adri *autoDetectionRetryIndex) fail(err error) {
	if err == nil {
		return
	}
	adri.failOnce.Do(func() {
		adri.mu.Lock()
		adri.failed = true
		adri.terminalErr = errors.Join(adri.terminalErr, err)
		failure := adri.failure
		adri.mu.Unlock()
		if failure != nil {
			failure(err)
		}
	})
}

func (h autoDetectionRetryHeap) Len() int {
	return len(h)
}

func (h autoDetectionRetryHeap) Less(left, right int) bool {
	if h[left].due == h[right].due {
		return h[left].token.generation <
			h[right].token.generation
	}
	return h[left].due < h[right].due
}

func (h autoDetectionRetryHeap) Swap(left, right int) {
	h[left], h[right] = h[right], h[left]
	h[left].index = left
	h[right].index = right
}

func (h *autoDetectionRetryHeap) Push(value any) {
	retry := value.(*autoDetectionRetry)
	retry.index = len(*h)
	*h = append(*h, retry)
}

func (h *autoDetectionRetryHeap) Pop() any {
	old := *h
	last := len(old) - 1
	retry := old[last]
	old[last] = nil
	retry.index = -1
	*h = old[:last]
	return retry
}
