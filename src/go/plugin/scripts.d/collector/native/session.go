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
	"os/exec"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

var _ collectorapi.CollectorV2Runner = (*Collector)(nil)

type scriptRuntime struct {
	requests chan collectionRequest
	done     chan struct{}
}

type collectionRequest struct {
	ctx   context.Context
	reply chan collectionResult
}

type collectionResult struct {
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
	frame, err := s.read(startupCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("persistent startup: %w", err)
	}
	if err := decodeReady(frame); err != nil {
		return fmt.Errorf("persistent startup: %w", err)
	}

	r := &scriptRuntime{
		requests: make(chan collectionRequest),
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
				return frame.err
			}
			return fmt.Errorf("unsolicited persistent response")
		case request := <-r.requests:
			sequence++
			requestCtx, cancel := context.WithTimeout(request.ctx, c.Timeout.Duration())
			data, err := s.exchange(requestCtx, strconv.FormatUint(sequence, 10))
			cancel()
			var result response
			if err == nil {
				result, err = c.definition.decodeReply(data, strconv.FormatUint(sequence, 10))
			}
			// A buffered reply lets Run finish teardown even if Collect was canceled.
			request.reply <- collectionResult{
				response: result,
				err:      err,
			}
			if err != nil && !errors.Is(err, errCollectionFailed) {
				return fmt.Errorf("persistent collection: %w", err)
			}
		}
	}
}

func (c *Collector) collectPersistent(ctx context.Context) (response, error) {
	c.runtimeMu.Lock()
	r := c.runtime
	c.runtimeMu.Unlock()
	if r == nil {
		return response{}, fmt.Errorf("persistent script is not running")
	}
	request := collectionRequest{
		ctx:   ctx,
		reply: make(chan collectionResult, 1),
	}
	select {
	case <-ctx.Done():
		return response{}, ctx.Err()
	case <-r.done:
		return response{}, fmt.Errorf("persistent script stopped")
	case r.requests <- request:
	}
	select {
	case <-ctx.Done():
		return response{}, ctx.Err()
	case result := <-request.reply:
		return result.response, result.err
	case <-r.done:
		return response{}, fmt.Errorf("persistent script stopped")
	}
}

type scriptFrame struct {
	data []byte
	err  error
}

type scriptSession struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cmd        *exec.Cmd
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
	cmd := ndexec.UnprivilegedCommandContext(ctx, argv[0], args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	// Own the read end: Cmd.Wait must not close it while the reader is using it.
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = childStdout, io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = childStdout.Close()
		return nil, fmt.Errorf("start persistent command: %w", err)
	}
	_ = childStdout.Close() // Only the child may keep stdout alive.
	s := &scriptSession{
		ctx:        ctx,
		cancel:     cancel,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		frames:     make(chan scriptFrame),
		readerDone: make(chan struct{}),
		exited:     make(chan struct{}),
	}
	go func() {
		s.waitErr = cmd.Wait()
		close(s.exited)
	}()
	go s.readFrames()
	return s, nil
}

func (s *scriptSession) close() {
	s.cancel()
	// Also cancel after Wait: an exited parent may have left descendants holding
	// its pipes. ndexec's callback targets only this command's process group.
	if s.cmd.Cancel != nil {
		_ = s.cmd.Cancel()
	} else {
		_ = s.cmd.Process.Kill()
	}
	_ = s.stdin.Close()
	_ = s.stdout.Close()
	<-s.readerDone
	<-s.exited
}

func (s *scriptSession) exitError() error {
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
		return frame.data, frame.err
	}
}

func (s *scriptSession) exchange(ctx context.Context, id string) ([]byte, error) {
	written := make(chan error, 1)
	go func() {
		_, err := fmt.Fprintf(s.stdin, "{\"id\":\"%s\",\"method\":\"collect\"}\n", id)
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			return nil, fmt.Errorf("write collection request: %w", err)
		}
	case <-ctx.Done():
		_ = s.stdin.Close()
		<-written
		return nil, ctx.Err()
	case <-s.ctx.Done():
		_ = s.stdin.Close()
		<-written
		return nil, s.ctx.Err()
	case <-s.exited:
		_ = s.stdin.Close()
		<-written
		return nil, s.exitError()
	}
	return s.read(ctx)
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
