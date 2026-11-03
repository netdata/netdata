//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type sourceFunc func() (Use, error)

func (f sourceFunc) BeginUse() (Use, error) { return f() }

type observedUse struct {
	paths      Paths
	completion ndexec.TreeCompletion
	mu         sync.Mutex
	finishes   []bool
	finishErr  error
}

func (u *observedUse) Paths() Paths                       { return u.paths }
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

func TestExecuteAcquiresRuntimeBeforeInspectionAndPassesCompletion(t *testing.T) {
	e, _, _ := testEngine(t)
	u := &observedUse{paths: e.source.(fixtureRuntime).Paths}
	u.paths.Node = filepath.Join(t.TempDir(), "prepared-node")
	e.source = sourceFunc(func() (Use, error) {
		// The source makes its protected files available only after acquiring use.
		require.NoError(t, os.WriteFile(u.paths.Node, []byte("fixture"), 0700))
		return u, nil
	})
	child := starter(t, func(ctx context.Context, args []string, raw []byte, out, errOut *os.File) ndexec.TreeResult {
		assert.Empty(t, u.finished())
		return resultPayload(success())(ctx, args, raw, out, errOut)
	})
	e.start = func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
		assert.Equal(t, u.paths.Node, bin)
		assert.Same(t, u.Completion(), opts.Completion)
		assert.Empty(t, u.finished())
		return child(ctx, opts, bin, args...)
	}
	got := e.Execute(context.Background(), journey(), nil)
	require.True(t, got.Drained)
	require.Equal(t, synthetic.Success, got.Run.Outcome)
	assert.Equal(t, []bool{true}, u.finished())
}

func TestRuntimeUseFinishesWithActualDrainage(t *testing.T) {
	for name, tc := range map[string]struct{ badFiles, startFailure, undrained, metadataFailure bool }{
		"file check failure":           {badFiles: true},
		"launch failure":               {startFailure: true},
		"uncertain tree":               {undrained: true},
		"storage error after drainage": {metadataFailure: true},
	} {
		for _, probe := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/probe=%t", name, probe), func(t *testing.T) {
				e, _, _ := testEngine(t)
				u := &observedUse{paths: e.source.(fixtureRuntime).Paths}
				if tc.badFiles {
					u.paths.Node = filepath.Join(t.TempDir(), "absent")
				}
				if tc.metadataFailure {
					u.finishErr = errors.New("owner records metadata storage failure")
				}
				e.source = sourceFunc(func() (Use, error) { return u, nil })
				starts := 0
				child := starter(t, func(ctx context.Context, args []string, raw []byte, out, errOut *os.File) ndexec.TreeResult {
					if tc.undrained {
						return ndexec.TreeResult{Err: ndexec.ErrTreeNotDrained}
					}
					if probe {
						_, _ = out.WriteString("version")
						return ndexec.TreeResult{Drained: true}
					}
					return resultPayload(success())(ctx, args, raw, out, errOut)
				})
				e.start = func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
					starts++
					assert.Same(t, u.Completion(), opts.Completion)
					if tc.startFailure {
						return nil, errors.New("failed before launch")
					}
					return child(ctx, opts, bin, args...)
				}
				if probe {
					result, err := e.probe(context.Background(), synthetic.Journey, func(Paths) []string { return []string{"--version"} })
					if tc.metadataFailure {
						require.NoError(t, err)
						assert.Equal(t, "version", string(result))
					} else {
						require.Error(t, err)
					}
				} else {
					result := e.Execute(context.Background(), journey(), nil)
					assert.Equal(t, !tc.undrained, result.Drained)
					if tc.metadataFailure {
						assert.Equal(t, synthetic.Success, result.Run.Outcome)
					}
				}
				if tc.badFiles {
					assert.Zero(t, starts)
				} else {
					assert.Equal(t, 1, starts)
				}
				assert.Equal(t, []bool{!tc.undrained}, u.finished())
				assert.Equal(t, tc.undrained, e.isPoisoned())
			})
		}
	}
}

func TestCheckUsesOneAcquiredRelease(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, _ := testEngine(t)
	paths := e.source.(fixtureRuntime).Paths
	var uses []*observedUse
	e.source = sourceFunc(func() (Use, error) {
		paths.Identity.Version = fmt.Sprintf("release-%d", len(uses))
		u := &observedUse{paths: paths}
		uses = append(uses, u)
		return u, nil
	})
	e.start = func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
		require.Len(t, uses, 1)
		u := uses[0]
		assert.Same(t, u.Completion(), opts.Completion)
		assert.Equal(t, u.paths.Node, bin)
		assert.Equal(t, []string{u.paths.EntryPoint, "--check"}, args)
		return starter(t, func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
			fmt.Fprintf(out, `{"version":%q,"contract":1,"uid":%d,"gid":%d}`, u.paths.Identity.Version, os.Geteuid(), os.Getegid())
			return ndexec.TreeResult{Drained: true}
		})(ctx, opts, bin, args...)
	}
	require.NoError(t, e.Check(context.Background(), synthetic.Journey))
	require.Len(t, uses, 1)
	assert.Equal(t, []bool{true}, uses[0].finished())
}

