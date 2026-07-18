package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessWritesAndObservesAtParentBoundaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	var output bytes.Buffer
	var timestamps []int64
	var lock sync.Mutex
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", `IFS= read -r line; sleep 0.01; printf '%s\n' "$line"`},
		ObserveOut: func(chunk []byte, readReturnMonoNS int64) error {
			lock.Lock()
			defer lock.Unlock()
			output.Write(chunk)
			timestamps = append(timestamps, readReturnMonoNS)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	_, err = process.Write([]byte("FUNCTION request\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := process.CloseInput(); err != nil {
		t.Fatal(err)
	}
	result, err := process.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lock.Lock()
	defer lock.Unlock()
	if output.String() != "FUNCTION request\n" || len(timestamps) == 0 {
		t.Fatalf("observed output differs: output=%q reads=%v", output.String(), timestamps)
	}
	for _, timestamp := range timestamps {
		if timestamp < 0 {
			t.Fatalf("observed negative read timestamp: %v", timestamps)
		}
	}
	if len(result.Stderr) != 0 || result.StderrBytes != 0 || result.StderrTruncated {
		t.Fatalf("stderr differs: %#v", result)
	}
	if _, err := process.Wait(context.Background()); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("second Wait was not rejected: %v", err)
	}
}

func TestProcessUsesAttachedStderrWithoutStartingCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrDone := make(chan string, 1)
	go func() {
		payload, _ := io.ReadAll(stderrReader)
		stderrDone <- string(payload)
	}()
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", `printf 'attached stderr' >&2`},
		Stderr:     stderrWriter,
	})
	if closeErr := stderrWriter.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		_ = stderrReader.Close()
		t.Fatal(err)
	}
	result, err := process.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StderrBytes != 0 ||
		len(result.Stderr) != 0 ||
		result.StderrTruncated {
		t.Fatalf("attached stderr was also captured: %#v", result)
	}
	if got := <-stderrDone; got != "attached stderr" {
		t.Fatalf("attached stderr=%q", got)
	}
	if err := stderrReader.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessPreservesFastOutputIdentityAcrossConcurrentWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	const writes = 512
	observed := make(map[int]int64, writes)
	var pending []byte
	var lock sync.Mutex
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", `while IFS= read -r line; do printf '%s\n' "$line"; done`},
		ObserveOut: func(chunk []byte, readReturnMonoNS int64) error {
			lock.Lock()
			defer lock.Unlock()
			pending = append(pending, chunk...)
			for {
				newline := bytes.IndexByte(pending, '\n')
				if newline < 0 {
					return nil
				}
				sequence, err := strconv.Atoi(string(pending[:newline]))
				if err != nil {
					return err
				}
				observed[sequence] = readReturnMonoNS
				pending = pending[newline+1:]
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	for sequence := range writes {
		if _, err := process.Write([]byte(strconv.Itoa(sequence) + "\n")); err != nil {
			t.Fatalf("write %d: %v", sequence, err)
		}
	}
	if err := process.CloseInput(); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	lock.Lock()
	defer lock.Unlock()
	if len(pending) != 0 || len(observed) != writes {
		t.Fatalf("observed output differs: pending=%q count=%d", pending, len(observed))
	}
	for sequence, t1 := range observed {
		if t1 < 0 {
			t.Fatalf("sequence %d has negative read timestamp: %d", sequence, t1)
		}
	}
}

func TestProcessPreservesOSReadTimestampAcrossObserverBackpressure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	firstObserved := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondTimestamp := make(chan int64, 1)
	var once sync.Once
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", `printf 'first\n'; sleep 0.02; printf 'second\n'`},
		ObserveOut: func(chunk []byte, readReturnMonoNS int64) error {
			if bytes.Contains(chunk, []byte("first\n")) {
				once.Do(func() { close(firstObserved) })
				<-releaseFirst
			}
			if bytes.Contains(chunk, []byte("second\n")) {
				secondTimestamp <- readReturnMonoNS
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	select {
	case <-firstObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("first stdout read was not observed")
	}
	time.Sleep(100 * time.Millisecond)
	releasedAt := process.Now()
	close(releaseFirst)
	if _, err := process.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case readAt := <-secondTimestamp:
		if readAt >= releasedAt {
			t.Fatalf("second read was timestamped after queued observer release: read=%d release=%d", readAt, releasedAt)
		}
	case <-time.After(time.Second):
		t.Fatal("second stdout read was not observed")
	}
}

func TestProcessDrainsStdoutWhileAStdinWriteIsBackpressured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh and head")
	}
	const transferBytes = 1024 * 1024
	var observed int
	var lock sync.Mutex
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", "head -c 1048576 /dev/zero; head -c 1048576 >/dev/null"},
		ObserveOut: func(chunk []byte, _ int64) error {
			lock.Lock()
			observed += len(chunk)
			lock.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	writeDone := make(chan error, 1)
	go func() {
		_, err := process.Write(make([]byte, transferBytes))
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stdin write and stdout drain deadlocked under full-duplex backpressure")
	}
	if err := process.CloseInput(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := process.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	lock.Lock()
	defer lock.Unlock()
	if observed != transferBytes {
		t.Fatalf("observed stdout bytes=%d, want %d", observed, transferBytes)
	}
}

func TestProcessPublishesWriteBoundaryAfterBackpressuredWriteCompletes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh and head")
	}
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", "sleep 0.1; head -c 1048576 >/dev/null"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	before := process.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	writtenAt, err := process.WriteContext(ctx, make([]byte, 1024*1024))
	if err != nil {
		t.Fatal(err)
	}
	if delay := time.Duration(writtenAt - before); delay < 50*time.Millisecond {
		t.Fatalf("write boundary was published before the backpressured write completed: %s", delay)
	}
	if err := process.CloseInput(); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestProcessWriteContextContainsChildAndJoinsBlockedWriter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	process, err := Start(Spec{Executable: "/bin/sh", Arguments: []string{"-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := process.WriteContext(ctx, make([]byte, 1024*1024)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked write deadline differs: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("blocked writer containment took %s", elapsed)
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	if _, err := process.Wait(waitCtx); errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("reaper did not join after blocked write containment: %v", err)
	}
}

func TestProcessWriteContextRejectsPreCancelledAndEmptyWithoutKillingChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	var output bytes.Buffer
	process, err := Start(Spec{
		Executable: "/bin/sh", Arguments: []string{"-c", "IFS= read -r line; printf '%s\\n' \"$line\""},
		ObserveOut: func(chunk []byte, _ int64) error {
			_, err := output.Write(chunk)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := process.WriteContext(cancelled, []byte("ignored\n")); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled write result differs: %v", err)
	}
	if _, err := process.WriteContext(context.Background(), nil); err == nil {
		t.Fatal("empty contextual write was accepted")
	}
	if _, err := process.Write([]byte("alive\n")); err != nil {
		t.Fatal(err)
	}
	if err := process.CloseInput(); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if output.String() != "alive\n" {
		t.Fatalf("healthy child was mutated by rejected writes: %q", output.String())
	}
}

func TestProcessQueuedWriteContextExpiresWithoutKillingActiveWriter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh and head")
	}
	process, err := Start(Spec{Executable: "/bin/sh", Arguments: []string{"-c", "sleep 0.15; head -c 1048576 >/dev/null"}})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	firstDone := make(chan error, 1)
	go func() {
		_, err := process.Write(make([]byte, 1024*1024))
		firstDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for len(process.writePermit) != 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if len(process.writePermit) != 0 {
		t.Fatal("first writer did not acquire the write permit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := process.WriteContext(ctx, []byte("must-not-write\n")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued contextual write result differs: %v", err)
	}
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("queued cancellation killed the active writer: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active writer did not complete after queued cancellation")
	}
	if err := process.CloseInput(); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestProcessContainsPhysicalStdoutQueueOverflow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh and head")
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	process, err := Start(Spec{
		Executable: "/bin/sh", Arguments: []string{"-c", "head -c 8388608 /dev/zero; head -c 1048576 >/dev/null"},
		ObserveOut: func([]byte, int64) error {
			once.Do(func() { close(entered) })
			<-release
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("stdout observer was not reached")
	}
	writeDone := make(chan error, 1)
	go func() {
		_, err := process.Write(make([]byte, 1024*1024))
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err == nil {
			t.Fatal("full-duplex overflow did not contain the active writer")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("full-duplex overflow did not unblock the active writer")
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := process.Wait(ctx); !errors.Is(err, errOutputQueueOverflow) {
		t.Fatalf("stdout overflow result differs: %v", err)
	}
}

func TestOutputQueueAcceptsExactBoundAndRejectsOneBeyond(t *testing.T) {
	exact := newOutputQueue()
	var active atomic.Bool
	if err := exact.push(make([]byte, outputQueueBytes), 11, &active); err != nil {
		t.Fatalf("exact queue bound was rejected: %v", err)
	}
	active.Store(true)
	if err := exact.push([]byte{0}, 12, &active); !errors.Is(err, errOutputQueueOverflow) {
		t.Fatalf("one byte beyond queue bound was accepted: %v", err)
	}
	for blocks := 0; ; blocks++ {
		payload, readAt, ok, err := exact.next()
		if !ok {
			t.Fatalf("queue closed while draining exact bound after %d blocks: %v", blocks, err)
		}
		if len(payload) != readBufferBytes {
			t.Fatalf("queue block %d has size %d", blocks, len(payload))
		}
		if readAt != 11 {
			t.Fatalf("queue block %d timestamp=%d, want 11", blocks, readAt)
		}
		if blocks+1 == outputQueueSlots {
			break
		}
	}
	oneBeyond := newOutputQueue()
	if err := oneBeyond.push(make([]byte, outputQueueBytes+1), 13, &active); !errors.Is(err, errOutputQueueOverflow) {
		t.Fatalf("oversized atomic push was accepted: %v", err)
	}
	if oneBeyond.count != 0 {
		t.Fatalf("failed oversized push mutated queue count: %d", oneBeyond.count)
	}
}

func TestOutputQueueRechecksActiveWriterAfterRegisteringWaiter(t *testing.T) {
	queue := newOutputQueue()
	var active atomic.Bool
	active.Store(true)
	done := make(chan error, 1)
	go func() {
		queue.mu.Lock()
		done <- queue.waitForSpace(&active)
		queue.mu.Unlock()
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errOutputQueueOverflow) {
			t.Fatalf("registered waiter writer-state result differs: %v", err)
		}
	case <-time.After(time.Second):
		queue.fail(errors.New("test release"))
		t.Fatal("registered waiter slept after the writer became active")
	}
	if waiting := queue.waitingPushers.Load(); waiting != 0 {
		t.Fatalf("registered waiter count leaked: %d", waiting)
	}
}

func TestProcessUsesSanitizedEnvironmentAndBoundsStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	t.Setenv("JOBMGR_EVALUATOR_SECRET", "must-not-cross")
	var output bytes.Buffer
	process, err := Start(Spec{
		Executable:  "/bin/sh",
		Arguments:   []string{"-c", `printf '%s|%s' "$JOBMGR_EVALUATOR_SECRET" "$LC_ALL"; printf '0123456789' >&2`},
		StderrLimit: 4,
		ObserveOut: func(chunk []byte, _ int64) error {
			output.Write(chunk)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	result, err := process.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "|C" {
		t.Fatalf("child environment was not sanitized: %q", output.String())
	}
	if string(result.Stderr) != "0123" || result.StderrBytes != 10 || !result.StderrTruncated {
		t.Fatalf("bounded stderr differs: %#v", result)
	}
}

func TestProcessContextKillsDescendantGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix process groups")
	}
	process, err := Start(Spec{Executable: "/bin/sh", Arguments: []string{"-c", "sleep 30 & wait"}})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = process.Wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error differs: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("contained process group took %s to terminate", elapsed)
	}
}

func TestOutputObserverFailureTerminatesChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	want := errors.New("observer rejected output")
	process, err := Start(Spec{
		Executable: "/bin/sh", Arguments: []string{"-c", "while :; do printf x; done"},
		ObserveOut: func(_ []byte, _ int64) error { return want },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = process.Wait(ctx)
	if !errors.Is(err, want) {
		t.Fatalf("observer error differs: %v", err)
	}
}

func TestSpecRejectsInheritedEnvironmentAndAmbiguousPaths(t *testing.T) {
	for _, spec := range []Spec{
		{Executable: "relative"},
		{Executable: "/bin/cat", Directory: "relative"},
		{Executable: "/bin/cat", ExtraFiles: []*os.File{nil}},
	} {
		if _, err := Start(spec); err == nil {
			t.Fatalf("invalid spec accepted: %#v", spec)
		}
	}
}
