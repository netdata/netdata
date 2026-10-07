// SPDX-License-Identifier: GPL-3.0-or-later

// Package streamexec supervises a long-running sampling command and keeps its latest record.
//
// A Source starts one owned command instance, decodes its standard output line by line into records, and publishes
// each record with the time it was read. When the instance exits or stops producing records, the Source withdraws the
// latest record, terminates and joins the instance, and starts a replacement after exponential backoff; an instance
// never overlaps its replacement. When the context ends, the Source withdraws the record, terminates and joins the
// instance, and returns.
package streamexec

import (
	"context"
	"errors"
	"fmt"
	"os"
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
// decoder reuses, and MUST NOT change afterwards. A false result skips the line without counting as a record, so
// output that never completes a record ends the instance as a stall. A decoder that accumulates lines across calls
// MUST bound what it keeps, discarding a partial record that exceeds its bound.
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
	// StallTimeout replaces an instance that produces no record for this long.
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
// are recovered here by replacing the instance. Run returns nil after cancellation once the instance is joined.
func (s *Source[T]) Run(ctx context.Context, started func()) error {
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
		healthy, err := s.follow(ctx, inst)
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
			if inst, err = s.start(ctx); err == nil {
				break
			}
		}
	}
}

// follow publishes the records of inst until it exits, stalls or ctx is canceled, then withdraws the latest record
// and closes inst. It reports how long records kept arriving: the time from the start of following to the last
// record.
func (s *Source[T]) follow(ctx context.Context, inst *instance[T]) (healthy time.Duration, err error) {
	defer func() {
		s.latest.Store(nil)
		inst.close()
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
		case <-stall.C:
			return healthy, fmt.Errorf("%s stopped producing records", s.cfg.Name)
		}
	}
}
