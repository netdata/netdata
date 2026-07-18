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
	"sync/atomic"
	"time"
)

const (
	defaultStderrLimit = 64 * 1024
	maximumStderrLimit = 1024 * 1024
	readBufferBytes    = 32 * 1024
	outputQueueBytes   = 4 * 1024 * 1024
	outputQueueSlots   = outputQueueBytes / readBufferBytes
)

var errOutputQueueOverflow = errors.New("evaluator runner: stdout queue exceeded 4 MiB")

var childEnvironment = [...]string{
	"PATH=/usr/bin:/bin",
	"LANG=C",
	"LC_ALL=C",
	"TZ=UTC",
	"GOMAXPROCS=4",
}

func ChildEnvironment() []string {
	return append([]string(nil), childEnvironment[:]...)
}

// OutputObserver must return promptly after consuming chunk. It must not call Wait;
// the queued chunk and process reaper remain owned by the runner until it returns.
type OutputObserver func(chunk []byte, observationMonoNS int64) error

type Spec struct {
	Executable  string
	Arguments   []string
	Directory   string
	ExtraFiles  []*os.File
	ObserveOut  OutputObserver
	Stderr      *os.File
	StderrLimit int
}

type Result struct {
	Stderr          []byte
	StderrBytes     int64
	StderrTruncated bool
}

type Process struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	clock   monotonicClock
	output  *outputQueue

	writePermit chan struct{}
	writeActive atomic.Bool
	closeOnce   sync.Once
	closeErr    error
	killOnce    sync.Once
	killErr     error
	waitMu      sync.Mutex
	waited      bool
	done        chan waitResult
	exited      chan struct{}
}

type waitResult struct {
	result Result
	err    error
}

type outputQueue struct {
	mu             sync.Mutex
	ready          *sync.Cond
	waitingPushers atomic.Int32
	blocks         [outputQueueSlots]outputBlock
	head           int
	count          int
	closed         bool
	err            error
}

type outputBlock struct {
	payload          []byte
	readReturnMonoNS int64
}

func newOutputQueue() *outputQueue {
	queue := &outputQueue{}
	queue.ready = sync.NewCond(&queue.mu)
	return queue
}

func Start(spec Spec) (*Process, error) {
	if err := validateSpec(&spec); err != nil {
		return nil, err
	}
	command := exec.Command(spec.Executable, spec.Arguments...)
	command.Dir = spec.Directory
	command.Env = ChildEnvironment()
	command.ExtraFiles = append([]*os.File(nil), spec.ExtraFiles...)
	configureContainment(command)

	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("evaluator runner: create stdin pipe: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("evaluator runner: create stdout pipe: %w", err)
	}
	var stderr io.ReadCloser
	if spec.Stderr == nil {
		stderr, err = command.StderrPipe()
		if err != nil {
			_ = stdin.Close()
			_ = stdout.Close()
			return nil, fmt.Errorf(
				"evaluator runner: create stderr pipe: %w",
				err,
			)
		}
	} else {
		command.Stderr = spec.Stderr
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		if stderr != nil {
			_ = stderr.Close()
		}
		return nil, fmt.Errorf("evaluator runner: start: %w", err)
	}

	queue := newOutputQueue()
	process := &Process{
		command: command, stdin: stdin, clock: newMonotonicClock(),
		output: queue, writePermit: make(chan struct{}, 1), done: make(chan waitResult, 1), exited: make(chan struct{}),
	}
	process.writePermit <- struct{}{}
	stdoutDone := make(chan error, 1)
	stderrDone := make(chan captureResult, 1)
	go process.observeOutput(stdout, queue, spec.ObserveOut, stdoutDone)
	if stderr == nil {
		stderrDone <- captureResult{}
	} else {
		go captureStderr(stderr, spec.StderrLimit, stderrDone)
	}
	go process.reap(stdoutDone, stderrDone)
	return process, nil
}

func (process *Process) Write(payload []byte) (int64, error) {
	if len(payload) == 0 {
		return 0, errors.New("evaluator runner: empty write")
	}
	<-process.writePermit
	defer func() { process.writePermit <- struct{}{} }()
	return process.writeOwned(payload)
}

