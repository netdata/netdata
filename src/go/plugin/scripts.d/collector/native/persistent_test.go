// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readyLine = "printf '%s\\n' '{\"version\":\"v1\",\"ready\":true}'\n"

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
    nd_metric processed_total "$count" queue mail
    nd_metric depth "$$" queue mail
    nd_check backlog critical queue mail
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
			reply: `printf '%s\n' '{"id":"1","result":{"version":"v1","metrics":[{"name":"depth","value":17},{"name":"undeclared","value":0}]}}'`,
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
nd_metric depth 1
nd_end
printf '%s\n' '{"id":"1","result":{"version":"v1","metrics":[{"name":"depth","value":999}]}}'
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
