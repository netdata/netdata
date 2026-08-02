// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
)

const (
	DefaultQueueCapacity      = 10000
	defaultFlushInterval      = 1 * time.Second
	maxRetentionSweepInterval = 1 * time.Hour
	minRetentionSweepInterval = 1 * time.Second
)

var errWriterStopped = errors.New("trap writer has stopped")

type Options struct {
	QueueCapacity int
	Report        output.OutcomeReporter
}

type Writer struct {
	journal    *sdkWriter
	queue      chan *model.TrapEntry
	flushCh    chan chan error
	doneCh     chan struct{}
	serializer hotSerializer

	queueMu   sync.Mutex
	started   bool
	closed    bool
	failedErr error
	failedMu  sync.Mutex
	report    output.OutcomeReporter

	flushInterval time.Duration

	retentionSweepInterval time.Duration
	lastRetentionSweep     time.Time
}

var (
	_ output.Writer             = (*Writer)(nil)
	_ output.BinaryFieldCounter = (*Writer)(nil)
)

func Prepare(dir string, cfg Config, host hostidentity.Provider, opts Options) (*Writer, error) {
	j, err := newSDKWriter(dir, cfg, host)
	if err != nil {
		return nil, err
	}
	return newWriter(j, opts), nil
}

func newWriter(j *sdkWriter, opts Options) *Writer {
	capacity := opts.QueueCapacity
	if capacity <= 0 {
		capacity = DefaultQueueCapacity
	}
	return &Writer{
		journal: j,
		queue:   make(chan *model.TrapEntry, capacity),
		// Keep unbuffered: Flush must handshake with a live worker, not queue a
		// request that Close can strand after the worker exits.
		flushCh:                make(chan chan error),
		doneCh:                 make(chan struct{}),
		flushInterval:          defaultFlushInterval,
		retentionSweepInterval: journalRetentionSweepInterval(j),
		lastRetentionSweep:     time.Now(),
		report:                 opts.Report,
	}
}

func (tw *Writer) Start() error {
	tw.queueMu.Lock()
	defer tw.queueMu.Unlock()
	if tw.closed {
		return output.ErrClosed
	}
	if tw.started {
		return nil
	}
	tw.started = true
	go tw.worker()
	return nil
}

func (tw *Writer) worker() {
	defer func() {
		if v := recover(); v != nil {
			tw.setFailure(fmt.Errorf("SNMP trap journal writer panic: %v", v))
			tw.drainAndDiscard()
		}
		close(tw.doneCh)
	}()

	ticker := time.NewTicker(tw.flushInterval)
	defer ticker.Stop()

	// Entries are written as they arrive; the durable fsync is batched onto the
	// flushInterval ticker and the explicit Flush handshake. flushPending marks
	// that at least one entry has been written but not yet synced.
	flushPending := false

	for {
		select {
		case entry, ok := <-tw.queue:
			if !ok {
				tw.drainRemaining(flushPending)
				return
			}
			if err := tw.writeOne(entry); err != nil {
				tw.setFailure(err)
				tw.drainAndDiscard()
				return
			}
			flushPending = true

		case <-ticker.C:
			if flushPending {
				if err := tw.sync(); err != nil {
					tw.setFailure(err)
					tw.drainAndDiscard()
					return
				}
				flushPending = false
			}
			if err := tw.maybeSweepRetention(time.Now()); err != nil {
				tw.setFailure(err)
				tw.drainAndDiscard()
				return
			}

		case replyCh := <-tw.flushCh:
			err := tw.drainForFlush(&flushPending)
			if err != nil {
				tw.setFailure(err)
				tw.drainAndDiscard()
				if replyCh != nil {
					replyCh <- err
				}
				return
			}
			if flushPending {
				err = tw.sync()
				flushPending = false
				if err != nil {
					tw.setFailure(err)
					tw.drainAndDiscard()
					if replyCh != nil {
						replyCh <- err
					}
					return
				}
			}
			if replyCh != nil {
				replyCh <- err
			}
		}
	}
}

func (tw *Writer) writeOne(entry *model.TrapEntry) error {
	if tw.journal == nil {
		// Test/benchmark sink mode; production Init always supplies a journal.
		return nil
	}
	payloads, binaryEncodedFields, err := tw.serializer.serialize(entry)
	if err != nil {
		return err
	}
	return tw.journal.writeRaw(payloads, binaryEncodedFields, entry.ReceivedRealtimeUsec, entry.ReceivedMonotonicUsec)
}

func (tw *Writer) sync() error {
	if tw.journal == nil {
		return nil
	}
	return tw.journal.sync()
}

