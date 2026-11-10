//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testBrowserOwner struct {
	id       string
	work     string
	stopping chan struct{}
	receipt  chan ndexec.TreeResult
}

type browserObservedUse struct {
	*observedUse
	finished func(bool)
}

func (u browserObservedUse) Finish(drained bool) error {
	u.finished(drained)
	return nil
}

func withBrowserOwners(t *testing.T, attempt startFunc, owners chan<- testBrowserOwner) startFunc {
	t.Helper()
	return func(ctx context.Context, opts ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
		if !slices.Contains(args, "--browser-owner") {
			return attempt(ctx, opts, bin, args...)
		}
		pipes, err := duplicatePipes(opts.Stdin, opts.Stdout, opts.Stderr)
		if err != nil {
			return nil, err
		}
		input, output, stderr := pipes[0], pipes[1], pipes[2]
		process := &fakeTree{done: make(chan ndexec.TreeResult, 1)}
		go func() {
			defer input.Close()
			defer output.Close()
			defer stderr.Close()
			reader := bufio.NewReader(input)
			raw, err := reader.ReadBytes('\n')
			if err != nil {
				process.done <- ndexec.TreeResult{Drained: true, Err: err}
				return
			}
			var request struct {
				Version int    `json:"version"`
				ID      string `json:"owner_id"`
				Work    string `json:"work_dir"`
			}
			if err := json.Unmarshal(raw, &request); err != nil {
				process.done <- ndexec.TreeResult{Drained: true, Err: err}
				return
			}
			assert.Equal(t, SupportedContract, request.Version)
			owner := testBrowserOwner{id: request.ID, work: request.Work, stopping: make(chan struct{}), receipt: make(chan ndexec.TreeResult, 1)}
			if owners != nil {
				owners <- owner
			} else {
				owner.receipt <- ndexec.TreeResult{Drained: true}
			}
			err = json.NewEncoder(output).Encode(map[string]string{"type": "ready", "owner_id": request.ID, "endpoint": request.Work + "/browser.sock"})
			if err != nil {
				process.done <- ndexec.TreeResult{Drained: true, Err: err}
				return
			}
			eof := make(chan struct{})
			go func() {
				_, _ = io.Copy(io.Discard, reader)
				close(eof)
			}()
			select {
			case <-eof:
			case <-ctx.Done():
				_ = input.Close()
				<-eof
			}
			close(owner.stopping)
			select {
			case result := <-owner.receipt:
				process.done <- result
			case <-ctx.Done():
				process.done <- ndexec.TreeResult{Drained: true, Err: ctx.Err()}
			}
		}()
		return process, nil
	}
}

func reuseRequest(name string) synthetic.Request {
	request := namedRequest(name, name)
	request.BrowserMode = "reuse"
	return request
}

func emitBrowserReport(w io.Writer, loan *browserLoan, state string) error {
	return json.NewEncoder(w).Encode(frame{Type: "browser", Browser: &browserReport{
		OwnerID: loan.OwnerID, Epoch: loan.Epoch, State: state,
	}})
}

func TestBrowserSocketMustRemainInOwnedDirectory(t *testing.T) {
	const work = "/tmp/nd-dem-owner-test"
	require.True(t, validBrowserEndpoint(work+"/home/browser/one.sock", work))
	for _, value := range []string{"ws://127.0.0.1:1234/path", "/tmp/other/browser.sock", work + "-other/browser.sock",
		work + "/../browser.sock", work + "/browser.sock\x00", work + "/" + strings.Repeat("a", 100) + ".sock"} {
		assert.False(t, validBrowserEndpoint(value, work), value)
	}
}

