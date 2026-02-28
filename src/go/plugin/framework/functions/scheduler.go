// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"sync"
)

var (
	errSchedulerQueueFull = errors.New("scheduler queue is full")
	errSchedulerStopping  = errors.New("scheduler is stopping")
	errSchedulerInvalid   = errors.New("scheduler invalid request")
)

type scheduleLane struct {
	ownerUID string
	queue    []*invocationRequest
}

// keyScheduler serializes execution by schedule key while allowing concurrency
// across different keys.
type keyScheduler struct {
	mux sync.Mutex

	cond *sync.Cond

	ready []*invocationRequest
	lanes map[string]*scheduleLane

	maxPending int
	pending    int
	accepting  bool
	stopping   bool
}

func newKeyScheduler(maxPending int) *keyScheduler {
	s := &keyScheduler{
		lanes:      make(map[string]*scheduleLane),
		maxPending: maxPending,
		accepting:  true,
	}
	s.cond = sync.NewCond(&s.mux)
	return s
}

func (s *keyScheduler) enqueue(req *invocationRequest) error {
	if req == nil || req.fn == nil || req.fn.UID == "" || req.scheduleKey == "" {
		return errSchedulerInvalid
	}

	s.mux.Lock()
	defer s.mux.Unlock()

	if s.stopping || !s.accepting {
		return errSchedulerStopping
	}
	if s.maxPending > 0 && s.pending >= s.maxPending {
		return errSchedulerQueueFull
	}

	lane := s.lanes[req.scheduleKey]
	if lane == nil {
		lane = &scheduleLane{}
		s.lanes[req.scheduleKey] = lane
	}

	s.pending++
	if lane.ownerUID == "" {
		lane.ownerUID = req.fn.UID
		s.ready = append(s.ready, req)
		s.cond.Signal()
		return nil
	}

	lane.queue = append(lane.queue, req)
	return nil
}

func (s *keyScheduler) next() (*invocationRequest, bool) {
	s.mux.Lock()
	defer s.mux.Unlock()

	for len(s.ready) == 0 && !s.stopping {
		if s.drainedLocked() {
			return nil, false
		}
		s.cond.Wait()
	}

	if len(s.ready) == 0 || s.stopping {
		return nil, false
	}

	req := s.ready[0]
	s.ready = s.ready[1:]
	if s.pending > 0 {
		s.pending--
	}
	if s.drainedLocked() {
		s.cond.Broadcast()
	}
	return req, true
}

func (s *keyScheduler) cancelQueued(scheduleKey, uid string) bool {
	if scheduleKey == "" || uid == "" {
		return false
	}

	s.mux.Lock()
	defer s.mux.Unlock()

	lane := s.lanes[scheduleKey]
	if lane == nil || len(lane.queue) == 0 {
		return false
	}

	for i, req := range lane.queue {
		if req == nil || req.fn == nil || req.fn.UID != uid {
			continue
		}

		copy(lane.queue[i:], lane.queue[i+1:])
		lane.queue = lane.queue[:len(lane.queue)-1]
		if s.pending > 0 {
			s.pending--
		}

		if lane.ownerUID == "" && len(lane.queue) == 0 {
			delete(s.lanes, scheduleKey)
		}
		if s.drainedLocked() {
			s.cond.Broadcast()
		}
		return true
	}
	return false
}

func (s *keyScheduler) complete(scheduleKey, uid string) {
	if scheduleKey == "" || uid == "" {
		return
	}

	s.mux.Lock()
	defer s.mux.Unlock()

	lane := s.lanes[scheduleKey]
	if lane == nil || lane.ownerUID != uid {
		return
	}

	if s.removeReadyLocked(uid) && s.pending > 0 {
		s.pending--
	}

	if s.stopping {
		delete(s.lanes, scheduleKey)
		if s.drainedLocked() {
			s.cond.Broadcast()
		}
		return
	}

	for len(lane.queue) > 0 {
		next := lane.queue[0]
		lane.queue = lane.queue[1:]
		if next == nil || next.fn == nil {
			if s.pending > 0 {
				s.pending--
			}
			continue
		}

		lane.ownerUID = next.fn.UID
		s.ready = append(s.ready, next)
		s.cond.Signal()
		return
	}

	lane.ownerUID = ""
	if len(lane.queue) == 0 {
		delete(s.lanes, scheduleKey)
	}
	if s.drainedLocked() {
		s.cond.Broadcast()
	}
}

func (s *keyScheduler) stopAccepting() {
	s.mux.Lock()
	s.accepting = false
	if s.drainedLocked() {
		s.cond.Broadcast()
	}
	s.mux.Unlock()
}

func (s *keyScheduler) stop() {
	s.mux.Lock()
	s.stopping = true
	s.cond.Broadcast()
	s.mux.Unlock()
}

func (s *keyScheduler) drainedLocked() bool {
	return !s.accepting && s.pending == 0 && len(s.ready) == 0
}

func (s *keyScheduler) removeReadyLocked(uid string) bool {
	for i, req := range s.ready {
		if req == nil || req.fn == nil || req.fn.UID != uid {
			continue
		}

		copy(s.ready[i:], s.ready[i+1:])
		s.ready = s.ready[:len(s.ready)-1]
		return true
	}
	return false
}

func (s *keyScheduler) pendingCount() int {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.pending
}
