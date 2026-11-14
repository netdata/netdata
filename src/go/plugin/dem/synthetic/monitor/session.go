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
		func(synthetic.JobState),
		func(time.Time) synthetic.Schedule,
	) synthetic.Execution
}

// Session is the only owner of an active job's state: schedule status, current state and the latest verified run
// of the current vnode association. Collection and Functions read it through Snapshot and Latest.
type Session struct {
	mu               sync.Mutex
	request          synthetic.Request
	jobID            string
	interval         time.Duration
	executor         Executor
	started          bool
	next             time.Time
	pending          time.Time
	queued           time.Time
	status           Status
	associationEpoch uint64
	// Current-association observation; SetVnodeGUID resets it.
	state         synthetic.JobState
	latest        synthetic.Run
	hasLatest     bool
	lastSuccessUS int64
	lastFailureUS int64
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
	s.state = synthetic.JobUnknown
	s.latest, s.hasLatest = synthetic.Run{}, false
	s.lastSuccessUS, s.lastFailureUS = 0, 0
}

// New prepares one activation without starting work. Run schedules an immediate
// bootstrap and an activation-relative JobID phase one to two intervals later,
// then advances at the fixed interval even when admission or execution is slow.
func New(req synthetic.Request, interval time.Duration, executor Executor) (*Session, error) {
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
		jobID:    req.JobID(),
		interval: interval,
		executor: executor,
		state:    synthetic.JobUnknown,
	}, nil
}

// JobID identifies the session's job; it never changes.
func (s *Session) JobID() string { return s.jobID }

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
	s.next = activation.Add(s.interval).Add(phase(s.jobID, s.interval))
	s.pending = activation
	s.status.Scheduled = 1
	s.status.Pending = true
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
					state := func(value synthetic.JobState) { s.setState(epoch, value) }
					result <- s.executor.ExecuteScheduled(workerCtx, request, state, s.admit)
				}(s.request, attemptEpoch)
			}
			reset(s.next.Sub(now()))
		}
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
			s.queued = time.Time{}
			s.status.QueuedUS = 0
			if !execution.Drained {
				s.clearPending()
				s.mu.Unlock()
				return errors.New("monitor execution completion is unverified")
			}
			if wasAdmitted {
				if execution.Run.CompletedUS <= 0 {
					s.clearPending()
					s.mu.Unlock()
					return errors.New("monitor execution has no verified completion time")
				}
				s.status.Outcomes.complete(execution.Run.Outcome)
				if attemptEpoch == s.associationEpoch {
					s.complete(execution.Run)
				}
			} else if ctx.Err() == nil {
				s.clearPending()
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
	s.status.AdmittedUS = now.UnixMicro()
	s.status.SchedulingDelay = delay
	s.status.Admitted++
	s.queued = time.Time{}
	s.status.QueuedUS = 0
	return schedule
}

func (s *Session) clearPending() {
	s.pending = time.Time{}
	s.status.Pending = false
}

// setState records the executor-reported phase of an attempt of the current association.
func (s *Session) setState(epoch uint64, state synthetic.JobState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if epoch == s.associationEpoch {
		s.state = state
	}
}

// complete records a verified completion of the current association. The copy shares no memory with the
// executor's record, and like the history copy it stores empty event and artifact lists as nil.
func (s *Session) complete(run synthetic.Run) {
	s.latest, s.hasLatest = synthetic.CloneRun(run), true
	s.state = synthetic.JobState(run.Outcome)
	switch run.Outcome {
	case synthetic.Success:
		s.lastSuccessUS = run.CompletedUS
	case synthetic.Failed, synthetic.Timeout:
		s.lastFailureUS = run.CompletedUS
	}
}

// Snapshot copies the current state and derives freshness, stale state and queue age at now. It copies scalars
// only, so per-cycle collection never copies events, evidence or captures.
func (s *Session) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{
		JobID:          s.jobID,
		Kind:           s.request.Kind,
		Name:           s.request.Name,
		VnodeGUID:      s.request.VnodeGUID,
		Interval:       s.interval,
		Status:         s.status,
		State:          s.state,
		HasObservation: s.hasLatest,
		LastSuccessUS:  s.lastSuccessUS,
		LastFailureUS:  s.lastFailureUS,
	}
	if !s.queued.IsZero() {
		out.QueueAge = nonnegative(now.Sub(s.queued))
	}
	if s.hasLatest {
		out.Latest = observe(s.latest)
		out.ObservationAge = now.Sub(time.UnixMicro(s.latest.CompletedUS))
		// Within two check intervals, written so that it cannot overflow.
		out.Fresh = out.ObservationAge >= 0 && out.ObservationAge-s.interval <= s.interval
		if !out.Fresh && out.State != synthetic.JobWaiting && out.State != synthetic.JobRunning {
			out.State = synthetic.JobStale
		}
	}
	return out
}

// Latest returns a copy of the latest verified run of the current association when its ID is runID.
func (s *Session) Latest(runID string) (synthetic.Run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasLatest || s.latest.ID != runID {
		return synthetic.Run{}, false
	}
	return synthetic.CloneRun(s.latest), true
}

func nonnegative(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}
