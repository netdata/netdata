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

	entries    map[string]*autoDetectionRetry
	queue      autoDetectionRetryHeap
	commands   jobmgr.PreparedCommandPort
	plan       autoDetectionRetryPlanner
	run        uint64
	clock      int
	generation uint64
	closed     bool
}

type autoDetectionRetry struct {
	config confgroup.Config
	token  autoDetectionRetryToken
	due    int
	index  int
}

type autoDetectionRetryHeap []*autoDetectionRetry

func newAutoDetectionRetryIndex() *autoDetectionRetryIndex {
	return &autoDetectionRetryIndex{
		entries: make(map[string]*autoDetectionRetry),
	}
}

func (adri *autoDetectionRetryIndex) bind(
	commands jobmgr.PreparedCommandPort,
	plan autoDetectionRetryPlanner,
	run uint64,
) error {
	if adri == nil || commands == nil || plan == nil || run == 0 {
		return errors.New(
			"job output: invalid autodetection retry binding",
		)
	}
	adri.mu.Lock()
	defer adri.mu.Unlock()
	if adri.closed ||
		adri.commands != nil ||
		adri.plan != nil ||
		adri.run != 0 {
		return errors.New(
			"job output: autodetection retries already bound",
		)
	}
	adri.commands = commands
	adri.plan = plan
	adri.run = run
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
	defer adri.mu.Unlock()
	if adri.closed {
		return
	}
	if current := adri.entries[config.FullName()]; current != nil {
		heap.Remove(&adri.queue, current.index)
		delete(adri.entries, config.FullName())
	}
	adri.generation++
	if adri.generation == 0 {
		adri.closed = true
		adri.entries = make(map[string]*autoDetectionRetry)
		adri.queue = nil
		return
	}
	retry := &autoDetectionRetry{
		config: cloned,
		token: autoDetectionRetryToken{
			uid:        cloned.UID(),
			generation: adri.generation,
		},
		due:   adri.clock + after,
		index: -1,
	}
	adri.entries[cloned.FullName()] = retry
	heap.Push(&adri.queue, retry)
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
	heap.Remove(&adri.queue, retry.index)
	delete(adri.entries, id)
}

func (adri *autoDetectionRetryIndex) close() {
	if adri == nil {
		return
	}
	adri.mu.Lock()
	adri.closed = true
	adri.entries = make(map[string]*autoDetectionRetry)
	adri.queue = nil
	adri.mu.Unlock()
}

func (adri *autoDetectionRetryIndex) dispatchDue(
	ctx context.Context,
	clock int,
) error {
	if adri == nil || ctx == nil {
		return errors.New(
			"job output: invalid autodetection retry dispatch",
		)
	}
	adri.mu.Lock()
	adri.clock = clock
	if adri.closed {
		adri.mu.Unlock()
		return nil
	}
	commands := adri.commands
	plan := adri.plan
	run := adri.run
	var due []*autoDetectionRetry
	for len(adri.queue) > 0 &&
		adri.queue[0].due <= clock {
		retry := heap.Pop(&adri.queue).(*autoDetectionRetry)
		delete(adri.entries, retry.config.FullName())
		due = append(due, retry)
	}
	adri.mu.Unlock()

	if len(due) == 0 {
		return nil
	}
	if commands == nil || plan == nil || run == 0 {
		return errors.New(
			"job output: due autodetection retry is unbound",
		)
	}
	var result error
	for _, retry := range due {
		work, err := plan(retry.config, retry.token)
		if err == nil {
			err = commands.SubmitPrepared(
				ctx,
				jobmgr.Request{
					UID: fmt.Sprintf(
						"jobmgr-autodetection-retry-%d-%d",
						run,
						retry.token.generation,
					),
					LaneKey: retry.config.FullName(),
					Source:  lifecycle.SourceJobManager,
					Route: "internal/jobs/" +
						"autodetection-retry",
				},
				work,
			)
		}
		result = errors.Join(result, err)
	}
	return result
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
