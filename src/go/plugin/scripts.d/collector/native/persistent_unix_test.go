// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package native

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistent_ContainsDescendantAfterParentExit(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		afterReady bool
	}{
		"startup": {afterReady: false},
		"idle":    {afterReady: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			body := `dir=$(dirname "$0")
(sleep 2; printf escaped > "$dir/escaped") &
printf started > "$dir/started"
`
			if tc.afterReady {
				body += bashHelper(t) + "nd_ready\nnd_next\nnd_begin\nnd_end\n"
			}
			body += "exit 0\n"
			c, dir := persistentCollector(t, body)
			r := startRuntime(t, c)
			if tc.afterReady {
				r.waitReady(t)
				// Either the final reply or the observed process exit may win.
				_, _ = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			}
			r.wait(t)
			require.Error(t, r.err)
			require.True(t, fileExists(filepath.Join(dir, "started")))
			time.Sleep(2100 * time.Millisecond)
			assert.False(t, fileExists(filepath.Join(dir, "escaped")), "descendant survived parent exit")
		})
	}
}

func TestPersistent_StopAfterReadiness(t *testing.T) {
	setupRunner(t)
	for range 10 {
		c, dir := persistentCollector(t, bashHelper(t)+`
printf '%s' "$$" > "$(dirname "$0")/pid"
nd_ready
while nd_next; do nd_begin; nd_end; done
`)
		ctx, cancel := context.WithCancel(context.Background())
		err := c.Run(ctx, func() {
			data, err := os.ReadFile(filepath.Join(dir, "pid"))
			require.NoError(t, err)
			pid, err := strconv.Atoi(string(data))
			require.NoError(t, err)
			cancel()
			// Model a scheduler pause after readiness: cancellation and the reaped
			// process are both observable before Run resumes its idle select.
			require.Eventually(
				t,
				func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) },
				time.Second,
				time.Millisecond,
			)
		})
		cancel()
		require.ErrorIs(t, err, context.Canceled)
	}
}
