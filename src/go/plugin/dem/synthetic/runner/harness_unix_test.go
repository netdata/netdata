//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type fakeTree struct{ done chan ndexec.TreeResult }

func (p *fakeTree) Wait() ndexec.TreeResult { return <-p.done }

type fakeHistory struct {
	mu        sync.Mutex
	phases    []string
	runs      []synthetic.Run
	err       error
	syncErr   error
	syncCalls int
}

func (h *fakeHistory) AppendRun(_ context.Context, phase synthetic.HistoryPhase, run synthetic.Run) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.phases = append(h.phases, string(phase))
	h.runs = append(h.runs, run)
	return true, h.err
}

func (h *fakeHistory) Sync(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.syncCalls++
	return h.syncErr
}

type observedArtifacts struct {
	*artifacts.Store
	finalized atomic.Int32
	beginErr  error
	mu        sync.Mutex
	begun     []string
}

func (a *observedArtifacts) Begin(runID, jobID, vnodeGUID string) (string, error) {
	if a.beginErr != nil {
		return "", a.beginErr
	}
	work, err := a.Store.Begin(runID, jobID, vnodeGUID)
	if err == nil {
		a.mu.Lock()
		a.begun = append(a.begun, work)
		a.mu.Unlock()
	}
	return work, err
}

func (a *observedArtifacts) Finalize(
	ctx context.Context,
	id string,
	c []synthetic.Capture,
) ([]synthetic.Artifact, error) {
	a.finalized.Add(1)
	return a.Store.Finalize(ctx, id, c)
}

type payload func(context.Context, []string, []byte, *os.File, *os.File) ndexec.TreeResult

func starter(t *testing.T, body payload) startFunc {
	t.Helper()
	return func(ctx context.Context, opts ndexec.TreeOptions, _ string, args ...string) (treeProcess, error) {
		pipes, err := duplicatePipes(opts.Stdin, opts.Stdout, opts.Stderr)
		if err != nil {
			return nil, err
		}
		input, output, stderr := pipes[0], pipes[1], pipes[2]
		process := &fakeTree{
			done: make(chan ndexec.TreeResult, 1),
		}
		go func() {
			defer input.Close()
			defer output.Close()
			defer stderr.Close()
			raw, _ := io.ReadAll(input)
			process.done <- body(ctx, args, raw, output, stderr)
		}()
		return process, nil
	}
}

func duplicatePipes(files ...*os.File) ([]*os.File, error) {
	var pipes []*os.File
	for _, file := range files {
		fd, err := unix.Dup(int(file.Fd()))
		if err != nil {
			for _, opened := range pipes {
				opened.Close()
			}
			return nil, err
		}
		pipes = append(pipes, os.NewFile(uintptr(fd), "test-owned-child-pipe"))
	}
	return pipes, nil
}

func testEngine(t *testing.T) (*Engine, *fakeHistory, *observedArtifacts) {
	t.Helper()
	return testEngineSlots(t, 1)
}

func testEngineSlots(t *testing.T, concurrency int) (*Engine, *fakeHistory, *observedArtifacts) {
	t.Helper()
	base := t.TempDir()
	bin, err := os.Executable()
	require.NoError(t, err)
	entrypoint := filepath.Join(base, "run.mjs")
	require.NoError(t, os.WriteFile(entrypoint, []byte("// runtime entrypoint fixture"), 0600))
	store, err := artifacts.Open(filepath.Join(base, "artifacts"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
		entries, err := os.ReadDir(filepath.Join(base, "artifacts", "work"))
		require.NoError(t, err)
		// All fake-payload pipe owners have joined before this cleanup runs.
		for _, entry := range entries {
			alias := filepath.Join("/tmp", "nd-dem-"+entry.Name())
			if target, err := os.Readlink(alias); err == nil &&
				target == filepath.Join(base, "artifacts", "work", entry.Name()) {
				require.NoError(t, os.Remove(alias))
			}
		}
	})
	history := &fakeHistory{}
	owner := &observedArtifacts{
		Store: store,
	}
	engine, err := New(
		fixtureRuntime{Paths: Paths{
			Node:       bin,
			EntryPoint: entrypoint,
			Identity:   synthetic.RuntimeIdentity{Version: "release-a", SHA256: strings.Repeat("a", 64), Contract: synthetic.ExecutionContract},
		}},
		history,
		owner,
		concurrency,
	)
	require.NoError(t, err)
	engine.platform = func() error { return nil }
	return engine, history, owner
}

func journey() synthetic.Request {
	return synthetic.Request{
		Kind:    synthetic.Journey,
		Name:    "checkout",
		Script:  "synthetic source",
		Timeout: time.Second,
	}
}

func resultPayload(result reporterResult) payload {
	return func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		emit(out, frame{
			Type:   "result",
			Result: &result,
		})
		return ndexec.TreeResult{
			Drained: true,
		}
	}
}

func intPointer(value int) *int { return &value }

type fixtureRuntime struct{ Paths Paths }

func (r fixtureRuntime) BeginUse() (Use, error) { return fixtureUse{r.Paths}, nil }

type fixtureUse struct{ paths Paths }

func (u fixtureUse) Paths() Paths { return u.paths }

func (u fixtureUse) Completion() *ndexec.TreeCompletion { return nil }

func (u fixtureUse) Finish(bool) error { return nil }

// requireSlotFree proves the attempt released its admission slot.
func requireSlotFree(t *testing.T, e *Engine) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	job, err := e.acquire(ctx, executionKey{jobID: "slot-probe"})
	require.NoError(t, err, "the attempt kept its admission slot")
	e.release(job)
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for owned test operation")
		var zero T
		return zero
	}
}

// waitQueued waits until jobID has n queued requests. Engine tests cannot use
// synctest: their fake children block on real pipes, which never count as
// durably blocked.
func waitQueued(t *testing.T, e *Engine, jobID string, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		j := e.jobs[executionKey{jobID: jobID}]
		return j != nil && len(j.waiters) == n
	}, time.Second, time.Millisecond, "request did not join its job's queue")
}

func namedRequest(name, script string) synthetic.Request {
	r := journey()
	r.Name = name
	r.Script = script
	r.Timeout = 5 * time.Second
	return r
}

type sourceFunc func() (Use, error)

func (f sourceFunc) BeginUse() (Use, error) { return f() }

type observedUse struct {
	paths      Paths
	completion ndexec.TreeCompletion
	mu         sync.Mutex
	finishes   []bool
	finishErr  error
}

func (u *observedUse) Paths() Paths { return u.paths }

func (u *observedUse) Completion() *ndexec.TreeCompletion { return &u.completion }

func (u *observedUse) Finish(drained bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.finishes = append(u.finishes, drained)
	return u.finishErr
}

func (u *observedUse) finished() []bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]bool(nil), u.finishes...)
}

// checkReply answers --check for the fixture release as the current identity.
func checkReply(out *os.File) ndexec.TreeResult {
	_, err := fmt.Fprintf(out, `{"version":"release-a","contract":2,"uid":%d,"gid":%d}`, os.Geteuid(), os.Getegid())
	return ndexec.TreeResult{Drained: true, Err: err}
}
