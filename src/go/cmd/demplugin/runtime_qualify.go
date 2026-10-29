// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type runtimeQualification struct {
	ExecutionContract        bool                       `json:"execution_contract"`
	SandboxedLocalLighthouse bool                       `json:"sandboxed_local_lighthouse"`
	Runtime                  *synthetic.RuntimeIdentity `json:"runtime,omitempty"`
	SandboxedLocalJourney    bool                       `json:"sandboxed_local_journey"`
	ProcessTreeDrained       bool                       `json:"process_tree_drained"`
	Scope                    string                     `json:"scope"`
}

// Qualification deliberately uses the normal adapter, ownership, sandbox and
// process supervisor. It proves this invocation's environment, not daemon policy.
func qualifyRuntime(ctx context.Context, source runner.Source) (_ *runtimeQualification, retErr error) {
	result := &runtimeQualification{Scope: fmt.Sprintf("Current CLI uid=%d gid=%d and environment: release contract check, sandboxed local Playwright journey and desktop Lighthouse audit on loopback content; does not certify daemon confinement or other hosts/platforms.", os.Geteuid(), os.Getegid())}
	check := runner.New(source, nil, nil)
	if err := check.Check(ctx, synthetic.Journey); err != nil {
		return result, err
	}
	result.ExecutionContract = true
	captures, err := artifacts.Open("")
	if err != nil {
		return result, err
	}
	drained := true
	defer func() {
		if drained {
			retErr = errors.Join(retErr, captures.Close())
		}
	}()
	engine := runner.New(source, nil, captures)
	execution := engine.Execute(ctx, synthetic.Request{Kind: synthetic.Journey, Name: "runtime-qualification", Timeout: 30 * time.Second,
		Script: `import { test, expect } from '@playwright/test';
 test('sandboxed local page', async ({ page }) => {
   await page.setContent('<title>DEM runtime qualification</title><h1>ready</h1>');
   await expect(page).toHaveTitle('DEM runtime qualification');
   await expect(page.locator('h1')).toHaveText('ready');
 });`, Capture: false}, nil)
	drained = execution.Drained
	result.ProcessTreeDrained = drained
	if !drained {
		return result, ndexec.ErrTreeNotDrained
	}
	if execution.Run.Outcome != synthetic.Success {
		return result, fmt.Errorf("local browser qualification %s: %s", execution.Run.Outcome, execution.Run.Error)
	}
	if execution.Run.Tests == nil || execution.Run.Tests.Passed != 1 {
		return result, errors.New("local browser qualification did not report exactly one passing test")
	}
	result.SandboxedLocalJourney = true
	result.Runtime = execution.Run.Runtime
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html lang=en><head><title>DEM runtime qualification</title><meta name=viewport content='width=device-width,initial-scale=1'></head><body><h1>ready</h1></body></html>")
	}))
	defer server.Close()
	audit := engine.Execute(ctx, synthetic.Request{Kind: synthetic.Lighthouse, Name: "runtime-qualification-audit", URL: server.URL, Timeout: 90 * time.Second, Capture: false}, nil)
	drained = audit.Drained
	result.ProcessTreeDrained = drained
	if !drained {
		return result, ndexec.ErrTreeNotDrained
	}
	if audit.Run.Outcome != synthetic.Success || audit.Run.Metrics == nil {
		return result, fmt.Errorf("local Lighthouse qualification %s: %s", audit.Run.Outcome, audit.Run.Error)
	}
	if result.Runtime == nil || audit.Run.Runtime == nil || *result.Runtime != *audit.Run.Runtime {
		return result, errors.New("runtime identity changed during qualification")
	}
	result.SandboxedLocalLighthouse = true
	return result, nil
}
