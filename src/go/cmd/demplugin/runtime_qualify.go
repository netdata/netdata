// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type runtimeQualification struct {
	VersionsAndAssets     bool   `json:"versions_and_assets"`
	SandboxedLocalJourney bool   `json:"sandboxed_local_journey"`
	ProcessTreeDrained    bool   `json:"process_tree_drained"`
	Scope                 string `json:"scope"`
}

// Qualification deliberately uses the normal adapter, ownership, sandbox and
// process supervisor. It proves this invocation's environment, not daemon policy.
func qualifyRuntime(ctx context.Context, assetsPath string, source runner.Source) (_ *runtimeQualification, retErr error) {
	result := &runtimeQualification{Scope: "Current CLI user/environment: version and asset checks plus one local Playwright page; no network target, Lighthouse audit, daemon sandbox, or cross-platform certification."}
	check := runner.New(assetsPath, source, nil, nil)
	if err := check.Check(ctx, synthetic.Journey); err != nil {
		return result, err
	}
	result.VersionsAndAssets = true
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
	engine := runner.New(assetsPath, source, nil, captures)
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
	return result, nil
}
