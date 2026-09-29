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
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

var _ collectorapi.CollectorV2Runner = (*Collector)(nil)

type scriptRuntime struct {
	requests chan scriptRequest
	done     chan struct{}
}

type scriptRequest struct {
	function *funcapi.RawMethodRequest
	ctx      context.Context
	reply    chan scriptResult
}

type scriptResult struct {
	function *funcapi.FunctionResponse
	response response
	err      error
}

func (c *Collector) runPersistent(ctx context.Context, ready func()) error {
	s, err := startSession(ctx, c.definition.Command)
	if err != nil {
		return err
	}
	defer s.close()
	startupCtx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	if len(c.configInput) > 0 {
		if err := s.write(startupCtx, c.configInput); err != nil {
			cancel()
			return fmt.Errorf("persistent configuration: %w", err)
		}
	}
	frame, err := s.read(startupCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("persistent startup: %w", err)
	}
	if err := decodeReady(frame); err != nil {
		return fmt.Errorf("persistent startup: %w", err)
	}

	r := &scriptRuntime{
		requests: make(chan scriptRequest),
		done:     make(chan struct{}),
	}
	c.runtimeMu.Lock()
	c.runtime = r
	c.runtimeMu.Unlock()
	defer func() {
		c.runtimeMu.Lock()
		c.runtime = nil
		c.runtimeMu.Unlock()
		close(r.done)
	}()
	ready()

	var sequence uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.exited:
			return s.exitError()
		case frame := <-s.frames:
			if frame.err != nil {
				return s.transportError(ctx, frame.err)
			}
			return fmt.Errorf("unsolicited persistent response")
		case request := <-r.requests:
			// A canceled sender can race with channel delivery. Until this point the
			// request owns no stream bytes, so reject it without disturbing the peer.
			if err := request.ctx.Err(); err != nil {
				request.reply <- scriptResult{
					err: err,
				}
				continue
			}
			sequence++
			id := strconv.FormatUint(sequence, 10)
			var result scriptResult
			if request.function == nil {
				// Queue waiting must not consume the admitted exchange budget.
				exchangeCtx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
				data, err := s.exchange(exchangeCtx, id)
				cancel()
				if err == nil {
					result.response, err = c.definition.decodeReply(data, id)
				}
				result.err = err
			} else {
				frame, err := encodeFunctionRequest(*request.function, id, request.ctx)
				if err != nil {
					request.reply <- scriptResult{
						err: err,
					}
					continue // A local encoding failure has not touched the stream.
				}
				data, err := s.exchangeFunction(request.ctx, frame, c.Timeout.Duration())
				if err == nil {
					result.function, err = decodeFunctionReply(data, id, request.function.Info)
				}
				result.err = err
			}
			// A buffered reply lets Run finish teardown after the caller stops waiting.
			request.reply <- result
			if result.err != nil && !errors.Is(result.err, errCollectionFailed) {
				return fmt.Errorf("persistent exchange: %w", result.err)
			}
		}
	}
}

func (c *Collector) collectPersistent(ctx context.Context) (response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	result := c.requestPersistent(ctx, nil)
	return result.response, result.err
}

func (c *Collector) requestPersistent(ctx context.Context, function *funcapi.RawMethodRequest) scriptResult {
	if err := ctx.Err(); err != nil {
		return scriptResult{
			err: err,
		}
	}
	c.runtimeMu.Lock()
	r := c.runtime
	c.runtimeMu.Unlock()
	if r == nil {
		return scriptResult{
			err: fmt.Errorf("persistent script is not running"),
		}
	}
	request := scriptRequest{
		ctx:      ctx,
		function: function,
		reply:    make(chan scriptResult, 1),
	}
	select {
	case <-ctx.Done():
		return scriptResult{
			err: ctx.Err(),
		}
	case <-r.done:
		return scriptResult{
			err: fmt.Errorf("persistent script stopped"),
		}
	case r.requests <- request:
	}
	select {
	case <-ctx.Done():
		return scriptResult{
			err: ctx.Err(),
		}
	case result := <-request.reply:
		return result
	case <-r.done:
		return scriptResult{
			err: fmt.Errorf("persistent script stopped"),
		}
	}
}

type scriptFrame struct {
	data []byte
	err  error
}

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

func startSession(ctx context.Context, argv []string) (*scriptSession, error) {
	ctx, cancel := context.WithCancel(ctx)
	args := append(append([]string(nil), argv[1:]...), "serve")
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
	process, err := ndexec.StartUnprivilegedProcess(ctx, opts, argv[0], args...)
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

// Cancellation can close pipes or reap a killed process before the select sees
// ctx.Done. Normalize only transport errors, never an observed protocol error.
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
	return fmt.Errorf("persistent command exited")
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

func (s *scriptSession) exchange(ctx context.Context, id string) ([]byte, error) {
	if err := s.write(ctx, []byte(fmt.Sprintf("{\"id\":\"%s\",\"method\":\"collect\"}\n", id))); err != nil {
		return nil, fmt.Errorf("write collection request: %w", err)
	}
	return s.read(ctx)
}

// A Function caller can stop waiting without abandoning bytes already owned by
// the session. Allow one timeout to complete the exchange after cancellation;
// job shutdown still interrupts I/O immediately through the session context.
func (s *scriptSession) exchangeFunction(caller context.Context, frame []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithCancelCause(s.ctx)
	stop := context.AfterFunc(caller, func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
			cancel(context.DeadlineExceeded)
		}
	})
	defer func() { stop(); cancel(nil) }()
	err := s.write(ctx, frame)
	var data []byte
	if err == nil {
		data, err = s.read(ctx)
	}
	if errors.Is(err, context.Canceled) {
		err = context.Cause(ctx)
	}
	return data, err
}

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
		if err != nil {
			return s.transportError(ctx, err)
		}
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
	return nil
}

func (s *scriptSession) readFrames() {
	defer close(s.readerDone)
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 4096), maxResponseBytes+1)
	scanner.Split(splitFrame)
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

// LF is mandatory, including for the last frame. Keep CR in the token so every
// byte counts toward the limit (JSON whitespace is accepted by the decoder).
func splitFrame(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		if i+1 > maxResponseBytes {
			return 0, nil, errResponseTooLarge
		}
		return i + 1, data[:i], nil
	}
	if len(data) >= maxResponseBytes {
		return 0, nil, errResponseTooLarge
	}
	if atEOF && len(data) != 0 {
		return 0, nil, io.ErrUnexpectedEOF
	}
	return 0, nil, nil
}
