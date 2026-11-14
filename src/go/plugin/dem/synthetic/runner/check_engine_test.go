//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckIsSupervisedWithoutWorkflowInput(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, a := testEngine(t)
	paths := e.source.(fixtureRuntime).Paths
	calls := 0
	e.start = starter(t, func(_ context.Context, args []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
		calls++
		assert.Empty(t, raw)
		assert.Equal(t, []string{paths.EntryPoint, "--check"}, args)
		fmt.Fprintf(out, `{"version":%q,"contract":2,"uid":%d,"gid":%d}`, paths.Identity.Version, os.Geteuid(), os.Getegid())
		return ndexec.TreeResult{Drained: true}
	})
	require.NoError(t, e.Check(context.Background()))
	assert.Equal(t, 1, calls)
	assert.Zero(t, a.finalized.Load())
}

func TestCheckRequiresTrustedReleaseAndPreservedIdentity(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	tests := map[string]struct {
		version            string
		contract, uid, gid int
		valid              bool
	}{
		"qualified release": {"release-a", synthetic.ExecutionContract, os.Geteuid(), os.Getegid(), true},
		"another release":   {"release-b", synthetic.ExecutionContract, os.Geteuid(), os.Getegid(), false},
		"another contract":  {"release-a", synthetic.ExecutionContract + 1, os.Geteuid(), os.Getegid(), false},
		"root":              {"release-a", synthetic.ExecutionContract, 0, os.Getegid(), false},
		"another uid":       {"release-a", synthetic.ExecutionContract, os.Geteuid() + 1, os.Getegid(), false},
		"another gid":       {"release-a", synthetic.ExecutionContract, os.Geteuid(), os.Getegid() + 1, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			paths := e.source.(fixtureRuntime).Paths
			calls := 0
			e.start = starter(t, func(_ context.Context, args []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
				calls++
				assert.Equal(t, []string{paths.EntryPoint, "--check"}, args)
				assert.Empty(t, raw)
				fmt.Fprintf(out, `{"version":%q,"contract":%d,"uid":%d,"gid":%d}`, tc.version, tc.contract, tc.uid, tc.gid)
				return ndexec.TreeResult{Drained: true}
			})
			err := e.Check(context.Background())
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, 1, calls)
		})
	}
}

// TestCheckResponseBounds pins the --check stdout cap and the stderr
// diagnostic kept from a failing check.
func TestCheckResponseBounds(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	reply := fmt.Sprintf(`{"version":"release-a","contract":2,"uid":%d,"gid":%d}`, os.Geteuid(), os.Getegid())
	tests := map[string]struct {
		stdout, stderr string
		failed         bool
		err            string
	}{
		"reply at limit":       {stdout: reply + strings.Repeat(" ", 4096-len(reply))},
		"reply over limit":     {stdout: reply + strings.Repeat(" ", 4097-len(reply)), err: "prepared runtime check: runtime check response exceeds 4096 bytes"},
		"clipped diagnostic":   {stderr: strings.Repeat("z", 3*textLimit), failed: true, err: "prepared runtime check: runtime check failed: exit status 1 (" + strings.Repeat("z", textLimit) + ")"},
		"multibyte diagnostic": {stderr: "a" + strings.Repeat("é", 1500), failed: true, err: "prepared runtime check: runtime check failed: exit status 1 (a" + strings.Repeat("é", 1000) + ")"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			e.start = starter(t, func(_ context.Context, _ []string, _ []byte, out, stderr *os.File) ndexec.TreeResult {
				_, _ = io.WriteString(out, tc.stdout)
				_, _ = io.WriteString(stderr, tc.stderr)
				if tc.failed {
					return ndexec.TreeResult{Drained: true, Err: errors.New("exit status 1")}
				}
				return ndexec.TreeResult{Drained: true}
			})
			err := e.Check(t.Context())
			if tc.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.err)
			}
		})
	}
}

// TestCheckViewsProbeIndependently gives a candidate view its own probe even
// for the same release identity, while it shares the fatal owner.
func TestCheckViewsProbeIndependently(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, a := testEngine(t)
	started, release := make(chan struct{}, 2), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return checkReply(out)
	})
	candidate := e.unrecordedView(e.source, a)
	monitorDone, candidateDone := make(chan error, 1), make(chan error, 1)
	go func() { monitorDone <- e.Check(t.Context()) }()
	receive(t, started)
	go func() { candidateDone <- candidate.Check(t.Context()) }()
	receive(t, started)
	assert.Same(t, e.executionOwner, candidate.executionOwner)
	close(release)
	require.NoError(t, receive(t, monitorDone))
	require.NoError(t, receive(t, candidateDone))
	assert.Equal(t, int32(2), calls.Load())
}

// TestUnverifiedCheckDrainPoisonsAllViews makes an undrained probe fatal for
// every view sharing the execution owner.
func TestUnverifiedCheckDrainPoisonsAllViews(t *testing.T) {
	e, _, a := testEngine(t)
	var calls atomic.Int32
	e.start = starter(t, func(context.Context, []string, []byte, *os.File, *os.File) ndexec.TreeResult {
		calls.Add(1)
		return ndexec.TreeResult{Drained: false, Err: ndexec.ErrTreeNotDrained}
	})
	candidate := e.unrecordedView(e.source, a)
	assert.ErrorIs(t, candidate.Check(t.Context()), ndexec.ErrTreeNotDrained)
	assert.ErrorIs(t, receive(t, e.Fatal()), ndexec.ErrTreeNotDrained)
	assert.ErrorIs(t, e.Check(t.Context()), ndexec.ErrTreeNotDrained)
	assert.False(t, e.execute(t.Context(), journey()).Drained)
	assert.Equal(t, int32(1), calls.Load())
}

// TestRejectInvalidReleaseBeforeLaunch refuses an untrusted release for both
// checks and executions without launching anything.
func TestRejectInvalidReleaseBeforeLaunch(t *testing.T) {
	tests := map[string]func(*Paths){
		"contract":            func(p *Paths) { p.Identity.Contract++ },
		"version":             func(p *Paths) { p.Identity.Version = "" },
		"digest":              func(p *Paths) { p.Identity.SHA256 = "not-a-digest" },
		"relative node":       func(p *Paths) { p.Node = "node" },
		"relative entrypoint": func(p *Paths) { p.EntryPoint = "run.mjs" },
		"missing entrypoint":  func(p *Paths) { p.EntryPoint += ".missing" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			source := e.source.(fixtureRuntime)
			change(&source.Paths)
			e.source = source
			e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
				t.Fatal("invalid release launched")
				return nil, nil
			}
			require.Error(t, e.Check(t.Context()))
			result := e.execute(t.Context(), journey())
			require.Equal(t, synthetic.Error, result.Run.Outcome)
			require.Nil(t, result.Run.Runtime)
		})
	}
}
