// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// setupDedupLoggerTest wires a temporary *os.File into the global dedupLogger
// so tests can observe the same read/write pattern that main() uses at runtime.
// The previous globals are restored on cleanup to keep other tests isolated.
func setupDedupLoggerTest(t *testing.T) *os.File {
	t.Helper()
	setupTest(t)

	tmp, err := os.CreateTemp(t.TempDir(), "dedup-race-*.log")
	if err != nil {
		t.Fatalf("create temp dedup log: %v", err)
	}

	prevLogger, prevPath := dedupLogger, dedupLogFile
	dedupLogger = tmp
	dedupLogFile = tmp.Name()

	t.Cleanup(func() {
		_ = tmp.Close()
		dedupLogger = prevLogger
		dedupLogFile = prevPath
	})
	return tmp
}

// TestDedupLogger_ConcurrentHandlersDoNotRaceOnClose reproduces the data race
// described in race.md: HTTP handlers read and write the global dedupLogger
// (server.go:385-389) without any lock, while main() closes it during shutdown
// (server.go:584-587). Under concurrent load the two paths access the same
// *os.File with no synchronization.
//
// Run with -race. Before the fix this reports DATA RACE on server.go:385 and
// server.go:387 against the closer goroutine; after the fix (guarding
// dedupLogger with a sync.RWMutex) it passes.
func TestDedupLogger_ConcurrentHandlersDoNotRaceOnClose(t *testing.T) {
	setupDedupLoggerTest(t)

	// Prime the dedup cache so every subsequent request with the same id is a
	// duplicate and takes the `else if dedupLogger != nil` branch that races.
	primer := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"id":"race-fixed"}`))
	captureOutput(t, func() { handler(httptest.NewRecorder(), primer) })

	// Silence handler stdout to keep test output readable. stderr is left
	// untouched on purpose: the Go race detector writes its DATA RACE report
	// to fd 2, and redirecting stderr would swallow that evidence.
	origStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut
	go func() { _, _ = io.Copy(io.Discard, rOut) }()
	t.Cleanup(func() {
		_ = wOut.Close()
		os.Stdout = origStdout
	})

	const (
		goroutines = 64
		duration   = 500 * time.Millisecond
	)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var reqCount int64

	body := []byte(`{"id":"race-fixed"}`)
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				handler(httptest.NewRecorder(),
					httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
				atomic.AddInt64(&reqCount, 1)
			}
		}()
	}

	// Repeatedly swap dedupLogger under the write lock to widen the window
	// that races with the reader path in handler(). Each iteration mirrors
	// the shutdown-time operation in main() (closeDedupLogger) followed by a
	// fresh openDedupLogger-like assignment done under the write lock so it
	// is itself race-free.
	closerDone := make(chan struct{})
	go func() {
		defer close(closerDone)
		time.Sleep(10 * time.Millisecond)
		for range 20 {
			select {
			case <-stop:
				return
			default:
			}
			closeDedupLogger()
			nf, err := os.CreateTemp(t.TempDir(), "dedup-race-*.log")
			if err != nil {
				return
			}
			dedupLoggerMu.Lock()
			dedupLogger = nf
			dedupLoggerMu.Unlock()
			time.Sleep(5 * time.Millisecond)
		}
		closeDedupLogger()
	}()

	time.Sleep(duration)
	close(stop)
	wg.Wait()
	<-closerDone

	t.Logf("handler invocations under contention: %d", atomic.LoadInt64(&reqCount))
}

// TestDedupLogger_HandlerAfterCloseDedupLoggerIsSilent verifies the shutdown
// semantics after the fix: closeDedupLogger() sets dedupLogger to nil under
// the write lock, so any handler waking up later observes nil and cleanly
// skips the dedup-write branch instead of writing to an already-closed *os.File.
// This is the positive counterpart to
// TestDedupLogger_ConcurrentHandlersDoNotRaceOnClose: it confirms the fix not
// only eliminates the race but also suppresses the closed-fd error log.
func TestDedupLogger_HandlerAfterCloseDedupLoggerIsSilent(t *testing.T) {
	setupDedupLoggerTest(t)

	// Prime the dedup cache so the next request with the same id is a duplicate.
	primer := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"id":"shutdown-sim"}`))
	captureOutput(t, func() { handler(httptest.NewRecorder(), primer) })

	// Simulate main() closing the dedup logger during shutdown.
	closeDedupLogger()

	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"id":"shutdown-sim"}`))
	rr := httptest.NewRecorder()
	_, stderr := captureOutput(t, func() {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr,
			&slog.HandlerOptions{Level: slog.LevelDebug})))
		handler(rr, req)
	})

	if rr.Code != http.StatusOK {
		t.Errorf("handler status: got %d, want %d", rr.Code, http.StatusOK)
	}
	if strings.Contains(stderr, "failed to write to deduplication log file") {
		t.Errorf("unexpected write-error after closeDedupLogger; got: %s", stderr)
	}
}
