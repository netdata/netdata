// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type Qualification struct {
	ExecutionContract        bool                       `json:"execution_contract"`
	SandboxedLocalLighthouse bool                       `json:"sandboxed_local_lighthouse"`
	Runtime                  *synthetic.RuntimeIdentity `json:"runtime,omitempty"`
	SandboxedLocalJourney    bool                       `json:"sandboxed_local_journey"`
	ProcessTreeDrained       bool                       `json:"process_tree_drained"`
	Scope                    string                     `json:"scope"`
}

// Qualify checks a candidate using this process's existing admission and fatal
// owner. It never replaces the monitoring source or appends monitor history.
func (e *Engine) Qualify(ctx context.Context, source Source) (*Qualification, error) {
	captures, err := artifacts.Open("")
	if err != nil {
		return nil, err
	}
	result, err := qualify(ctx, e.qualificationView(source, captures))
	if !errors.Is(err, ndexec.ErrTreeNotDrained) {
		err = errors.Join(err, captures.Close())
	}
	return result, err
}

type qualificationExecutor interface {
	Check(context.Context, synthetic.Kind) error
	Execute(context.Context, synthetic.Request, func(string)) synthetic.Execution
}

// Qualification exercises the normal adapter and sandbox in the caller's
// environment; it does not certify another daemon's security policy.
func qualify(ctx context.Context, engine qualificationExecutor) (*Qualification, error) {
	result := &Qualification{Scope: fmt.Sprintf("Current process uid=%d gid=%d and environment: release contract check, sandboxed local Playwright journey and desktop Lighthouse audit on loopback content; does not certify other process confinement or hosts/platforms.", os.Geteuid(), os.Getegid())}
	if err := engine.Check(ctx, synthetic.Journey); err != nil {
		return result, err
	}
	result.ExecutionContract = true
	execution := engine.Execute(ctx, synthetic.Request{Kind: synthetic.Journey, Name: "runtime-qualification", Timeout: 30 * time.Second,
		Script: `import { test, expect } from '@playwright/test';
 test('sandboxed local page', async ({ page }) => {
   await page.setContent('<title>DEM runtime qualification</title><h1>ready</h1>');
   await expect(page).toHaveTitle('DEM runtime qualification');
   await expect(page.locator('h1')).toHaveText('ready');
 });`, Capture: false}, nil)
	drained := execution.Drained
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return result, fmt.Errorf("listen for local runtime qualification: %w", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html lang=en><head><title>DEM runtime qualification</title><meta name=viewport content='width=device-width,initial-scale=1'></head><body><h1>ready</h1></body></html>")
	})}
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
		<-served
	}()
	audit := engine.Execute(ctx, synthetic.Request{Kind: synthetic.Lighthouse, Name: "runtime-qualification-audit", URL: "http://" + listener.Addr().String(), Timeout: 90 * time.Second, Capture: false}, nil)
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