func (process *Process) writeOwned(payload []byte) (int64, error) {
	select {
	case <-process.exited:
		return 0, errors.New("evaluator runner: write after child exit")
	default:
	}
	process.writeActive.Store(true)
	process.output.wake()
	err := writeFull(process.stdin, payload)
	writtenAt := int64(0)
	if err == nil {
		writtenAt = process.clock.now()
	}
	process.writeActive.Store(false)
	if err != nil {
		return 0, wrapWriteError(err)
	}
	return writtenAt, nil
}

func (process *Process) WriteContext(ctx context.Context, payload []byte) (int64, error) {
	if ctx == nil {
		return 0, errors.New("evaluator runner: nil write context")
	}
	if len(payload) == 0 {
		return 0, errors.New("evaluator runner: empty write")
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-process.exited:
		return 0, errors.New("evaluator runner: write after child exit")
	case <-process.writePermit:
	}
	defer func() { process.writePermit <- struct{}{} }()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-process.exited:
		return 0, errors.New("evaluator runner: write after child exit")
	default:
	}
	callbackDone := make(chan struct{})
	var killErr error
	stop := context.AfterFunc(ctx, func() {
		defer close(callbackDone)
		killErr = process.Kill()
	})
	writtenAt, writeErr := process.writeOwned(payload)
	if stop() {
		return writtenAt, writeErr
	}
	<-callbackDone
	if ctxErr := ctx.Err(); ctxErr != nil {
		return 0, errors.Join(ctxErr, killErr, writeErr)
	}
	return 0, errors.Join(errors.New("evaluator runner: write cancellation callback ran without context error"), killErr, writeErr)
}

func wrapWriteError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("evaluator runner: write child stdin: %w", err)
}

func (process *Process) CloseInput() error {
	process.closeOnce.Do(func() { process.closeErr = process.stdin.Close() })
	return process.closeErr
}

func (process *Process) Signal(signal os.Signal) error {
	if signal == nil || process.command == nil || process.command.Process == nil {
		return errors.New("evaluator runner: invalid process signal")
	}
	if err := process.command.Process.Signal(signal); err != nil {
		return fmt.Errorf("evaluator runner: signal child: %w", err)
	}
	return nil
}

func (process *Process) Kill() error {
	process.killOnce.Do(func() {
		_ = process.CloseInput()
		process.killErr = killContained(process.command)
	})
	return process.killErr
}

func (process *Process) Wait(ctx context.Context) (Result, error) {
	process.waitMu.Lock()
	if process.waited {
		process.waitMu.Unlock()
		return Result{}, errors.New("evaluator runner: Wait called more than once")
	}
	process.waited = true
	process.waitMu.Unlock()
	select {
	case completed := <-process.done:
		return completed.result, completed.err
	case <-ctx.Done():
		killErr := process.Kill()
		completed := <-process.done
		return completed.result, errors.Join(ctx.Err(), killErr, completed.err)
	}
}

func (process *Process) PID() int {
	if process.command.Process == nil {
		return 0
	}
	return process.command.Process.Pid
}

func (process *Process) Now() int64 {
	return process.clock.now()
}

func (process *Process) observeOutput(stdout io.ReadCloser, queue *outputQueue, observer OutputObserver, done chan<- error) {
	go process.readOutput(stdout, queue)
	var observerErr error
	for {
		chunk, readReturnMonoNS, ok, readErr := queue.next()
		if !ok {
			done <- errors.Join(observerErr, readErr)
			return
		}
		if observer == nil || observerErr != nil {
			continue
		}
		if err := observer(chunk, readReturnMonoNS); err != nil {
			observerErr = fmt.Errorf("evaluator runner: observe stdout: %w", err)
			queue.fail(observerErr)
			_ = process.Kill()
		}
	}
}

func (process *Process) readOutput(stdout io.ReadCloser, queue *outputQueue) {
	defer stdout.Close()
	buffer := make([]byte, readBufferBytes)
	for {
		count, err := stdout.Read(buffer)
		if count > 0 {
			readReturnMonoNS := process.clock.now()
			if queueErr := queue.push(buffer[:count], readReturnMonoNS, &process.writeActive); queueErr != nil {
				queue.fail(queueErr)
				_ = process.Kill()
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				queue.close(nil)
			} else {
				queue.close(fmt.Errorf("evaluator runner: read stdout: %w", err))
			}
			return
		}
	}
}