func TestBrowserPoolReplacesContinuouslyBusyOwner(t *testing.T) {
	e, _, _ := testEngine(t)
	var ids []string
	e.start = withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		assert.NoError(t, json.Unmarshal(raw, &req))
		ids = append(ids, req.Browser.OwnerID)
		assert.NoError(t, emitBrowserReport(out, req.Browser, "clean"))
		return resultPayload(reporterResult{Status: synthetic.Success, Tests: &synthetic.TestCounts{Declared: 1, Passed: 1}, CaptureState: "disabled"})(ctx, args, raw, out, stderr)
	}), nil)
	for i := 0; i <= browserMaxLoans; i++ {
		result := e.Execute(t.Context(), reuseRequest("busy"), nil)
		require.True(t, result.Drained)
		require.Equal(t, synthetic.Success, result.Run.Outcome, result.Run.Error)
	}
	require.Len(t, ids, browserMaxLoans+1)
	for _, id := range ids[:browserMaxLoans] {
		assert.Equal(t, ids[0], id)
	}
	assert.NotEqual(t, ids[0], ids[browserMaxLoans])
	require.NoError(t, e.Close(t.Context()))
}

func TestBrowserPoolRetainsFairSlotThroughPhysicalDrain(t *testing.T) {
	e, history, artifacts := testEngineSlots(t, 2)
	owners := make(chan testBrowserOwner, 4)
	requests := make(chan wireRequest, 8)
	cliReceipt := make(chan struct{})
	cliClean := make(chan struct{})
	var firstOwner, staleEpoch string
	e.start = withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		requests <- req
		if req.Browser != nil {
			report := *req.Browser
			switch req.Script {
			case "first":
				firstOwner, staleEpoch = report.OwnerID, report.Epoch
			case "stale":
				assert.Equal(t, firstOwner, report.OwnerID)
				assert.NotEqual(t, staleEpoch, report.Epoch)
				report.Epoch = staleEpoch
			}
			assert.NoError(t, emitBrowserReport(out, &report, "clean"))
			_, _ = io.WriteString(stderr, "private: "+req.Browser.Endpoint+"\n")
		}
		result := resultPayload(success())(ctx, args, raw, out, stderr)
		if req.Script == "first" {
			close(cliClean)
			<-cliReceipt
		}
		return result
	}), owners)
	first, stale, peer := make(chan synthetic.Execution, 1), make(chan synthetic.Execution, 1), make(chan synthetic.Execution, 1)
	go func() { first <- e.Execute(t.Context(), reuseRequest("first"), nil) }()
	owner := receive(t, owners)
	require.Equal(t, owner.id, receive(t, requests).Browser.OwnerID)
	receive(t, cliClean)
	staleRequest := reuseRequest("first")
	staleRequest.Script = "stale"
	go func() { stale <- e.Execute(t.Context(), staleRequest, nil) }()
	queued(t, e, "first", 1)
	require.Zero(t, artifacts.finalized.Load(), "a clean frame does not release the slot or publish artifacts")
	close(cliReceipt)
	firstRun := receive(t, first)
	require.Equal(t, synthetic.Success, firstRun.Run.Outcome)
	foundRedacted := false
	for _, event := range firstRun.Run.Events {
		assert.NotContains(t, event.Message, owner.id, "private loan endpoint must be redacted")
		foundRedacted = foundRedacted || event.Message == "private: [REDACTED]\n"
	}
	assert.True(t, foundRedacted)

	require.Equal(t, owner.id, receive(t, requests).Browser.OwnerID)
	receive(t, owner.stopping)
	assert.Equal(t, int32(1), artifacts.finalized.Load(), "owner retirement fences terminal publication")
	go func() { peer <- e.Execute(t.Context(), namedRequest("peer", "peer"), nil) }()
	require.Nil(t, receive(t, requests).Browser)
	require.Equal(t, synthetic.Success, receive(t, peer).Run.Outcome)
	assert.DirExists(t, owner.work)
	owner.receipt <- ndexec.TreeResult{Drained: true}
	staleRun := receive(t, stale)
	require.True(t, staleRun.Drained)
	require.Equal(t, synthetic.Error, staleRun.Run.Outcome)
	assert.Contains(t, staleRun.Run.Error, "invalid managed browser acknowledgment")
	assert.NoDirExists(t, owner.work)
	history.mu.Lock()
	assert.Len(t, history.phases, 6)
	history.mu.Unlock()
	require.NoError(t, e.Close(t.Context()))
}

