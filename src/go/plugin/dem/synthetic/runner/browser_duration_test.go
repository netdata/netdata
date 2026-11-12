//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserDurationIncludesStartupAndRetirement(t *testing.T) {
	for _, phase := range []string{"startup", "retirement"} {
		t.Run(phase, func(t *testing.T) {
			e, _, _ := testEngine(t)
			owners := make(chan testBrowserOwner, 1)
			start := withBrowserOwners(t, starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
				var req wireRequest
				assert.NoError(t, json.Unmarshal(raw, &req))
				assert.NoError(t, emitBrowserReport(out, req.Browser, "retire"))
				return resultPayload(reporterResult{Status: synthetic.Error, Error: "cleanup failure", CaptureState: "disabled"})(ctx, args, raw, out, stderr)
			}), owners)
			var startupDelay time.Duration
			e.start = func(ctx context.Context, options ndexec.TreeOptions, bin string, args ...string) (treeProcess, error) {
				if phase == "startup" && slices.Contains(args, "--browser-owner") {
					begin := time.Now()
					time.Sleep(25 * time.Millisecond)
					startupDelay = time.Since(begin)
				}
				return start(ctx, options, bin, args...)
			}
			done := make(chan synthetic.Execution, 1)
			go func() { done <- e.Execute(t.Context(), reuseRequest("duration"), nil) }()
			owner := receive(t, owners)
			receive(t, owner.stopping)
			var retirementDelay time.Duration
			if phase == "retirement" {
				begin := time.Now()
				time.Sleep(25 * time.Millisecond)
				retirementDelay = time.Since(begin)
			}
			owner.receipt <- ndexec.TreeResult{Drained: true}
			result := receive(t, done)
			require.True(t, result.Drained)
			require.NotNil(t, result.Run.DurationMS)
			assert.GreaterOrEqual(t, *result.Run.DurationMS, float64(startupDelay+retirementDelay)/float64(time.Millisecond))
			require.NoError(t, e.Close(t.Context()))
		})
	}
}
