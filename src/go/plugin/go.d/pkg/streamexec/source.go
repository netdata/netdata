// SPDX-License-Identifier: GPL-3.0-or-later

// Package streamexec supervises a long-running sampling command and keeps its latest record.
//
// A Source starts one owned command instance, decodes its standard output line by line into records, and publishes
// each record with the time it was read. When the instance exits or stops producing records, the Source withdraws the
// latest record, terminates the instance, and starts a replacement after exponential backoff. When the context ends,
// the Source withdraws the record, terminates the instance, waits for it to exit, and returns.
//
// A command that cannot be signaled, such as one that ndsudo runs as root for an unprivileged caller, ends only by
// exiting or at its next write after the Source closes its output. The Source therefore waits at most StallTimeout for
// a terminated instance to exit and then warns; when the context has ended, it returns without the exit. Instances of
// one Source never overlap: every start, including the first one of a later Run or Background, waits for the previous
// instance to exit.
package streamexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// StartFunc starts one command instance with stdout as its standard output. It MUST start the command through an
// owned ndexec constructor given ctx or a context derived from it: the Source terminates the instance only by
// canceling ctx. It MUST hand stdout to the child before returning and MUST NOT retain or close it; the Source closes
// its copy in every case. It returns either a started Process or a non-nil error.
type StartFunc func(ctx context.Context, stdout *os.File) (*ndexec.Process, error)

// Decoder turns output lines into records. Decode receives one line without its "\n" or "\r\n" terminator; the
// slice is valid only during the call. It reports a record when the line completes one.
//
// A returned record is published to other goroutines as is: it MUST NOT share memory with line or with buffers the
// decoder reuses, and MUST NOT change afterwards. A false result skips the line without counting as a record. A
// decoder MAY accumulate lines across calls: output that never completes a record ends the instance as a stall, and
// the replacement gets a fresh decoder, so accumulation is bounded by what one instance writes within StallTimeout.
type Decoder[T any] interface {
	Decode(line []byte) (T, bool)
}

// DecoderFunc adapts a stateless function to Decoder.
type DecoderFunc[T any] func(line []byte) (T, bool)

func (f DecoderFunc[T]) Decode(line []byte) (T, bool) { return f(line) }

// Timing holds the supervision intervals.
type Timing struct {
	// MaxSampleAge is how long a record stays available from Latest.
	MaxSampleAge time.Duration
	// StallTimeout replaces an instance that produces no record for this long. It also bounds the wait for a
	// terminated instance to exit: a command that still writes does so within it.
	StallTimeout time.Duration
	// RestartDelayMin and RestartDelayMax bound the exponential restart backoff. The delay returns to the minimum
	// after an instance kept producing records for at least StallTimeout.
	RestartDelayMin time.Duration
	RestartDelayMax time.Duration
}

// Config describes one supervised command.
type Config[T any] struct {
	// Name identifies the command in errors and in the rate-limited restart warning.
	Name  string
	Start StartFunc
	// NewDecoder returns the decoder of one instance, so a replacement never inherits partial record state.
	NewDecoder func() Decoder[T]
	Timing     Timing
	// Logger receives the restart warning; nil uses a new logger, so the warning stays rate-limited.
	Logger *logger.Logger
}

// Source supervises one command. Run MAY be called again after it returns but MUST NOT run concurrently with itself;
// Latest is safe for concurrent use.
type Source[T any] struct {
	cfg Config[T]

	// latest is the newest record of the running instance; nil before its first record and while none runs. Only the
	// Run goroutine writes it, so withdrawal before a replacement starts cannot race with a late record.
	latest atomic.Pointer[record[T]]
	// last is the most recently started instance; it may still run after a bounded close. Only the Run goroutine uses
	// it, and runs of one Source are sequential.
	last *instance[T]
}

// observer receives supervision events on the Run goroutine; Run itself uses none.
type observer struct {
	// published is called after a record is published.
	published func()
	// ended is called after an instance ended with err; returning true ends supervision without a replacement, and
	// run then returns err.
	ended func(err error) bool
}

// record is a decoded value and the time its last line was read.
type record[T any] struct {
	value T
	at    time.Time
}

// New validates cfg and returns a Source that has not started.
func New[T any](cfg Config[T]) (*Source[T], error) {
	switch t := cfg.Timing; {
	case cfg.Name == "":
		return nil, errors.New("streamexec: empty command name")
	case cfg.Start == nil:
		return nil, errors.New("streamexec: nil start function")
	case cfg.NewDecoder == nil:
		return nil, errors.New("streamexec: nil decoder constructor")
	case t.MaxSampleAge <= 0, t.StallTimeout <= 0, t.RestartDelayMin <= 0, t.RestartDelayMax <= 0:
		return nil, errors.New("streamexec: timing values must be positive")
	case t.RestartDelayMin > t.RestartDelayMax:
		return nil, errors.New("streamexec: minimum restart delay exceeds the maximum")
	}
	if cfg.Logger == nil {
		cfg.Logger = logger.New()
	}
	return &Source[T]{
		cfg: cfg,
	}, nil
}