func TestBrowserPoolFreshHandoffAndIndependentRuntimeUses(t *testing.T) {
	e, _, _ := testEngine(t)
	paths := e.source.(fixtureRuntime).Paths
	type completion struct {
		use     int
		drained bool
	}
	finished := make(chan completion, 8)
	var next int
	e.source = sourceFunc(func() (Use, error) {
		next++
		index := next
		return browserObservedUse{
			observedUse: &observedUse{paths: paths},
			finished:    func(drained bool) { finished <- completion{index, drained} },
		}, nil
	})
	owners := make(chan testBrowserOwner, 2)
	launch := make(chan wireRequest, 4)
	e.start = withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		err := json.Unmarshal(raw, &req)
		assert.NoError(t, err)
		launch <- req
		if req.Browser != nil {
			assert.NoError(t, emitBrowserReport(out, req.Browser, "clean"))
		}
		return resultPayload(success())(ctx, args, raw, out, stderr)
	}), owners)
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(t.Context(), reuseRequest("reuse"), nil) }()
	owner := receive(t, owners)
	require.Equal(t, owner.id, receive(t, launch).Browser.OwnerID)
	require.Equal(t, synthetic.Success, receive(t, done).Run.Outcome)
	require.Equal(t, completion{1, true}, receive(t, finished), "owner's runtime use survives CLI completion")

	fresh := namedRequest("fresh", "fresh")
	fresh.BrowserMode = "fresh"
	go func() { done <- e.Execute(t.Context(), fresh, nil) }()
	receive(t, owner.stopping)
	select {
	case <-launch:
		t.Fatal("fresh browser started before retirement receipt")
	default:
	}
	owner.receipt <- ndexec.TreeResult{Drained: false, Err: errors.New("missing receipt")}
	require.ErrorIs(t, receive(t, e.Fatal()), ndexec.ErrTreeNotDrained)
	require.False(t, receive(t, done).Drained)
	require.Equal(t, completion{2, false}, receive(t, finished))
	require.Equal(t, completion{3, true}, receive(t, finished), "unstarted attempt can release its independent use")
	assert.DirExists(t, owner.work)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(owner.work)) })
}

func TestBrowserPoolIdleExpiryAndShutdownJoin(t *testing.T) {
	e, _, _ := testEngine(t)
	e.idleFor = 25 * time.Millisecond
	owners := make(chan testBrowserOwner, 2)
	launch := make(chan wireRequest, 2)
	e.start = withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		assert.NoError(t, json.Unmarshal(raw, &req))
		launch <- req
		assert.NoError(t, emitBrowserReport(out, req.Browser, "unused"))
		return resultPayload(success())(ctx, args, raw, out, stderr)
	}), owners)
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(t.Context(), reuseRequest("one"), nil) }()
	first := receive(t, owners)
	require.Equal(t, first.id, receive(t, launch).Browser.OwnerID)
	require.Equal(t, synthetic.Success, receive(t, done).Run.Outcome)
	receive(t, first.stopping)
	go func() { done <- e.Execute(t.Context(), reuseRequest("two"), nil) }()
	queued(t, e, "two", 1)
	first.receipt <- ndexec.TreeResult{Drained: true}
	second := receive(t, owners)
	require.NotEqual(t, first.id, second.id)
	require.Equal(t, second.id, receive(t, launch).Browser.OwnerID)
	require.Equal(t, synthetic.Success, receive(t, done).Run.Outcome)

	var callers sync.WaitGroup
	closed := make(chan error, 2)
	for range 2 {
		callers.Go(func() { closed <- e.Close(t.Context()) })
	}
	receive(t, second.stopping)
	assert.DirExists(t, second.work)
	select {
	case <-closed:
		t.Fatal("shutdown returned before verified retirement")
	default:
	}
	second.receipt <- ndexec.TreeResult{Drained: true}
	require.NoError(t, receive(t, closed))
	require.NoError(t, receive(t, closed))
	callers.Wait()
	assert.NoDirExists(t, second.work)
}

