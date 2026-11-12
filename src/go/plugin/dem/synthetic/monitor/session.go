// SPDX-License-Identifier: GPL-3.0-or-later

// Package monitor owns one native activation's browser cadence and observations.
package monitor

import (
	"context"
	"errors"
	"hash/fnv"
	"maps"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type Executor interface {
	ExecuteScheduled(
		context.Context,
		synthetic.Request,
		func(string),
		func(time.Time) synthetic.Schedule,
	) synthetic.Execution
}

// Observer callbacks are serialized and finish before Run returns. They must not
// reenter Session. Complete receives the executor-owned immutable run record.
type Observer interface {
	SetVnodeGUID(string)
	SetState(string)
	Complete(synthetic.Run)
	SetSchedule(Status)
}

type Session struct {
	mu               sync.Mutex
	request          synthetic.Request
	interval         time.Duration
	executor         Executor
	observer         Observer
	started          bool
	next             time.Time
	pending          time.Time
	queued           time.Time
	status           Status
	latest           Observation
	hasLatest        bool
	associationEpoch uint64
}

// SetVnodeGUID invalidates current observations without relabeling queued or
// active attempts. The epoch also fences A-to-B-to-A placement changes.
func (s *Session) SetVnodeGUID(guid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.request.VnodeGUID == guid {
		return
	}
	s.request.VnodeGUID = guid
	s.associationEpoch++
	s.latest, s.hasLatest = Observation{}, false
	if s.observer != nil {
		s.observer.SetVnodeGUID(guid)
	}
}

// New prepares one activation without starting work. Run schedules an immediate
// bootstrap and an activation-relative JobID phase one to two intervals later,
// then advances at the fixed interval even when admission or execution is slow.
func New(req synthetic.Request, interval time.Duration, executor Executor, observer Observer) (*Session, error) {
	if interval <= 0 {
		return nil, errors.New("monitor interval must be positive")
	}
	if executor == nil {
		return nil, errors.New("monitor executor is required")
	}
	if err := synthetic.ValidateRequest(req); err != nil {
		return nil, err
	}
	req.Secrets = maps.Clone(req.Secrets)
	return &Session{
		request:  req,
		interval: interval,
		executor: executor,
		observer: observer,
	}, nil
}

// Run executes until cancellation or an unverifiable executor result. Cancellation
// joins the outstanding executor before returning, including queued work.
func (s *Session) Run(ctx context.Context) error {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	return s.run(ctx, time.Now, timer.C, func(d time.Duration) { timer.Reset(d) })
}

// The clock inputs keep elapsed-period and cancellation tests deterministic. The
// production owner remains this one loop and one reusable timer, with no ticker
// goroutine and no allocation or iteration per missed occurrence.
func (s *Session) run(
	ctx context.Context,
	now func() time.Time,
	wake <-chan time.Time,
	reset func(time.Duration),
) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("monitor session already started")
	}
	s.started = true
	if ctx.Err() != nil {
		s.mu.Unlock()
		return nil
	}
	activation := now()
	s.next = activation.Add(s.interval).Add(phase(s.request.JobID(), s.interval))
	s.pending = activation
	s.status.Scheduled = 1
	s.status.Pending = true
	s.status.PendingDueUS = activation.UnixMicro()
	s.mu.Unlock()

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan synthetic.Execution, 1)
	inflight := false
	var attemptEpoch uint64
	stopping := false
	var stop <-chan struct{} = ctx.Done()
	for {
		s.mu.Lock()
		if ctx.Err() != nil {
			stopping, stop = true, nil
			cancel()
		}
		if stopping && !inflight {
			s.clearPending()
			s.queued = time.Time{}
			s.status.QueuedUS = 0
		}
		if !stopping {
			s.advance(now())
			if !inflight && s.status.Pending {
				s.queued = now()
				s.status.QueuedUS = s.queued.UnixMicro()
				inflight = true
				attemptEpoch = s.associationEpoch
				go func(request synthetic.Request, epoch uint64) {
					state := func(value string) { s.setState(epoch, value) }
					result <- s.executor.ExecuteScheduled(workerCtx, request, state, s.admit)
				}(s.request, attemptEpoch)
			}
			reset(s.next.Sub(now()))
		}
		s.publishSchedule()
		s.mu.Unlock()
		if stopping && !inflight {
			return nil
		}
		select {
		case <-stop:
			stopping, stop = true, nil
			cancel()
		case <-wake:
		case execution := <-result:
			inflight = false
			s.mu.Lock()
			wasAdmitted := s.status.Active
			s.status.Active = false
			s.status.ActiveDueUS, s.status.ActiveQueuedUS = 0, 0
			s.queued = time.Time{}
			s.status.QueuedUS = 0
			if !execution.Drained {
				s.clearPending()
				s.publishSchedule()
				s.mu.Unlock()
				return errors.New("monitor execution completion is unverified")
			}
			if wasAdmitted {
				if execution.Run.CompletedUS <= 0 {
					s.clearPending()
					s.publishSchedule()
					s.mu.Unlock()
					return errors.New("monitor execution has no verified completion time")
				}
				s.status.Outcomes.complete(execution.Run.Outcome)
				s.publishSchedule()
				if attemptEpoch == s.associationEpoch {
					s.latest, s.hasLatest = observe(execution.Run), true
					if s.observer != nil {
						s.observer.Complete(execution.Run)
					}
				}
			} else if ctx.Err() == nil {
				s.clearPending()
				s.publishSchedule()
				s.mu.Unlock()
				return errors.New("monitor execution ended before admission")
			}
			s.mu.Unlock()
		}
	}
}