func (queue *outputQueue) push(payload []byte, readReturnMonoNS int64, writeActive *atomic.Bool) error {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for {
		if queue.closed {
			return errors.New("evaluator runner: stdout queue closed")
		}
		available := (outputQueueSlots - queue.count) * readBufferBytes
		if len(payload) <= available {
			break
		}
		if writeActive == nil || writeActive.Load() {
			return errOutputQueueOverflow
		}
		if err := queue.waitForSpace(writeActive); err != nil {
			return err
		}
	}
	for len(payload) > 0 {
		tail := (queue.head + queue.count) % outputQueueSlots
		block := outputBlock{payload: make([]byte, 0, readBufferBytes), readReturnMonoNS: readReturnMonoNS}
		count := len(payload)
		if count > readBufferBytes {
			count = readBufferBytes
		}
		block.payload = append(block.payload, payload[:count]...)
		queue.blocks[tail] = block
		queue.count++
		payload = payload[count:]
	}
	queue.ready.Signal()
	return nil
}

// waitForSpace closes the transition window between the first writer-state
// check and sleeping on the condition variable. The queue mutex is held.
func (queue *outputQueue) waitForSpace(writeActive *atomic.Bool) error {
	queue.waitingPushers.Add(1)
	defer queue.waitingPushers.Add(-1)
	if writeActive == nil || writeActive.Load() {
		return errOutputQueueOverflow
	}
	queue.ready.Wait()
	return nil
}

func (queue *outputQueue) next() ([]byte, int64, bool, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for queue.count == 0 && !queue.closed {
		queue.ready.Wait()
	}
	if queue.count == 0 {
		return nil, 0, false, queue.err
	}
	block := queue.blocks[queue.head]
	queue.blocks[queue.head] = outputBlock{}
	queue.head = (queue.head + 1) % outputQueueSlots
	queue.count--
	queue.ready.Broadcast()
	return block.payload, block.readReturnMonoNS, true, nil
}

func (queue *outputQueue) wake() {
	if queue.waitingPushers.Load() == 0 {
		return
	}
	queue.mu.Lock()
	queue.ready.Broadcast()
	queue.mu.Unlock()
}

func (queue *outputQueue) close(err error) {
	queue.mu.Lock()
	if !queue.closed {
		queue.closed = true
		queue.err = err
		queue.ready.Broadcast()
	}
	queue.mu.Unlock()
}

func (queue *outputQueue) fail(err error) {
	queue.mu.Lock()
	if !queue.closed {
		for index := range queue.blocks {
			queue.blocks[index] = outputBlock{}
		}
		queue.head = 0
		queue.count = 0
		queue.closed = true
		queue.err = err
		queue.ready.Broadcast()
	}
	queue.mu.Unlock()
}

func (process *Process) reap(stdoutDone <-chan error, stderrDone <-chan captureResult) {
	stdoutErr := <-stdoutDone
	stderrResult := <-stderrDone
	waitErr := process.command.Wait()
	process.killOnce.Do(func() {})
	close(process.exited)
	process.done <- waitResult{
		result: Result{Stderr: stderrResult.payload, StderrBytes: stderrResult.total, StderrTruncated: stderrResult.truncated},
		err:    errors.Join(stdoutErr, stderrResult.err, waitErr),
	}
}

func validateSpec(spec *Spec) error {
	if !filepath.IsAbs(spec.Executable) {
		return errors.New("evaluator runner: executable must be absolute")
	}
	if strings.IndexByte(spec.Executable, 0) >= 0 {
		return errors.New("evaluator runner: executable contains NUL")
	}
	if spec.Directory != "" && !filepath.IsAbs(spec.Directory) {
		return errors.New("evaluator runner: directory must be absolute")
	}
	for _, argument := range spec.Arguments {
		if strings.IndexByte(argument, 0) >= 0 {
			return errors.New("evaluator runner: argument contains NUL")
		}
	}
	for _, file := range spec.ExtraFiles {
		if file == nil {
			return errors.New("evaluator runner: nil extra file")
		}
	}
	if spec.StderrLimit == 0 {
		spec.StderrLimit = defaultStderrLimit
	}
	if spec.StderrLimit < 0 || spec.StderrLimit > maximumStderrLimit {
		return fmt.Errorf("evaluator runner: stderr limit must be within 1..%d", maximumStderrLimit)
	}
	return nil
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

type monotonicClock struct {
	origin time.Time
}

func newMonotonicClock() monotonicClock {
	return monotonicClock{origin: time.Now()}
}

func (clock monotonicClock) now() int64 {
	return time.Since(clock.origin).Nanoseconds()
}
