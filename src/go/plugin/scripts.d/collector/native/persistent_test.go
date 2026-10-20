// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistent_StateAndTransientFailure(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
printf '%s' "$$" > "$(dirname "$0")/pid"
nd_ready
count=0
while nd_next; do
    count=$((count+1))
    if [[ $count == 2 ]]; then nd_fail; continue; fi
    nd_begin
    nd_metric processed_total counter jobs
    nd_sample "$ND_FAMILY" "$count" queue mail
    nd_metric depth gauge jobs
    nd_sample "$ND_FAMILY" "$$" queue mail
    nd_check backlog "Queue Backlog" queue
    nd_check_sample "$ND_FAMILY" critical queue mail
    nd_end
done
`)
	requireNoFile(t, filepath.Join(dir, "pid"))
	r := startRuntime(t, c)
	r.waitReady(t)
	first, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(1), first[`processed_total{queue="mail"}`])
	assert.Equal(t, float64(1), first[`native.check.backlog{native.check.backlog="critical",queue="mail"}`])
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.ErrorIs(t, err, errCollectionFailed)
	third, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(3), third[`processed_total{queue="mail"}`])
	assert.Equal(t, first[`depth{queue="mail"}`], third[`depth{queue="mail"}`], "same process after transient failure")
	r.cancel()
	r.wait(t)
	require.ErrorIs(t, r.err, context.Canceled)
	// A new generation cannot reuse its predecessor's state or closed channels.
	r2 := startRuntime(t, c)
	r2.waitReady(t)
	restarted, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, float64(1), restarted[`processed_total{queue="mail"}`])
	assert.NotEqual(t, first[`depth{queue="mail"}`], restarted[`depth{queue="mail"}`])
}

func TestPersistent_StartupFailure(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		body    string
		timeout time.Duration
		wantErr error // optional specific cause
	}{
		"no ready":         {body: "sleep 30\n"},
		"partial ready":    {body: `printf '{"version":"v1","ready":true}'; sleep 30`},
		"invalid ready":    {body: `printf '%s\n' '{"version":"v1","ready":false}'; sleep 30`},
		"early exit":       {body: "exit 0\n"},
		"eof without exit": {body: "exec 1>&-\nsleep 30\n"},
		"oversize": {
			body:    fmt.Sprintf("head -c %d /dev/zero; printf '\\n'; sleep 30\n", maxMessageBytes),
			timeout: 5 * time.Second,
			wantErr: errResponseTooLarge,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := persistentCollector(t, tc.body)
			c.Timeout = confopt.Duration(time.Second)
			if tc.timeout > 0 {
				c.Timeout = confopt.Duration(tc.timeout)
			}
			r := startRuntime(t, c)
			r.wait(t)
			require.Error(t, r.err)
			if tc.wantErr != nil {
				require.ErrorIs(t, r.err, tc.wantErr)
			}
			select {
			case <-r.ready:
				t.Fatal("invalid startup signaled readiness")
			default:
			}
		})
	}
}

func TestPersistent_TerminalReply(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		reply   string
		timeout time.Duration
		wantErr error // optional specific cause
	}{
		"wrong id": {reply: `printf '%s\n' '{"id":"2","result":{"version":"v1"}}'`},
		"invalid batch": {
			reply: `printf '%s\n' '{"id":"1","result":{"version":"v1","metrics":[{"name":"depth","samples":[{"value":17}]},{"name":"invalid","samples":[{"value":null}]}]}}'`,
		},
		"unknown error": {reply: `printf '%s\n' '{"id":"1","error":"secret diagnostic"}'`},
		"partial":       {reply: `printf '%s' '{"id":"1","result":{"version":"v1"}}'`},
		"timeout":       {reply: ":"},
		"exit":          {reply: "exit 1"},
		"oversize": {
			reply:   fmt.Sprintf("head -c %d /dev/zero; printf '\\n'", maxMessageBytes),
			timeout: 5 * time.Second,
			wantErr: errResponseTooLarge,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := persistentCollector(t, readyLine+"read -r request\n"+tc.reply+"\nsleep 30\n")
			c.Timeout = confopt.Duration(time.Second)
			if tc.timeout > 0 {
				c.Timeout = confopt.Duration(tc.timeout)
			}
			r := startRuntime(t, c)
			r.waitReady(t)
			err := collectAndCommit(t, c)
			require.Error(t, err)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			assert.Empty(t, rawSeries(c), "a failed exchange stages no sample")
			r.wait(t)
			require.Error(t, r.err, "a terminal reply stops the session")
			assert.NotContains(t, r.err.Error(), "secret diagnostic")
		})
	}
}

func TestPersistent_DuplicateReplyIsTerminal(t *testing.T) {
	setupRunner(t)
	c, _ := persistentCollector(t, bashHelper(t)+`
