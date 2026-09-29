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