func journalRetentionSweepInterval(j *sdkWriter) time.Duration {
	if j == nil {
		return 0
	}
	if j.cfg.MaxDuration <= 0 {
		if j.cfg.MaxSize == 0 {
			return 0
		}
		interval := maxRetentionSweepInterval
		if j.cfg.RotateDur > 0 && j.cfg.RotateDur < interval {
			interval = j.cfg.RotateDur
		}
		return interval
	}

	interval := min(max(j.cfg.MaxDuration/2, minRetentionSweepInterval), maxRetentionSweepInterval)
	if j.cfg.RotateDur > 0 && j.cfg.RotateDur < interval {
		interval = j.cfg.RotateDur
	}
	return interval
}

func (tw *Writer) maybeSweepRetention(now time.Time) error {
	if tw.journal == nil || tw.retentionSweepInterval <= 0 {
		return nil
	}
	if now.Sub(tw.lastRetentionSweep) < tw.retentionSweepInterval {
		return nil
	}
	tw.lastRetentionSweep = now
	return tw.journal.sweepRetention()
}

func (tw *Writer) drainForFlush(flushPending *bool) error {
	for {
		select {
		case entry, ok := <-tw.queue:
			if !ok {
				return output.ErrClosed
			}
			if err := tw.writeOne(entry); err != nil {
				return err
			}
			*flushPending = true
		default:
			return nil
		}
	}
}

func (tw *Writer) drainRemaining(pending bool) {
	for entry := range tw.queue {
		if err := tw.writeOne(entry); err != nil {
			tw.setFailure(err)
			continue
		}
		if tw.journal != nil {
			pending = true
		}
	}
	if pending && tw.journal != nil {
		if err := tw.journal.sync(); err != nil {
			tw.setFailure(err)
		}
	}
}

func (tw *Writer) drainAndDiscard() {
	// The writer has already failed. Drain entries that are immediately
	// available without blocking because Close may not have closed the queue yet.
	for {
		select {
		case _, ok := <-tw.queue:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func (tw *Writer) Write(entry *model.TrapEntry) error {
	tw.queueMu.Lock()
	defer tw.queueMu.Unlock()

	if tw.closed {
		return output.ErrClosed
	}
	if !tw.started {
		return output.ErrNotStarted
	}
	tw.failedMu.Lock()
	failErr := tw.failedErr
	tw.failedMu.Unlock()
	if failErr != nil {
		return failErr
	}

	select {
	case tw.queue <- entry:
		return nil
	default:
		return output.ErrQueueFull
	}
}

func (tw *Writer) BinaryEncodedFields() uint64 {
	if tw.journal == nil {
		return 0
	}
	return tw.journal.binaryFieldCount()
}

func (tw *Writer) Flush() error {
	tw.queueMu.Lock()
	if tw.closed {
		tw.queueMu.Unlock()
		return output.ErrClosed
	}
	if !tw.started {
		tw.queueMu.Unlock()
		return output.ErrNotStarted
	}

	tw.failedMu.Lock()
	failErr := tw.failedErr
	tw.failedMu.Unlock()
	if failErr != nil {
		tw.queueMu.Unlock()
		return failErr
	}

	replyCh := make(chan error, 1)
	select {
	case tw.flushCh <- replyCh:
		tw.queueMu.Unlock()
		return <-replyCh
	case <-tw.doneCh:
		tw.queueMu.Unlock()
		tw.failedMu.Lock()
		defer tw.failedMu.Unlock()
		if tw.failedErr != nil {
			return tw.failedErr
		}
		return errWriterStopped
	}
}

func (tw *Writer) Close() error {
	tw.queueMu.Lock()
	if tw.closed {
		tw.queueMu.Unlock()
		tw.failedMu.Lock()
		defer tw.failedMu.Unlock()
		return tw.failedErr
	}
	tw.closed = true
	started := tw.started
	if started {
		close(tw.queue)
	}
	tw.queueMu.Unlock()

	if !started {
		if tw.journal == nil {
			return nil
		}
		if err := tw.journal.close(); err != nil {
			tw.setFailure(err)
			return err
		}
		return nil
	}
	<-tw.doneCh

	tw.failedMu.Lock()
	workerErr := tw.failedErr
	tw.failedMu.Unlock()

	if tw.journal != nil {
		if err := tw.journal.close(); err != nil {
			tw.setFailure(err)
			if workerErr != nil {
				return errors.Join(workerErr, err)
			}
			return err
		}
	}

	return workerErr
}

func (tw *Writer) Directory() string {
	if tw.journal == nil {
		return ""
	}
	return tw.journal.directory()
}

func (tw *Writer) setFailure(err error) {
	var shouldReport bool
	tw.failedMu.Lock()
	if tw.failedErr == nil {
		tw.failedErr = err
		shouldReport = true
	}
	tw.failedMu.Unlock()
	if shouldReport {
		tw.report.Report(output.Outcome{
			Backend:       output.BackendJournal,
			Stage:         output.StageWorker,
			Err:           err,
			Authoritative: true,
		})
	}
}