nd_ready
nd_next
nd_begin
nd_metric depth gauge jobs
nd_sample "$ND_FAMILY" 1
nd_end
printf '%s\n' '{"id":"1","result":{"version":"v1","metrics":[{"name":"depth","samples":[{"value":999}]}]}}'
sleep 30
`)
	r := startRuntime(t, c)
	r.waitReady(t)
	first, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	// Run may already have failed by the time the caller receives its first reply.
	if err == nil {
		assert.Equal(t, float64(1), first["depth"])
	}
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.Error(t, err)
	r.wait(t)
	require.Error(t, r.err)
}

func TestPersistent_CancelInFlight(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
nd_ready
nd_next
printf started > "$(dirname "$0")/started"
sleep 30
`)
	c.Timeout = confopt.Duration(500 * time.Millisecond)
	r := startRuntime(t, c)
	r.waitReady(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collected := make(chan error, 1)
	go func() { collected <- c.Collect(ctx) }()
	waitFile(t, filepath.Join(dir, "started"))
	cancel()
	select {
	case err := <-collected:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Collect ignored cancellation")
	}
	r.wait(t)
	require.ErrorIs(t, r.err, context.DeadlineExceeded, "the admitted exchange keeps its full timeout")
}

func TestPersistent_ConfigurationWriteTimeout(t *testing.T) {
	setupRunner(t)
	registry, _ := configuredFixture(t, "sleep 30\n", modePersistent)
	c := registry["native-fixture"].CreateV2().(*Collector)
	c.ScriptConfig = Settings{
		"text": strings.Repeat("x", 512*1024),
	}
	c.Timeout = confopt.Duration(150 * time.Millisecond)
	require.NoError(t, c.Init(context.Background()))
	start := time.Now()
	err := c.Run(context.Background(), func() { t.Error("readiness before config consumed") })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestPersistent_FunctionQueue(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, false)
	c := initFunctionCollector(t, registry)
	startRuntime(t, c).waitReady(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	active := make(chan error, 1)
	go func() {
		_, err := c.executeFunction(ctx, funcapi.RawMethodRequest{
			Method: "items",
			Args:   []string{"wait"},
		})
		active <- err
	}()
	waitFile(t, filepath.Join(dir, "23.active"))
	queued, queuedCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer queuedCancel()
	_, err := c.executeFunction(queued, funcapi.RawMethodRequest{
		Method: "items",
		Args:   []string{"must-not-reach-peer"},
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// Collection's deadline includes time spent waiting behind an interactive call.
	_, err = c.collectPersistent(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	select {
	case err := <-active:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("active call stuck")
	}
	_, err = c.collectPersistent(context.Background())
	require.NoError(t, err)
	requests := readLines(t, filepath.Join(dir, "requests"))
	assert.NotContains(t, strings.Join(requests, "\n"), "must-not-reach-peer")
	assert.Len(t, requests, 2)
}

func TestPersistent_CanceledRequestAtDequeue(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, true)
	c := initFunctionCollector(t, registry)
	startRuntime(t, c).waitReady(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Exercise the actual receive boundary deterministically: a select may deliver
	// a sender whose context is already canceled. No synthetic runtime is installed.
	c.runtimeMu.Lock()
	r := c.runtime
	c.runtimeMu.Unlock()
	request := scriptRequest{
		ctx: ctx,
		function: &funcapi.RawMethodRequest{
			Method: "items",
		},
		reply: make(chan scriptResult, 1),
	}
	select {
	case r.requests <- request:
	case <-time.After(time.Second):
		t.Fatal("request not received")
	}
	select {
	case result := <-request.reply:
		require.ErrorIs(t, result.err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("request not rejected")
	}
	_, err := c.executeFunction(context.Background(), funcapi.RawMethodRequest{
		Method: "items",
	})
	require.NoError(t, err)
	assert.Len(t, readLines(t, filepath.Join(dir, "requests")), 1)
}

func TestPersistent_MalformedFunctionReplyStopsSession(t *testing.T) {
	setupRunner(t)
	registry, _ := functionFixture(t, modePersistent, true)
	c := initFunctionCollector(t, registry)
	r := startRuntime(t, c)
	r.waitReady(t)
	_, err := c.executeFunction(context.Background(), funcapi.RawMethodRequest{
		Method: "items",
		Args:   []string{"malformed"},
	})
	require.Error(t, err)
	r.wait(t)
	require.Error(t, r.err)
}

func TestPersistent_CollectionRecoversQueueBudget(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, false)
	peer := strings.Replace(functionPeer, `if req["method"] == "collect":`, `if req["method"] == "collect":
        time.sleep(0.5)`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
	c := initFunctionCollector(t, registry)
	c.Timeout = confopt.Duration(time.Second)
	startRuntime(t, c).waitReady(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	active := make(chan error, 1)
	go func() {
		_, err := c.executeFunction(ctx, funcapi.RawMethodRequest{
			Method: "items",
			Args:   []string{"wait"},
		})
		active <- err
	}()
	waitFile(t, filepath.Join(dir, "23.active"))
	collected := make(chan error, 1)
	go func() { collected <- c.Collect(ctx) }()
	// Leave less than the peer's 500ms operation time in the caller budget.
	time.Sleep(700 * time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	require.NoError(t, <-active)
	require.ErrorIs(t, <-collected, context.DeadlineExceeded)
	requests := strings.Join(readLines(t, filepath.Join(dir, "requests")), "\n")
	require.Contains(t, requests, `"method": "collect"`, "collection must reach the peer before its caller times out")
	_, err := c.executeFunction(ctx, funcapi.RawMethodRequest{
		Method: "items",
	})
	require.NoError(t, err, "drain the timed-out collection before the next reply")
	samples, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.NotEmpty(t, samples, "collection must still publish after recovery")
}

// A caller that stops waiting leaves the peer's owed reply to be drained; the
// session survives and the late reply is never delivered to another caller.
func TestPersistent_FunctionCallerStopRecovery(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		deadline bool // caller deadline instead of explicit cancellation
		wantErr  error
	}{
		"cancel":   {deadline: false, wantErr: context.Canceled},
		"deadline": {deadline: true, wantErr: context.DeadlineExceeded},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry, dir := functionFixture(t, modePersistent, false)
			c := initFunctionCollector(t, registry)
			startRuntime(t, c).waitReady(t)
			ctx, cancel := context.WithCancel(context.Background())
			if tc.deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.executeFunction(ctx, funcapi.RawMethodRequest{
					Method: "items",
					Args:   []string{"wait"},
				})
				done <- err
			}()
			waitFile(t, filepath.Join(dir, "23.active"))
			if !tc.deadline {
				cancel()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, tc.wantErr)
			case <-time.After(time.Second):
				t.Fatal("caller kept waiting for recovery")
			}
			// The caller has already returned while the peer still owes its reply.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
			nextCtx, nextCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer nextCancel()
			result, err := c.executeFunction(nextCtx, funcapi.RawMethodRequest{
				Method: "items",
				Info:   true,
			})
			require.NoError(t, err)
			assert.Nil(t, result.Data, "the canceled request's data must not become the info reply")
			_, err = c.collectPersistent(nextCtx)
			require.NoError(t, err)
			assert.Len(t, readLines(t, filepath.Join(dir, "starts")), 1, "recovery reuses the peer")
		})
	}
}

func TestPersistent_FunctionCallerStopRecoveryFailure(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		releaseReply bool // false: the peer never replies within the drain budget
		args         []string
		wantErrIs    error
		wantErrText  string
	}{
		"missing reply": {
			args:      []string{"wait"},
			wantErrIs: context.DeadlineExceeded,
		},
		"invalid reply": {
			releaseReply: true,
			args:         []string{"wait", "malformed"},
			wantErrText:  "version",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry, dir := functionFixture(t, modePersistent, true)
			c := initFunctionCollector(t, registry)
			r := startRuntime(t, c)
			r.waitReady(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.executeFunction(ctx, funcapi.RawMethodRequest{
					Method: "items",
					Args:   tc.args,
				})
				done <- err
			}()
			waitFile(t, filepath.Join(dir, "23.active"))
			cancel()
			require.ErrorIs(t, <-done, context.Canceled)
			if tc.releaseReply {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
			}
			r.wait(t)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, r.err, tc.wantErrIs)
			}
			if tc.wantErrText != "" {
				require.ErrorContains(t, r.err, tc.wantErrText)
			}
		})
	}
}