func TestRuntimeUseWaitsForLocalReaderJoin(t *testing.T) {
	e, _, _ := testEngine(t)
	u := &observedUse{paths: e.source.(fixtureRuntime).Paths}
	e.source = sourceFunc(func() (Use, error) { return u, nil })
	waited := make(chan struct{})
	var held *os.File
	child := starter(t, resultPayload(success()))
	e.start = func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
		fd, err := unix.Dup(int(opts.Stdout.Fd()))
		if err != nil {
			return nil, err
		}
		held = os.NewFile(uintptr(fd), "held-reader-join-fixture")
		process, err := child(ctx, opts, bin, args...)
		if err != nil {
			held.Close()
			return nil, err
		}
		return &notifyingTree{treeProcess: process, waited: waited}, nil
	}
	result := make(chan synthetic.Execution, 1)
	go func() { result <- e.Execute(context.Background(), journey(), nil) }()
	select {
	case <-waited:
	case got := <-result:
		t.Fatalf("execution returned before starting the reader-join fixture: %+v", got)
	}
	defer held.Close()
	assert.Empty(t, u.finished())
	select {
	case <-result:
		t.Fatal("returned before local reader joined")
	case <-time.After(25 * time.Millisecond):
	}
	require.NoError(t, held.Close())
	require.True(t, (<-result).Drained)
	assert.Equal(t, []bool{true}, u.finished())
}

type notifyingTree struct {
	treeProcess
	waited chan struct{}
}

func (p *notifyingTree) Wait() ndexec.TreeResult {
	result := p.treeProcess.Wait()
	close(p.waited)
	return result
}

func TestUnavailableSourceDoesNotLaunch(t *testing.T) {
	for _, probe := range []bool{false, true} {
		e, _, _ := testEngine(t)
		e.source = sourceFunc(func() (Use, error) { return nil, errors.New("runtime unavailable") })
		e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
			t.Fatal("launched without runtime use")
			return nil, nil
		}
		if probe {
			require.ErrorContains(t, e.Check(context.Background(), synthetic.Journey), "runtime unavailable")
		} else {
			got := e.Execute(context.Background(), journey(), nil)
			assert.True(t, got.Drained)
			assert.Contains(t, got.Run.Error, "runtime unavailable")
		}
	}
}

func TestConcurrentCheckAndExecutionKeepDistinctRuntimeUses(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Check requires preserved nonroot identity")
	}
	e, _, _ := testEngine(t)
	paths := e.source.(fixtureRuntime).Paths
	var mu sync.Mutex
	var uses []*observedUse
	byCompletion := make(map[*ndexec.TreeCompletion]*observedUse)
	e.source = sourceFunc(func() (Use, error) {
		invocation := paths
		invocation.Node = filepath.Join(t.TempDir(), "node")
		if err := os.WriteFile(invocation.Node, []byte("fixture"), 0700); err != nil {
			return nil, err
		}
		u := &observedUse{paths: invocation}
		mu.Lock()
		uses = append(uses, u)
		byCompletion[u.Completion()] = u
		mu.Unlock()
		return u, nil
	})
	running, release := make(chan struct{}), make(chan struct{})
	e.start = func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
		mu.Lock()
		u := byCompletion[opts.Completion]
		mu.Unlock()
		if u == nil {
			return nil, errors.New("invocation has no matching runtime use")
		}
		assert.Equal(t, u.paths.Node, bin)
		return starter(t, func(ctx context.Context, args []string, raw []byte, out, errOut *os.File) ndexec.TreeResult {
			if len(args) == 1 {
				close(running)
				<-release
				assert.Empty(t, u.finished())
				return resultPayload(success())(ctx, args, raw, out, errOut)
			}
			fmt.Fprintf(out, `{"version":%q,"contract":1,"uid":%d,"gid":%d}`, u.paths.Identity.Version, os.Geteuid(), os.Getegid())
			return ndexec.TreeResult{Drained: true}
		})(ctx, opts, bin, args...)
	}
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(context.Background(), journey(), nil) }()
	select {
	case <-running:
	case got := <-done:
		t.Fatalf("execution returned before starting the concurrent fixture: %+v", got)
	}
	checkErr := e.Check(context.Background(), synthetic.Journey)
	close(release)
	got := <-done
	require.NoError(t, checkErr)
	require.Equal(t, synthetic.Success, got.Run.Outcome)
	require.Len(t, uses, 2)
	for _, u := range uses {
		assert.Equal(t, []bool{true}, u.finished())
	}
}