func phase(jobID string, interval time.Duration) time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(jobID))
	return time.Duration(h.Sum64() % uint64(interval))
}

// advance accounts for any number of elapsed grid points in constant time.
// An existing queued executor keeps its turn while only its selected due changes.
func (s *Session) advance(now time.Time) {
	if now.Before(s.next) {
		return
	}
	count := uint64(now.Sub(s.next)/s.interval) + 1
	latest := s.next.Add(time.Duration(count-1) * s.interval)
	s.next = latest.Add(s.interval)
	s.status.Scheduled += count
	s.status.Coalesced += count - 1
	if s.status.Pending {
		s.status.Coalesced++
	}
	s.pending = latest
	s.status.Pending = true
	s.status.PendingDueUS = latest.UnixMicro()
}

func (s *Session) admit(now time.Time) synthetic.Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Admission can beat delivery of the timer wakeup at this exact grid point.
	s.advance(now)
	schedule := synthetic.Schedule{
		DueUS:    s.pending.UnixMicro(),
		QueuedUS: s.queued.UnixMicro(),
	}
	delay := nonnegative(now.Sub(s.pending))
	s.clearPending()
	s.status.Active = true
	s.status.ActiveDueUS = schedule.DueUS
	s.status.ActiveQueuedUS = schedule.QueuedUS
	s.status.AdmittedUS = now.UnixMicro()
	s.status.SchedulingDelay = delay
	s.status.Admitted++
	s.queued = time.Time{}
	s.status.QueuedUS = 0
	s.publishSchedule()
	return schedule
}

func (s *Session) clearPending() {
	s.pending = time.Time{}
	s.status.Pending = false
	s.status.PendingDueUS = 0
}

func (s *Session) setState(epoch uint64, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if epoch == s.associationEpoch && s.observer != nil {
		s.observer.SetState(state)
	}
}

func (s *Session) publishSchedule() {
	if s.observer != nil {
		s.observer.SetSchedule(s.status)
	}
}

func (s *Session) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{
		Status:         s.status,
		Latest:         s.latest,
		HasObservation: s.hasLatest,
	}
	if !s.queued.IsZero() {
		out.QueueAge = nonnegative(now.Sub(s.queued))
	}
	if s.hasLatest {
		out.ObservationAge = now.Sub(time.UnixMicro(s.latest.CompletedUS))
		out.Fresh = out.ObservationAge >= 0 &&
			(out.ObservationAge <= s.interval || out.ObservationAge-s.interval <= s.interval)
	}
	return out
}

func nonnegative(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}
