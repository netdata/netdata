// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package native

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunOneshot_CancellationContainsDescendant(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(
		t,
		"dir=$(dirname \"$0\")\n(sleep 1; printf escaped > \"$dir/escaped\") &\nprintf started > \"$dir/started\"\nwait\n",
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runOneshot(ctx, c.definition.command, "collect", nil); done <- err }()
	waitFile(t, filepath.Join(dir, "started"))
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("command did not stop")
	}
	// The descendant has passed its scheduled write time. A surviving child would
	// create this marker even after the parent and command's pipes were closed.
	time.Sleep(1100 * time.Millisecond)
	assert.False(t, fileExists(filepath.Join(dir, "escaped")), "descendant survived cancellation")
}

func TestRunDescribe_ContainsDescendants(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		callerCancels bool
	}{
		"leader exits":   {callerCancels: false},
		"caller cancels": {callerCancels: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			body := `dir=$(dirname "$0")
(sleep 1; printf escaped > "$dir/escaped") &
printf started > "$dir/started"
printf '%s\n' 'version: v1' 'checks: [{id: ready, title: Ready}]'
`
			if tc.callerCancels {
				body += "wait\n"
			}
			c, dir := fixtureCollector(t, body)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := runDescribe(ctx, c.definition.command); done <- err }()
			waitFile(t, filepath.Join(dir, "started"))
			if tc.callerCancels {
				cancel()
			}
			select {
			case err := <-done:
				if tc.callerCancels {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.NoError(t, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("description process did not finish")
			}
			time.Sleep(1100 * time.Millisecond)
			assert.False(
				t,
				fileExists(filepath.Join(dir, "escaped")),
				"contained descendant must not outlive description",
			)
		})
	}
}

// A normally exiting script still owns its descendants: they are terminated
// when it exits, and one holding stdio does not turn valid output into a failure.
func TestRunOneshot_LeaderExitContainsDescendants(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		child string
	}{
		"child holds stdio":         {child: `(sleep 1; printf escaped > "$dir/escaped") &`},
		"child detached from stdio": {child: `(sleep 1; printf escaped > "$dir/escaped") >/dev/null 2>&1 </dev/null &`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, dir := fixtureCollector(t, "dir=$(dirname \"$0\")\n"+tc.child+"\nprintf done\n")
			data, err := runOneshot(context.Background(), c.definition.command, opCollect, nil)
			require.NoError(t, err)
			assert.Equal(t, "done", string(data))
			time.Sleep(1100 * time.Millisecond)
			assert.False(t, fileExists(filepath.Join(dir, "escaped")), "descendant survived leader exit")
		})
	}
}
