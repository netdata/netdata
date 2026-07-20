package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	stderrLimit       = 64 * 1024
	readBufferBytes   = 32 * 1024
	processJoinPeriod = 5 * time.Second
)

var childEnvironment = [...]string{
	"PATH=/usr/bin:/bin",
	"LANG=C",
	"LC_ALL=C",
	"TZ=UTC",
}

type OutputObserver func(chunk []byte) error

type Spec struct {
	Executable string
	Arguments  []string
	Directory  string
	ObserveOut OutputObserver
}

type Result struct {
	Stderr          []byte
	StderrTruncated bool
}

type Process struct {
	command *exec.Cmd
	stdin   io.WriteCloser

	writeMu sync.Mutex

	closeOnce sync.Once
	closeErr  error

	killOnce sync.Once
	killErr  error

	done   chan struct{}
	result waitResult
}

type waitResult struct {
	result Result
	err    error
}

func Start(spec Spec) (*Process, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	command := exec.Command(spec.Executable, spec.Arguments...)
	command.Dir = spec.Directory
	command.Env = append([]string(nil), childEnvironment[:]...)
	configureContainment(command)

	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("jobmgr root runner: create stdin pipe: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("jobmgr root runner: create stdout pipe: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("jobmgr root runner: create stderr pipe: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("jobmgr root runner: start: %w", err)
	}

	process := &Process{
		command: command,
		stdin:   stdin,
		done:    make(chan struct{}),
	}
	stdoutDone := make(chan error, 1)
	stderrDone := make(chan captureResult, 1)
	go process.observeOutput(stdout, spec.ObserveOut, stdoutDone)
	go captureStderr(stderr, stderrDone)
	go process.reap(stdoutDone, stderrDone)
	return process, nil
}

func (p *Process) Done() <-chan struct{} {
	return p.done
}

func (p *Process) WriteContext(
	ctx context.Context,
	payload []byte,
) error {
	if ctx == nil {
		return errors.New("jobmgr root runner: nil write context")
	}
	if len(payload) == 0 {
		return errors.New("jobmgr root runner: empty write")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return errors.New("jobmgr root runner: write after child exit")
	default:
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return errors.New("jobmgr root runner: write after child exit")
	default:
	}
	callbackDone := make(chan struct{})
	var killErr error
	stop := context.AfterFunc(ctx, func() {
		killErr = p.Kill()
		close(callbackDone)
	})
	writeErr := writeFull(p.stdin, payload)
	if stop() {
		return wrapWriteError(writeErr)
	}
	<-callbackDone
	return errors.Join(ctx.Err(), killErr, wrapWriteError(writeErr))
}

func (p *Process) Signal(signal os.Signal) error {
	if signal == nil || p.command.Process == nil {
		return errors.New("jobmgr root runner: invalid process signal")
	}
	if err := p.command.Process.Signal(signal); err != nil {
		return fmt.Errorf("jobmgr root runner: signal child: %w", err)
	}
	return nil
}

func (p *Process) Kill() error {
	p.killOnce.Do(func() {
		p.closeOnce.Do(func() {
			p.closeErr = p.stdin.Close()
		})
		p.killErr = errors.Join(
			p.closeErr,
			killContained(p.command),
		)
	})
	return p.killErr
}

func (p *Process) Wait(ctx context.Context) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("jobmgr root runner: nil wait context")
	}
	select {
	case <-p.done:
		return p.result.result, p.result.err
	case <-ctx.Done():
	}

	killErr := p.Kill()
	timer := time.NewTimer(processJoinPeriod)
	defer timer.Stop()
	select {
	case <-p.done:
		return p.result.result, errors.Join(
			ctx.Err(),
			killErr,
			p.result.err,
		)
	case <-timer.C:
		return Result{}, errors.Join(
			ctx.Err(),
			killErr,
			errors.New("jobmgr root runner: process join timed out"),
		)
	}
}

func (p *Process) observeOutput(
	stdout io.ReadCloser,
	observer OutputObserver,
	done chan<- error,
) {
	defer stdout.Close()
	buffer := make([]byte, readBufferBytes)
	var observerErr error
	for {
		count, readErr := stdout.Read(buffer)
		if count > 0 && observer != nil && observerErr == nil {
			if err := observer(buffer[:count]); err != nil {
				observerErr = fmt.Errorf(
					"jobmgr root runner: observe stdout: %w",
					err,
				)
				_ = p.Kill()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				readErr = nil
			} else {
				readErr = fmt.Errorf(
					"jobmgr root runner: read stdout: %w",
					readErr,
				)
			}
			done <- errors.Join(observerErr, readErr)
			return
		}
	}
}

func (p *Process) reap(
	stdoutDone <-chan error,
	stderrDone <-chan captureResult,
) {
	stdoutErr := <-stdoutDone
	stderrResult := <-stderrDone
	waitErr := p.command.Wait()
	p.killOnce.Do(func() {})
	p.result = waitResult{
		result: Result{
			Stderr:          stderrResult.payload,
			StderrTruncated: stderrResult.truncated,
		},
		err: errors.Join(stdoutErr, stderrResult.err, waitErr),
	}
	close(p.done)
}

func validateSpec(spec Spec) error {
	if !filepath.IsAbs(spec.Executable) {
		return errors.New("jobmgr root runner: executable must be absolute")
	}
	if strings.IndexByte(spec.Executable, 0) >= 0 {
		return errors.New("jobmgr root runner: executable contains NUL")
	}
	if spec.Directory != "" && !filepath.IsAbs(spec.Directory) {
		return errors.New("jobmgr root runner: directory must be absolute")
	}
	for _, argument := range spec.Arguments {
		if strings.IndexByte(argument, 0) >= 0 {
			return errors.New("jobmgr root runner: argument contains NUL")
		}
	}
	return nil
}

func wrapWriteError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("jobmgr root runner: write child stdin: %w", err)
}

func writeFull(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		count, err := writer.Write(payload)
		if count > 0 {
			payload = payload[count:]
		}
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
