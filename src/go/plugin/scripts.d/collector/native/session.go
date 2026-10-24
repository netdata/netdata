// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// scriptSession owns a persistent script process and its LF-framed stdio.
// Canceling the session context or closing it terminates contained processes.
type scriptSession struct {
	ctx        context.Context
	cancel     context.CancelFunc
	process    *ndexec.Process
	stdin      io.WriteCloser
	stdout     *os.File
	frames     chan scriptFrame
	readerDone chan struct{}
	exited     chan struct{}
	waitErr    error // Published by closing exited.
}

// scriptFrame is one stdout frame, or the terminal read error.
type scriptFrame struct {
	data []byte
	err  error
}

func startSession(ctx context.Context, command []string) (*scriptSession, error) {
	ctx, cancel := context.WithCancel(ctx)
	childStdin, stdin, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	// Own the read end for the protocol reader; the process owner receives only the child end.
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		cancel()
		_ = childStdin.Close()
		_ = stdin.Close()
		return nil, err
	}
	opts := ndexec.ProcessOptions{
		Stdin:  childStdin,
		Stdout: childStdout,
	}
	process, err := ndexec.StartUnprivilegedProcess(ctx, opts, command[0], operationArgs(command, opServe)...)
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		cancel()
		_ = childStdin.Close()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = childStdout.Close()
		return nil, fmt.Errorf("start persistent command: %w", err)
	}
	_ = childStdin.Close()
	_ = childStdout.Close() // Only the child may keep stdout alive.
	s := &scriptSession{
		ctx:        ctx,
		cancel:     cancel,
		process:    process,
		stdin:      stdin,
		stdout:     stdout,
		frames:     make(chan scriptFrame),
		readerDone: make(chan struct{}),
		exited:     make(chan struct{}),
	}
	go func() {
		s.waitErr = process.Wait()
		close(s.exited)
	}()
	go s.readFrames()
	return s, nil
}

func (s *scriptSession) close() {
	s.cancel()
	_ = s.process.Close()
	_ = s.stdin.Close()
	_ = s.stdout.Close()
	<-s.readerDone
	<-s.exited
}

// transportError normalizes transport errors only, never an observed protocol
// error: cancellation can close pipes or reap a killed process before a select
// observes ctx.Done.
func (s *scriptSession) transportError(ctx context.Context, err error) error {
	if err == nil || errors.Is(err, errResponseTooLarge) {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	return err
}

func (s *scriptSession) exitError() error {
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if s.waitErr != nil {
		return fmt.Errorf("persistent command exited: %w", s.waitErr)
	}
	return errors.New("persistent command exited")
}

// exchange writes one request frame and reads its reply frame.
func (s *scriptSession) exchange(ctx context.Context, frame []byte) ([]byte, error) {
	if err := s.write(ctx, frame); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	return s.read(ctx)
}

// exchangeWithDrain lets a caller stop waiting without abandoning bytes already
// owned by the session: after caller cancellation, the exchange has up to drain
// to complete. Session shutdown still interrupts I/O immediately.
func (s *scriptSession) exchangeWithDrain(caller context.Context, frame []byte, drain time.Duration) ([]byte, error) {
	ctx, cancel := context.WithCancelCause(s.ctx)
	stop := context.AfterFunc(caller, func() {
		timer := time.NewTimer(drain)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
			cancel(context.DeadlineExceeded)
		}
	})
	defer func() { stop(); cancel(nil) }()
	data, err := s.exchange(ctx, frame)
	if errors.Is(err, context.Canceled) {
		err = context.Cause(ctx)
	}
	return data, err
}

func (s *scriptSession) read(ctx context.Context) ([]byte, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.exited:
		return nil, s.exitError()
	case frame := <-s.frames:
		return frame.data, s.transportError(ctx, frame.err)
	}
}

// write joins the writer goroutine on every path, closing stdin to unblock a
// peer that stopped reading.
func (s *scriptSession) write(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	written := make(chan error, 1)
	go func() {
		_, err := s.stdin.Write(data)
		written <- err
	}()
	select {
	case err := <-written:
		return s.transportError(ctx, err)
	case <-ctx.Done():
		_ = s.stdin.Close()
		<-written
		return ctx.Err()
	case <-s.ctx.Done():
		_ = s.stdin.Close()
		<-written
		return s.ctx.Err()
	case <-s.exited:
		_ = s.stdin.Close()
		<-written
		return s.exitError()
	}
}

func (s *scriptSession) readFrames() {
	defer close(s.readerDone)
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 4096), maxMessageBytes+1)
	scanner.Split(newFrameSplitter())
	for scanner.Scan() {
		frame := scriptFrame{
			data: bytes.Clone(scanner.Bytes()),
		}
		select {
		case s.frames <- frame:
		case <-s.ctx.Done():
			return
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	} else if errors.Is(err, bufio.ErrTooLong) {
		err = errResponseTooLarge
	}
	select {
	case s.frames <- scriptFrame{
		err: err,
	}:
	case <-s.ctx.Done():
	}
}

// newFrameSplitter returns a split function for one scanner. It requires LF,
// including after the last frame. It keeps CR in the token so every byte counts
// toward the limit (JSON whitespace is accepted by the decoder).
//
// The scanner retries with the whole pending frame after every read, so the LF
// search resumes after the bytes already checked: rescanning from the start
// makes a frame delivered in pipe-sized reads cost quadratic time.
func newFrameSplitter() bufio.SplitFunc {
	var checked int // leading bytes of the pending frame known to hold no LF
	return func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data[checked:], '\n'); i >= 0 {
			i += checked
			checked = 0
			if i+1 > maxMessageBytes {
				return 0, nil, errResponseTooLarge
			}
			return i + 1, data[:i], nil
		}
		checked = len(data)
		if len(data) >= maxMessageBytes {
			return 0, nil, errResponseTooLarge
		}
		if atEOF && len(data) != 0 {
			return 0, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil
	}
}
