// SPDX-License-Identifier: GPL-3.0-or-later

package streamexec

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// maxLineSize bounds the memory one output line can hold, terminator included, using bufio.Scanner's default token
// limit; a longer line is discarded whole instead of ending the output as a Scanner would.
const maxLineSize = bufio.MaxScanTokenSize

// instance is one owned command process. While its output lasts, it sends each decoded record on records.
type instance[T any] struct {
	name   string
	stdout *os.File
	cancel context.CancelFunc

	records  chan record[T]
	readDone chan struct{}
	exited   chan struct{}
	exitErr  error // valid after exited is closed
}

// start starts one instance. Canceling ctx terminates its process tree; the caller MUST close the result.
func (s *Source[T]) start(ctx context.Context) (*instance[T], error) {
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("open %s output pipe: %w", s.cfg.Name, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	process, err := s.cfg.Start(ctx, childStdout)
	// Close our copy of the write end: only the child's process tree keeps the pipe open.
	_ = childStdout.Close()
	if err == nil && process == nil {
		err = errors.New("start function returned no process")
	}
	if err != nil {
		cancel()
		_ = stdout.Close()
		return nil, fmt.Errorf("start %s: %w", s.cfg.Name, err)
	}

	inst := &instance[T]{
		name:     s.cfg.Name,
		stdout:   stdout,
		cancel:   cancel,
		records:  make(chan record[T]),
		readDone: make(chan struct{}),
		exited:   make(chan struct{}),
	}
	s.last = inst
	go inst.read(ctx, s.cfg.NewDecoder())
	go func() {
		defer close(inst.exited)
		inst.exitErr = process.Wait()
	}()
	return inst, nil
}

// close terminates the owned process tree, joins the reader, and waits at most wait for the process to exit. It
// reports whether the process exited. Canceling the instance context makes ndexec terminate the tree; closing the read
// end also makes a writer that cannot be signaled die of SIGPIPE at its next write, unless it handles SIGPIPE or EPIPE
// itself. Such a writer that no longer writes keeps running, which is why the wait is bounded.
func (inst *instance[T]) close(wait time.Duration) bool {
	inst.cancel()
	_ = inst.stdout.Close()
	<-inst.readDone
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-inst.exited:
		return true
	case <-timer.C:
		return false
	}
}

// exitError describes the process exit; valid only after exited is closed.
func (inst *instance[T]) exitError() error {
	if inst.exitErr != nil {
		return fmt.Errorf("%s exited: %w", inst.name, inst.exitErr)
	}
	return fmt.Errorf("%s exited", inst.name)
}

// read decodes the output until it ends or ctx is canceled. An unterminated final fragment is not a line.
func (inst *instance[T]) read(ctx context.Context, dec Decoder[T]) {
	defer close(inst.readDone)
	r := bufio.NewReaderSize(inst.stdout, maxLineSize)
	discarding := false
	for {
		line, err := r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			discarding = true
			continue
		}
		if err != nil {
			return
		}
		if discarding {
			// The terminator of an over-long line.
			discarding = false
			continue
		}
		v, ok := dec.Decode(trimTerminator(line))
		if !ok {
			continue
		}
		select {
		case inst.records <- record[T]{
			value: v,
			at:    time.Now(),
		}:
		case <-ctx.Done():
			return
		}
	}
}

func trimTerminator(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r"))
}
