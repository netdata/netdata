// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package credentialfile

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

type commandFunc func(args ...string) *exec.Cmd

func helperCommand(args ...string) *exec.Cmd {
	// File mode requires caller-owned stdout and numeric stderr handling; generic
	// command runners expose stderr diagnostics and do not implement this protocol.
	return exec.Command(filepath.Join(buildinfo.NetdataBinDir, "nd-run"), append([]string{"--file-reader"}, args...)...)
}

// Read requires a regular file of at most safefile.MaxSize bytes. It returns no
// partial contents when any file or helper operation fails.
func Read(ctx context.Context, path string) ([]byte, error) {
	return read(ctx, path, true, helperCommand)
}

// ReadAll retains unbounded file/stream semantics.
func ReadAll(ctx context.Context, path string) ([]byte, error) {
	return read(ctx, path, false, helperCommand)
}

func read(ctx context.Context, path string, regular bool, command commandFunc) ([]byte, error) {
	mode, limit := "stream", "0"
	if regular {
		mode, limit = "regular", strconv.FormatInt(safefile.MaxSize, 10)
	}
	s, err := open(ctx, path, command, "read", mode, limit, path)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	var input io.Reader = s
	if regular {
		input = io.LimitReader(s, safefile.MaxSize+1)
	}
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	if regular && int64(len(data)) > safefile.MaxSize {
		return nil, transportError("invalid output size")
	}
	return data, nil
}

// Open returns an unbounded stream. File errors can surface on Read. The caller
// must Close the stream, including after parser failure or an early stop.
func Open(ctx context.Context, path string) (io.ReadCloser, error) {
	return open(ctx, path, helperCommand, "read", "stream", "0", path)
}

// Stat returns the modification time observed by the reduced-authority helper.
func Stat(ctx context.Context, path string) (time.Time, error) {
	return stat(ctx, path, helperCommand)
}

func stat(ctx context.Context, path string, command commandFunc) (time.Time, error) {
	s, err := open(ctx, path, command, "stat", path)
	if err != nil {
		return time.Time{}, err
	}
	defer s.Close()
	// A signed int64 second and nine-digit nanosecond field fit this exact bound.
	const maxStat = len("-9223372036854775808 999999999\n")
	data, err := io.ReadAll(io.LimitReader(s, int64(maxStat+1)))
	if err != nil {
		return time.Time{}, err
	}
	var seconds int64
	var nanos int32
	if _, err := fmt.Sscanf(string(data), "%d %d\n", &seconds, &nanos); err != nil ||
		nanos < 0 || nanos >= 1e9 || string(data) != fmt.Sprintf("%d %d\n", seconds, nanos) {
		return time.Time{}, transportError("invalid stat result")
	}
	return time.Unix(seconds, int64(nanos)), nil
}

// The private result has a one-digit code and a positive C int errno, at most
// INT32_MAX. Drain excess stderr without retaining or exposing diagnostics.
type resultBuffer struct {
	data     [len("NDFILE 6 2147483647\n")]byte
	n        int
	overflow bool
}

func (b *resultBuffer) Write(p []byte) (int, error) {
	n := copy(b.data[b.n:], p)
	b.n += n
	b.overflow = b.overflow || n < len(p)
	return len(p), nil
}

func (b *resultBuffer) result(path string, waitErr error) error {
	if b.overflow {
		return transportError("invalid result")
	}
	record := string(b.data[:b.n])
	var code, errno int32
	if _, err := fmt.Sscanf(record, "NDFILE %d %d\n", &code, &errno); err != nil ||
		record != fmt.Sprintf("NDFILE %d %d\n", code, errno) || code < 0 || code > 6 ||
		(code == 0 || code >= 5) && errno != 0 || code >= 1 && code <= 4 && errno <= 0 {
		return transportError("invalid result")
	}
	if code == 0 {
		if waitErr != nil {
			return transportError("unsuccessful exit")
		}
		return nil
	}
	exit, ok := waitErr.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		return transportError("result and exit disagree")
	}
	switch code {
	case 1, 2, 3, 4:
		return &fileError{[]string{"", "open", "stat", "read", "close"}[code], path, syscall.Errno(errno)}
	case 5:
		return &fileError{"read", path, safefile.ErrNotRegular}
	default:
		return &fileError{"read", path, safefile.ErrTooLarge}
	}
}

type stream struct {
	ctx      context.Context
	cmd      *exec.Cmd
	output   *os.File
	done     chan struct{}
	halted   chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex // Serializes Reads; Close must not wait for a blocked Read.
	terminal error
	result   error // Published by closing done after Wait.
}

func open(ctx context.Context, path string, command commandFunc, args ...string) (*stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.IndexByte(path, 0) >= 0 {
		return nil, &fileError{"open", path, syscall.EINVAL}
	}
	output, childOutput, err := os.Pipe()
	if err != nil {
		return nil, transportError("pipe failed")
	}
	result := &resultBuffer{}
	cmd := command(args...)
	cmd.Stdout, cmd.Stderr = childOutput, result
	err = cmd.Start()
	childOutput.Close()
	if err != nil {
		output.Close()
		return nil, startError(err)
	}
	s := &stream{
		ctx:    ctx,
		cmd:    cmd,
		output: output,
		done:   make(chan struct{}),
		halted: make(chan struct{}),
	}
	stopWatch := context.AfterFunc(ctx, s.stop)
	// Own the pipe ourselves: Cmd.Wait must not close stdout before Read drains it.
	// Exactly one goroutine reaps, even if a killed child is delayed in kernel I/O.
	go func() {
		s.result = result.result(path, cmd.Wait())
		stopWatch()
		close(s.done)
	}()
	return s, nil
}

func (s *stream) stop() {
	s.stopOnce.Do(func() {
		close(s.halted)
		s.output.Close()
		_ = s.cmd.Process.Kill()
	})
}

func (s *stream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal != nil {
		return 0, s.terminal
	}
	if len(p) == 0 {
		return 0, nil
	}
	n, err := s.output.Read(p)
	if err == nil {
		return n, nil
	}
	if err == io.EOF {
		select {
		case <-s.done:
			err = s.result
			if err == nil {
				err = io.EOF
			}
		case <-s.ctx.Done():
			err = s.ctx.Err()
		case <-s.halted:
			err = transportError("closed")
		}
	} else {
		s.stop()
		err = transportError("I/O failed")
	}
	if ctxErr := s.ctx.Err(); ctxErr != nil {
		err = ctxErr
	}
	s.terminal = err
	s.output.Close()
	return n, err
}

// Close interrupts active I/O and terminates this exact child. It does not wait
// for an uninterruptible kernel operation; the Wait goroutine remains its owner.
func (s *stream) Close() error {
	s.stop()
	return nil
}