func TestBrowserPoolCancellationRetiresEvenAfterCleanAcknowledgment(t *testing.T) {
	e, _, artifacts := testEngine(t)
	owners := make(chan testBrowserOwner, 1)
	clean := make(chan struct{})
	e.start = withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		assert.NoError(t, json.Unmarshal(raw, &req))
		assert.NoError(t, emitBrowserReport(out, req.Browser, "clean"))
		close(clean)
		<-ctx.Done()
		return resultPayload(success())(ctx, args, raw, out, stderr)
	}), owners)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(ctx, reuseRequest("cancel"), nil) }()
	owner := receive(t, owners)
	receive(t, clean)
	closed := make(chan error, 1)
	go func() { closed <- e.Close(t.Context()) }()
	cancel()
	receive(t, owner.stopping)
	require.Zero(t, artifacts.finalized.Load())
	select {
	case <-closed:
		t.Fatal("shutdown returned before the cancelled attempt and owner joined")
	default:
	}
	owner.receipt <- ndexec.TreeResult{Drained: true}
	result := receive(t, done)
	assert.True(t, result.Drained)
	assert.Equal(t, synthetic.Cancelled, result.Run.Outcome)
	require.NoError(t, receive(t, closed))
}

func TestBrowserPoolPreservesWebsiteFailureAfterRetirement(t *testing.T) {
	e, _, artifacts := testEngine(t)
	owners := make(chan testBrowserOwner, 1)
	e.start = withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		assert.NoError(t, json.Unmarshal(raw, &req))
		assert.NoError(t, emitBrowserReport(out, req.Browser, "retire"))
		resultPayload(reporterResult{
			Status:       synthetic.Failed,
			Error:        "assertion failed",
			Tests:        &synthetic.TestCounts{Declared: 2, Passed: 1, Failed: 1},
			CaptureState: "disabled",
		})(ctx, args, raw, out, stderr)
		return ndexec.TreeResult{Drained: true, Err: errors.New("exit status 1")}
	}), owners)
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(t.Context(), reuseRequest("failed"), nil) }()
	owner := receive(t, owners)
	receive(t, owner.stopping)
	require.Zero(t, artifacts.finalized.Load())
	owner.receipt <- ndexec.TreeResult{Drained: true}
	result := receive(t, done)
	require.True(t, result.Drained)
	require.Equal(t, synthetic.Failed, result.Run.Outcome)
	assert.Equal(t, "assertion failed", result.Run.Error)
	assert.Equal(t, 1, result.Run.Tests.Failed)
	require.NoError(t, e.Close(t.Context()))
}

func TestBrowserQualificationHasPrivateAffinityAndRequiresCleanLoan(t *testing.T) {
	e, _, artifacts := testEngineSlots(t, 2)
	owners := make(chan testBrowserOwner, 2)
	loans := make(chan browserLoan, 3)
	e.start = withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var req wireRequest
		assert.NoError(t, json.Unmarshal(raw, &req))
		loans <- *req.Browser
		state := "clean"
		if req.Script == "qualification" {
			state = "unused"
		}
		assert.NoError(t, emitBrowserReport(out, req.Browser, state))
		return resultPayload(success())(ctx, args, raw, out, stderr)
	}), owners)
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(t.Context(), reuseRequest("monitor"), nil) }()
	production := receive(t, owners)
	require.Equal(t, production.id, receive(t, loans).OwnerID)
	require.Equal(t, synthetic.Success, receive(t, done).Run.Outcome)

	candidate := e.unrecordedView(e.source, artifacts)
	candidate.browserSource.ephemeral = true
	go func() { done <- candidate.Execute(t.Context(), reuseRequest("qualification"), nil) }()
	ephemeral := receive(t, owners)
	require.NotEqual(t, production.id, ephemeral.id)
	require.Equal(t, ephemeral.id, receive(t, loans).OwnerID)
	receive(t, ephemeral.stopping)
	ephemeral.receipt <- ndexec.TreeResult{Drained: true}
	result := receive(t, done)
	require.True(t, result.Drained)
	require.Equal(t, synthetic.Error, result.Run.Outcome)
	assert.Contains(t, result.Run.Error, "qualification did not use the managed browser")

	rehearsed := make(chan error, 1)
	go func() { rehearsed <- e.Rehearse(t.Context(), reuseRequest("rehearsal")) }()
	require.Equal(t, production.id, receive(t, loans).OwnerID)
	require.NoError(t, receive(t, rehearsed))
	production.receipt <- ndexec.TreeResult{Drained: true}
	require.NoError(t, e.Close(t.Context()))
}
