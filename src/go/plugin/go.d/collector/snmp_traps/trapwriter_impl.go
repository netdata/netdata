// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultQueueCapacity      = 10000
	defaultFlushEntries       = 1000
	defaultFlushInterval      = 1 * time.Second
	maxRetentionSweepInterval = 1 * time.Hour
	minRetentionSweepInterval = 1 * time.Second
)

var (
	errQueueFull     = errors.New("trap writer queue is full")
	errWriterClosed  = errors.New("trap writer is closed")
	errWriterFlushed = errors.New("trap writer has been flushed/closed")
)

type journalTrapWriter struct {
	journal    *JournalWriter
	queue      chan *TrapEntry
	flushCh    chan chan error
	doneCh     chan struct{}
	serializer journalHotSerializer

	closed    int32
	queueMu   sync.Mutex
	failedErr error
	failedMu  sync.Mutex
	onFailure func(error)

	flushInterval time.Duration
	flushEntries  int

	retentionSweepInterval time.Duration
	lastRetentionSweep     time.Time
}

func newJournalTrapWriter(j *JournalWriter, capacity int, onFailure ...func(error)) *journalTrapWriter {
	if capacity <= 0 {
		capacity = defaultQueueCapacity
	}
	tw := &journalTrapWriter{
		journal: j,
		queue:   make(chan *TrapEntry, capacity),
		// Keep unbuffered: Flush must handshake with a live worker, not queue a
		// request that Close can strand after the worker exits.
		flushCh:                make(chan chan error),
		doneCh:                 make(chan struct{}),
		flushInterval:          defaultFlushInterval,
		flushEntries:           defaultFlushEntries,
		retentionSweepInterval: journalRetentionSweepInterval(j),
		lastRetentionSweep:     time.Now(),
	}
	if len(onFailure) > 0 {
		tw.onFailure = onFailure[0]
	}
	go tw.worker()
	return tw
}

func (tw *journalTrapWriter) worker() {
	defer func() {
		if v := recover(); v != nil {
			tw.setFailure(fmt.Errorf("SNMP trap journal writer panic: %v", v))
			tw.drainAndDiscard()
		}
		close(tw.doneCh)
	}()

	ticker := time.NewTicker(tw.flushInterval)
	defer ticker.Stop()

	written := 0
	flushPending := false

	for {
		select {
		case entry, ok := <-tw.queue:
			if !ok {
				tw.drainRemaining(written)
				return
			}
			if err := tw.writeOne(entry); err != nil {
				tw.setFailure(err)
				tw.drainAndDiscard()
				return
			}
			written++
			if written >= tw.flushEntries {
				if err := tw.sync(); err != nil {
					tw.setFailure(err)
					tw.drainAndDiscard()
					return
				}
				written = 0
				flushPending = false
			} else {
				flushPending = true
			}

		case <-ticker.C:
			if flushPending {
				if err := tw.sync(); err != nil {
					tw.setFailure(err)
					tw.drainAndDiscard()
					return
				}
				written = 0
				flushPending = false
			}
			if err := tw.maybeSweepRetention(time.Now()); err != nil {
				tw.setFailure(err)
				tw.drainAndDiscard()
				return
			}

		case replyCh := <-tw.flushCh:
			err := tw.drainForFlush(&written, &flushPending)
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
				written = 0
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

func (tw *journalTrapWriter) writeOne(entry *TrapEntry) error {
	if tw.journal == nil {
		// Test/benchmark sink mode; production Init always supplies a journal.
		return nil
	}
	payloads, binaryEncodedFields, err := tw.serializer.serialize(entry)
	if err != nil {
		return err
	}
	return tw.journal.WriteRawEntry(payloads, binaryEncodedFields, entry.ReceivedRealtimeUsec, entry.ReceivedMonotonicUsec)
}

func (tw *journalTrapWriter) sync() error {
	if tw.journal == nil {
		return nil
	}
	return tw.journal.Sync()
}

func journalRetentionSweepInterval(j *JournalWriter) time.Duration {
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

func (tw *journalTrapWriter) maybeSweepRetention(now time.Time) error {
	if tw.journal == nil || tw.retentionSweepInterval <= 0 {
		return nil
	}
	if now.Sub(tw.lastRetentionSweep) < tw.retentionSweepInterval {
		return nil
	}
	tw.lastRetentionSweep = now
	return tw.journal.SweepRetention()
}

func (tw *journalTrapWriter) drainForFlush(written *int, flushPending *bool) error {
	for {
		select {
		case entry, ok := <-tw.queue:
			if !ok {
				return errWriterClosed
			}
			if err := tw.writeOne(entry); err != nil {
				return err
			}
			*written = *written + 1
			if *written >= tw.flushEntries {
				if err := tw.sync(); err != nil {
					return err
				}
				*written = 0
				*flushPending = false
			} else {
				*flushPending = true
			}
		default:
			return nil
		}
	}
}

func (tw *journalTrapWriter) drainRemaining(written int) {
	for entry := range tw.queue {
		if err := tw.writeOne(entry); err != nil {
			tw.setFailure(err)
			continue
		}
		if tw.journal != nil {
			written++
		}
	}
	if written > 0 && tw.journal != nil {
		if err := tw.journal.Sync(); err != nil {
			tw.setFailure(err)
		}
	}
}

func (tw *journalTrapWriter) drainAndDiscard() {
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

func (tw *journalTrapWriter) Write(entry *TrapEntry) error {
	tw.queueMu.Lock()
	defer tw.queueMu.Unlock()

	if atomic.LoadInt32(&tw.closed) != 0 {
		return errWriterClosed
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
		return errQueueFull
	}
}

func (tw *journalTrapWriter) BinaryEncodedFields() uint64 {
	if tw.journal == nil {
		return 0
	}
	return tw.journal.BinaryEncodedFields()
}

func (tw *journalTrapWriter) Flush() error {
	if atomic.LoadInt32(&tw.closed) != 0 {
		return errWriterClosed
	}

	tw.failedMu.Lock()
	failErr := tw.failedErr
	tw.failedMu.Unlock()
	if failErr != nil {
		return failErr
	}

	replyCh := make(chan error, 1)
	select {
	case tw.flushCh <- replyCh:
		return <-replyCh
	case <-tw.doneCh:
		tw.failedMu.Lock()
		defer tw.failedMu.Unlock()
		if tw.failedErr != nil {
			return tw.failedErr
		}
		return errWriterFlushed
	}
}

func (tw *journalTrapWriter) Close() error {
	if !atomic.CompareAndSwapInt32(&tw.closed, 0, 1) {
		tw.failedMu.Lock()
		defer tw.failedMu.Unlock()
		return tw.failedErr
	}

	tw.queueMu.Lock()
	close(tw.queue)
	tw.queueMu.Unlock()
	<-tw.doneCh

	tw.failedMu.Lock()
	workerErr := tw.failedErr
	tw.failedMu.Unlock()

	if tw.journal != nil {
		if err := tw.journal.Close(); err != nil {
			tw.setFailure(err)
			if workerErr != nil {
				return errors.Join(workerErr, err)
			}
			return err
		}
	}

	return workerErr
}

func (tw *journalTrapWriter) setFailure(err error) {
	var shouldLog bool
	tw.failedMu.Lock()
	if tw.failedErr == nil {
		tw.failedErr = err
		shouldLog = true
	}
	tw.failedMu.Unlock()
	if shouldLog && tw.onFailure != nil {
		tw.onFailure(err)
	}
}