// Latest returns the newest record while its age does not exceed MaxSampleAge.
func (s *Source[T]) Latest() (T, bool) {
	r := s.latest.Load()
	if r == nil || time.Since(r.at) > s.cfg.Timing.MaxSampleAge {
		var zero T
		return zero, false
	}
	return r.value, true
}

// Run starts the command and supervises it until ctx is canceled. A failure to start the first instance is returned,
// or nil if ctx is canceled by then; started, when non-nil, is called once, after that instance starts. Later failures
// are recovered here by replacing the instance. Run returns nil after cancellation once the instance has exited, or
// after StallTimeout without the exit of an instance that cannot be signaled.
func (s *Source[T]) Run(ctx context.Context, started func()) error {
	return s.run(ctx, started, observer{})
}

// Background supervises the command on its own goroutine, for a collector without a managed runner. It returns once
// the first instance has published a record; ctx bounds only this wait. It fails without a replacement when the first
// instance cannot start, ends before publishing a record (with that instance's failure), or ctx ends first; supervision
// has then ended, and only an instance that did not exit after termination can still run. On success the caller MUST
// call stop, which ends supervision and waits for it; stop is idempotent.
func (s *Source[T]) Background(ctx context.Context) (stop func(), err error) {
	recorded := make(chan struct{})
	var recordedOnce sync.Once
	obs := observer{
		published: func() { recordedOnce.Do(func() { close(recorded) }) },
		// An instance that ends before publishing a record ends supervision with its failure.
		ended: func(error) bool {
			select {
			case <-recorded:
				return false
			default:
				return true
			}
		},
	}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.run(runCtx, nil, obs) }()
	var stopOnce sync.Once
	stop = func() {
		stopOnce.Do(func() {
			cancel()
			<-done
		})
	}

	select {
	case <-recorded:
		return stop, nil
	case err := <-done:
		// Before cancellation, run returns only a failure: the first instance did not start, or ended before
		// publishing a record.
		cancel()
		return nil, err
	case <-ctx.Done():
		stop()
		return nil, fmt.Errorf("wait for the first %s record: %w", s.cfg.Name, context.Cause(ctx))
	}
}

func (s *Source[T]) run(ctx context.Context, started func(), obs observer) error {
	if !s.awaitLast(ctx) {
		return nil
	}
	inst, err := s.start(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if started != nil {
		started()
	}

	timing := s.cfg.Timing
	delay := timing.RestartDelayMin
	for {
		healthy, err := s.follow(ctx, inst, obs)
		if obs.ended != nil && obs.ended(err) {
			return err
		}
		// An instance that kept producing records for a stall period restarts without accumulated backoff; one that
		// fails sooner keeps backing off.
		if healthy >= timing.StallTimeout {
			delay = timing.RestartDelayMin
		}
		for {
			if ctx.Err() != nil {
				return nil
			}
			s.cfg.Logger.Limit("streamexec:"+s.cfg.Name, 1, time.Minute).
				Warningf("%s source unavailable: %v", s.cfg.Name, err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
			delay = min(delay*2, timing.RestartDelayMax)
			if !s.awaitLast(ctx) {
				return nil
			}
			if inst, err = s.start(ctx); err == nil {
				break
			}
		}
	}
}

// awaitLast waits for the last instance to exit; follow leaves running one that did not exit within its bound. It
// reports false if ctx ends first.
func (s *Source[T]) awaitLast(ctx context.Context) bool {
	if s.last == nil {
		return true
	}
	select {
	case <-s.last.exited:
		return true
	case <-ctx.Done():
		return false
	}
}

// follow publishes the records of inst until it exits, stalls or ctx is canceled, then withdraws the latest record
// and closes inst, waiting at most StallTimeout for its exit. It reports how long records kept arriving: the time
// from the start of following to the last record.
func (s *Source[T]) follow(ctx context.Context, inst *instance[T], obs observer) (healthy time.Duration, err error) {
	defer func() {
		s.latest.Store(nil)
		if !inst.close(s.cfg.Timing.StallTimeout) {
			s.cfg.Logger.Limit("streamexec:"+s.cfg.Name+":exit", 1, time.Minute).
				Warningf("%s has not exited %v after termination; no replacement starts until it does",
					s.cfg.Name, s.cfg.Timing.StallTimeout)
		}
	}()

	// The end of output is not a failure of its own: the process exit reports the status, and a process that keeps
	// running without records is a stall.
	began := time.Now()
	stall := time.NewTimer(s.cfg.Timing.StallTimeout)
	defer stall.Stop()
	for {
		select {
		case <-ctx.Done():
			return healthy, ctx.Err()
		case <-inst.exited:
			return healthy, inst.exitError()
		case r := <-inst.records:
			healthy = r.at.Sub(began)
			s.latest.Store(&r)
			stall.Reset(s.cfg.Timing.StallTimeout)
			if obs.published != nil {
				obs.published()
			}
		case <-stall.C:
			return healthy, fmt.Errorf("%s stopped producing records", s.cfg.Name)
		}
	}
}
