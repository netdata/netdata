//go:build linux

// SPDX-License-Identifier: GPL-3.0-or-later

// Build this file from src/go. Run as the unprivileged service identity
// against an isolated store with a selected contract-2 runtime.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type countedSource struct {
	runner.Source
	uses atomic.Int64
}

func (s *countedSource) BeginUse() (runner.Use, error) {
	use, err := s.Source.BeginUse()
	if err == nil {
		s.uses.Add(1)
	}
	return use, err
}

type fixture struct {
	store    *runtime.Store
	source   *countedSource
	history  *history.Store
	captures *artifacts.Store
	identity synthetic.RuntimeIdentity
	origin   string
	hits     chan string
	work     string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (result error) {
	storePath := flag.String("store", "", "isolated managed store with a selected contract-2 runtime")
	helper := flag.String("helper", "", "installed nd-run executable")
	work := flag.String("work", "", "new output directory for history and captures")
	mode := flag.String("mode", "proof", "proof, fresh workload, or reuse workload")
	journeys := flag.Int("journeys", 300, "journey workload count")
	lighthouse := flag.Int("lighthouse", 100, "Lighthouse workload count")
	slots := flag.Int("slots", 12, "workload admission slots")
	repeat := flag.Int("repeat", 3, "proof clean/retire cycles")
	window := flag.Duration("paced", 0, "paced workload window; zero starts all checks at one barrier")
	journeyEvery := flag.Duration("journey-every", 5*time.Minute, "paced journey interval")
	auditEvery := flag.Duration("lighthouse-every", 15*time.Minute, "paced Lighthouse interval")
	idle := flag.Duration("idle-after", 0, "observe idle owners after the workload window and all completions")
	timeout := flag.Duration("timeout", 20*time.Minute, "whole proof or workload deadline")
	flag.Parse()
	for _, path := range []string{*storePath, *helper, *work} {
		if !filepath.IsAbs(path) {
			return errors.New("store, helper and work paths must be absolute")
		}
	}
	if os.Geteuid() == 0 {
		return errors.New("run as the unprivileged runtime service identity")
	}
	if *timeout <= 0 || *slots <= 0 || *repeat <= 0 || *journeys < 0 || *lighthouse < 0 ||
		(*mode != "proof" && *mode != "reuse" && *mode != "fresh") {
		return errors.New("invalid proof or workload options")
	}
	options := workloadOptions{window: *window, journeyEvery: *journeyEvery, auditEvery: *auditEvery, idle: *idle}
	if err := options.validate(); err != nil {
		return err
	}
	if err := os.Mkdir(*work, 0700); err != nil {
		return err
	}
	restore := ndexec.SetRunnerPathsForTests(*helper, "")
	defer restore()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	store, err := runtime.Open(runtime.DefaultConfig(*storePath))
	if err != nil {
		return err
	}
	defer closeVerified(&result, store.Close)
	status, err := store.Inspect()
	if err != nil {
		return err
	}
	if status.ProcessClaims != 0 || status.CandidateClaims != 0 || status.ExecutionPins != 0 || status.UncertainPins != 0 {
		return errors.New("proof requires an idle, isolated runtime store")
	}
	consumer, err := store.AcquireSelected()
	if err != nil {
		return err
	}
	defer closeVerified(&result, consumer.Close)
	journal, err := demjournal.Open(ctx, filepath.Join(*work, "history"))
	if err != nil {
		return err
	}
	defer closeVerified(&result, journal.Close)
	captures, err := artifacts.Open(filepath.Join(*work, "captures"))
	if err != nil {
		return err
	}
	defer closeVerified(&result, captures.Close)
	manifest := consumer.Manifest()
	f := fixture{
		store:    store,
		source:   &countedSource{Source: runtimehost.Borrow(consumer)},
		history:  history.NewStore(journal),
		captures: captures,
		identity: synthetic.RuntimeIdentity{Version: manifest.Version, SHA256: manifest.Archive.SHA256, Contract: manifest.Contract},
		hits:     make(chan string, 4),
		work:     *work,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/cancel" || req.URL.Path == "/timeout" {
			select {
			case f.hits <- req.URL.Path:
			case <-ctx.Done():
				return
			}
			select {
			case <-req.Context().Done():
			case <-ctx.Done():
			}
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<!doctype html><html lang=en><head><title>DEM engine proof</title><meta name=viewport content='width=device-width,initial-scale=1'></head><body><h1>Ready</h1></body></html>")
	}))
	defer func() {
		cancel()
		server.Close()
	}()
	f.origin = server.URL
	if *mode != "proof" {
		return f.workload(ctx, *mode, *slots, *journeys, *lighthouse, options)
	}
	start := time.Now()
	if err := f.serialProof(ctx, *repeat); err != nil {
		return err
	}
	if err := f.peerProof(ctx); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"mode": "proof", "runtime": f.identity, "cycles": *repeat, "seconds": time.Since(start).Seconds(),
		"verified": []string{"warm reuse", "same-digest qualification", "fresh handoff", "history", "screenshot",
			"guard retirement", "timeout", "cancellation", "peer progress", "shutdown pins"},
	})
}

func closeVerified(result *error, close func() error) {
	if !errors.Is(*result, ndexec.ErrTreeNotDrained) {
		*result = errors.Join(*result, close())
	}
}

func (f fixture) finish(engine *runner.Engine, result error) error {
	if errors.Is(result, ndexec.ErrTreeNotDrained) {
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := engine.Close(ctx); err != nil {
		return errors.Join(result, err)
	}
	return errors.Join(result, f.pins(0))
}

func (f fixture) journey(name, mode, body string) synthetic.Request {
	return synthetic.Request{Kind: synthetic.Journey, Name: name, BrowserMode: mode, Timeout: 45 * time.Second,
		Script: "import {test,expect} from '@playwright/test';\n" + body}
}

func (f fixture) visit(name, mode, route string) synthetic.Request {
	return f.journey(name, mode, fmt.Sprintf(
		"test('visit',async({page})=>{await page.goto(%q);await expect(page.locator('h1')).toHaveText('Ready');});", f.origin+route))
}

func (f fixture) pins(expected int) error {
	status, err := f.store.Inspect()
	if err != nil {
		return err
	}
	if status.ExecutionPins != expected || status.UncertainPins != 0 {
		return fmt.Errorf("expected %d execution pins and no uncertain pins, observed %d and %d", expected, status.ExecutionPins, status.UncertainPins)
	}
	return nil
}

func (f fixture) verify(ctx context.Context, execution synthetic.Execution, want synthetic.Outcome) error {
	run := execution.Run
	if !execution.Drained {
		return fmt.Errorf("%s: %w: %s", run.Name, ndexec.ErrTreeNotDrained, run.Error)
	}
	if run.Outcome != want || run.Runtime == nil || *run.Runtime != f.identity || run.HistoryError != "" {
		return fmt.Errorf("%s: expected %s and recorded identity, got %s (%s, history %s)",
			run.Name, want, run.Outcome, run.Error, run.HistoryError)
	}
	retained, err := f.history.GetRun(ctx, run.JobID, run.ID)
	if err != nil {
		return err
	}
	if retained.CompletedUS != run.CompletedUS || retained.Outcome != want ||
		retained.Runtime == nil || *retained.Runtime != f.identity {
		return fmt.Errorf("%s: retained outcome or identity differs from verified completion", run.Name)
	}
	return nil
}

func (f fixture) serialProof(ctx context.Context, repeats int) (result error) {
	engine, err := runner.New(f.source, f.history, f.captures, 1)
	if err != nil {
		return err
	}
	defer func() { result = f.finish(engine, result) }()
	run := func(request synthetic.Request, expected synthetic.Outcome) error {
		return f.verify(ctx, engine.Execute(ctx, request, nil), expected)
	}
	if err := engine.Check(ctx, synthetic.Journey); err != nil {
		return err
	}
	initial := f.source.uses.Load()
	for _, name := range []string{"first", "second"} {
		if err := run(f.visit(name, "reuse", "/"), synthetic.Success); err != nil {
			return err
		}
	}
	if got := f.source.uses.Load() - initial; got != 3 {
		return fmt.Errorf("expected one owner and two CLI runtime uses, got %d", got)
	}
	if err := f.pins(1); err != nil {
		return err
	}
	if err := f.qualifySameDigest(ctx, engine); err != nil {
		return err
	}
	if err := f.pins(0); err != nil {
		return err
	}

	for i := range repeats {
		if err := run(f.visit(fmt.Sprintf("warm-%d", i), "reuse", "/"), synthetic.Success); err != nil {
			return err
		}
		if err := f.pins(1); err != nil {
			return err
		}
		denied := f.journey(fmt.Sprintf("retire-%d", i), "reuse",
			"test('guard',async({browser})=>{try { await browser.newBrowserCDPSession(); } catch {} });")
		if err := run(denied, synthetic.Error); err != nil {
			return err
		}
		if err := f.pins(0); err != nil {
			return err
		}
	}
	both := f.journey("failure-and-retirement", "reuse",
		"test('failure then guard',async({browser})=>{expect.soft(1).toBe(2);try { await browser.newBrowserCDPSession(); } catch {} });")
	if err := run(both, synthetic.Failed); err != nil {
		return err
	}
	if err := f.pins(0); err != nil {
		return err
	}
	failure := f.journey("screenshot", "reuse", "test('failure',async({page})=>{await page.setContent('<h1>Expected evidence</h1>');expect(1).toBe(2);});")
	failure.Capture = true
	execution := engine.Execute(ctx, failure, nil)
	if err := f.verify(ctx, execution, synthetic.Failed); err != nil {
		return err
	}
	if execution.Run.CaptureState != "available" || len(execution.Run.Artifacts) != 1 {
		return errors.New("verified website failure did not publish its screenshot")
	}
	runResult := execution.Run
	_, png, err := f.captures.Fetch(ctx, runResult.JobID, runResult.ID, runResult.Artifacts[0].ID)
	if err != nil {
		return err
	}
	if len(png) < 8 || string(png[:8]) != "\x89PNG\r\n\x1a\n" {
		return errors.New("published screenshot does not contain PNG data")
	}
	if err := f.pins(1); err != nil {
		return err
	}
	fresh := f.journey("fresh-cdp", "fresh", "test('CDP',async({browser})=>{const cdp=await browser.newBrowserCDPSession();await cdp.detach();});")
	if err := run(fresh, synthetic.Success); err != nil {
		return err
	}
	if err := f.pins(0); err != nil {
		return err
	}
	if err := run(f.visit("warm-before-audit", "reuse", "/"), synthetic.Success); err != nil {
		return err
	}
	if err := f.pins(1); err != nil {
		return err
	}
	audit := synthetic.Request{Kind: synthetic.Lighthouse, Name: "lighthouse-handoff", URL: f.origin, Timeout: 90 * time.Second}
	if err := run(audit, synthetic.Success); err != nil {
		return err
	}
	return f.pins(0)
}

func (f fixture) qualifySameDigest(ctx context.Context, engine *runner.Engine) (result error) {
	candidate, err := f.store.AcquireSelected()
	if err != nil {
		return err
	}
	defer closeVerified(&result, candidate.Close)
	source := &countedSource{Source: runtimehost.Borrow(candidate)}
	selectedUses := f.source.uses.Load()
	qualified, err := engine.Qualify(ctx, source)
	if err != nil {
		return err
	}
	if qualified.Runtime == nil || *qualified.Runtime != f.identity || !qualified.ProcessTreeDrained ||
		!qualified.SandboxedLocalJourney || !qualified.SandboxedLocalLighthouse ||
		source.uses.Load() != 4 || f.source.uses.Load() != selectedUses {
		return errors.New("same-digest qualification did not own a separate probe, browser and two CLI uses")
	}
	return nil
}

func (f fixture) peerProof(ctx context.Context) (result error) {
	engine, err := runner.New(f.source, f.history, f.captures, 2)
	if err != nil {
		return err
	}
	defer func() { result = f.finish(engine, result) }()
	for _, route := range []string{"/cancel", "/timeout"} {
		attemptCtx, cancel := context.WithCancel(ctx)
		request := f.visit(route[1:], "reuse", route)
		request.Timeout = 10 * time.Second
		done := make(chan synthetic.Execution, 1)
		go func() { done <- engine.Execute(attemptCtx, request, nil) }()
		var err error
		select {
		case hit := <-f.hits:
			if hit != route {
				err = fmt.Errorf("expected fixture request %s, got %s", route, hit)
			}
		case execution := <-done:
			cancel()
			return errors.Join(errors.New("attempt exited before reaching the blocking fixture"), f.verify(ctx, execution, synthetic.Success))
		case <-ctx.Done():
			err = ctx.Err()
		}
		expected, wantPins := synthetic.Timeout, 0
		if route == "/cancel" {
			expected, wantPins = synthetic.Cancelled, 1
			if err == nil {
				peer := f.visit("peer", "reuse", "/")
				peer.Timeout = 7 * time.Second
				err = f.verify(ctx, engine.Execute(ctx, peer, nil), synthetic.Success)
				select {
				case execution := <-done:
					cancel()
					return errors.Join(err, errors.New("blocked attempt ended before peer progress"), f.verify(ctx, execution, synthetic.Success))
				default:
				}
			}
			cancel()
		} else if err != nil {
			cancel()
		}
		outcome := <-done
		cancel()
		if err := errors.Join(err, f.verify(ctx, outcome, expected)); err != nil {
			return err
		}
		if err := f.pins(wantPins); err != nil {
			return err
		}
	}
	return f.verify(ctx, engine.Execute(ctx, f.visit("replacement", "reuse", "/"), nil), synthetic.Success)
}

type cpuTime struct{ agent, children float64 }

func cpu() cpuTime {
	var own, child syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &own)
	_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &child)
	return cpuTime{agent: float64(own.Utime.Nano()+own.Stime.Nano()) / 1e9,
		children: float64(child.Utime.Nano()+child.Stime.Nano()) / 1e9}
}

type workloadOptions struct {
	window, journeyEvery, auditEvery, idle time.Duration
}

func (o workloadOptions) validate() error {
	if o.window < 0 || o.idle < 0 || o.journeyEvery <= 0 || o.auditEvery <= 0 {
		return errors.New("workload durations must be positive, except zero window or idle time")
	}
	return nil
}

type plannedAttempt struct {
	kind     synthetic.Kind
	name     string
	due      time.Duration
	interval time.Duration
}

func planWorkload(journeys, lighthouse int, options workloadOptions) []plannedAttempt {
	var plan []plannedAttempt
	total := journeys + lighthouse
	for i := range total {
		attempt := plannedAttempt{kind: synthetic.Journey, name: fmt.Sprintf("journey-%04d", i), interval: options.journeyEvery}
		if (i+1)*lighthouse/total != i*lighthouse/total {
			attempt.kind, attempt.name, attempt.interval = synthetic.Lighthouse, fmt.Sprintf("audit-%04d", i), options.auditEvery
		}
		if options.window == 0 {
			plan = append(plan, attempt)
			continue
		}
		// Match monitor/session.go's stable JobID phase. This models steady
		// arrivals; it does not run monitor bootstrap or occurrence coalescing.
		h := fnv.New64a()
		_, _ = h.Write([]byte(string(attempt.kind) + ":" + attempt.name))
		attempt.due = time.Duration(h.Sum64() % uint64(attempt.interval))
		for attempt.due < options.window {
			plan = append(plan, attempt)
			attempt.due += attempt.interval
		}
	}
	slices.SortStableFunc(plan, func(a, b plannedAttempt) int {
		if a.due < b.due {
			return -1
		}
		if a.due > b.due {
			return 1
		}
		return 0
	})
	return plan
}

func waitUntil(ctx context.Context, due time.Time) error {
	timer := time.NewTimer(time.Until(due))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (f fixture) workload(ctx context.Context, mode string, slots, journeys, lighthouse int, options workloadOptions) (result error) {
	engine, err := runner.New(f.source, f.history, f.captures, slots)
	if err != nil {
		return err
	}
	if err := engine.Check(ctx, synthetic.Journey); err != nil {
		return f.finish(engine, err)
	}
	plan := planWorkload(journeys, lighthouse, options)
	total := len(plan)
	trace, err := os.Create(filepath.Join(f.work, "attempts.jsonl"))
	if err != nil {
		return f.finish(engine, err)
	}
	defer closeVerified(&result, trace.Close)
	encode := json.NewEncoder(trace)
	type completion struct {
		execution synthetic.Execution
		occupied  time.Duration
		planned   plannedAttempt
		queued    time.Time
		admitted  time.Time
		completed time.Time
		err       error
	}
	results := make(chan completion, total)
	startGate := make(chan struct{})
	var workers sync.WaitGroup
	var start time.Time
	for _, attempt := range plan {
		var request synthetic.Request
		if attempt.kind == synthetic.Lighthouse {
			request = synthetic.Request{Kind: synthetic.Lighthouse, Name: attempt.name,
				URL: f.origin, Timeout: 90 * time.Second}
		} else {
			request = f.visit(attempt.name, mode, "/")
		}
		workers.Go(func() {
			<-startGate
			due := start.Add(attempt.due)
			if err := waitUntil(ctx, due); err != nil {
				results <- completion{planned: attempt, err: err}
				return
			}
			queued := time.Now()
			var admitted time.Time
			execution := engine.ExecuteScheduled(ctx, request, nil, func(when time.Time) synthetic.Schedule {
				admitted = when
				return synthetic.Schedule{DueUS: due.UnixMicro(), QueuedUS: queued.UnixMicro()}
			})
			var occupied time.Duration
			if !admitted.IsZero() {
				occupied = time.Since(admitted)
			}
			results <- completion{execution: execution, occupied: occupied, planned: attempt,
				queued: queued, admitted: admitted, completed: time.Now()}
		})
	}
	beforeCPU := cpu()
	start = time.Now()
	close(startGate)
	var first, journeySlotSeconds, auditSlotSeconds float64
	var maxDelay, maxResponse float64
	var cadenceOverruns, completedJourneys, completedAudits int
	for i := range total {
		completed := <-results
		if completed.err != nil {
			result = errors.Join(result, completed.err)
			continue
		}
		execution := completed.execution
		if i == 0 {
			first = time.Since(start).Seconds()
		}
		run := execution.Run
		due := start.Add(completed.planned.due)
		delay := completed.admitted.Sub(due).Seconds()
		response := completed.completed.Sub(due).Seconds()
		maxDelay, maxResponse = max(maxDelay, delay), max(maxResponse, response)
		if options.window > 0 && completed.completed.Sub(due) >= completed.planned.interval {
			cadenceOverruns++
		}
		result = errors.Join(result, encode.Encode(map[string]any{
			"job": run.JobID, "kind": run.Kind, "due_seconds": completed.planned.due.Seconds(),
			"queued_seconds": completed.queued.Sub(start).Seconds(), "admitted_seconds": completed.admitted.Sub(start).Seconds(),
			"completed_seconds": completed.completed.Sub(start).Seconds(), "slot_seconds": completed.occupied.Seconds(),
			"outcome": run.Outcome, "drained": execution.Drained, "runtime": run.Runtime,
		}))
		if !execution.Drained {
			result = errors.Join(result, fmt.Errorf("%s: %w", run.Name, ndexec.ErrTreeNotDrained))
		} else if run.Outcome != synthetic.Success || run.Runtime == nil || *run.Runtime != f.identity || run.HistoryError != "" {
			result = errors.Join(result, fmt.Errorf("%s: %s (%s, history %s)", run.Name, run.Outcome, run.Error, run.HistoryError))
		}
		if run.Kind == synthetic.Journey {
			completedJourneys++
			journeySlotSeconds += completed.occupied.Seconds()
		} else {
			completedAudits++
			auditSlotSeconds += completed.occupied.Seconds()
		}
	}
	workers.Wait()
	if cadenceOverruns != 0 {
		result = errors.Join(result, fmt.Errorf("%d attempts crossed the next same-job occurrence; paced fixture does not model coalescing", cadenceOverruns))
	}
	batch := time.Since(start).Seconds()
	if options.window > 0 {
		result = errors.Join(result, waitUntil(ctx, start.Add(options.window)))
	}
	workloadEnd := time.Since(start).Seconds()
	if options.idle > 0 {
		result = errors.Join(result, waitUntil(ctx, time.Now().Add(options.idle)), f.pins(0))
	}
	beforeClose := time.Now()
	result = f.finish(engine, result)
	retirement := time.Since(beforeClose).Seconds()
	afterCPU := cpu()
	return errors.Join(result, json.NewEncoder(os.Stdout).Encode(map[string]any{
		"mode": mode, "runtime": f.identity, "journeys": journeys, "lighthouse": lighthouse, "slots": slots,
		"start_unix_us":        start.UnixMicro(),
		"paced_window_seconds": options.window.Seconds(), "idle_after_seconds": options.idle.Seconds(),
		"journey_interval_seconds": options.journeyEvery.Seconds(), "lighthouse_interval_seconds": options.auditEvery.Seconds(),
		"planned_attempts": total, "completed_journeys": completedJourneys, "completed_audits": completedAudits,
		"max_scheduling_delay_seconds": maxDelay, "max_response_seconds": maxResponse, "cadence_overruns": cadenceOverruns,
		"workload_end_seconds": workloadEnd,
		"first_result_seconds": first, "batch_seconds": batch, "shutdown_seconds": retirement,
		"total_seconds": time.Since(start).Seconds(), "journey_slot_seconds": journeySlotSeconds, "lighthouse_slot_seconds": auditSlotSeconds,
		"agent_cpu_seconds": afterCPU.agent - beforeCPU.agent, "child_cpu_seconds": afterCPU.children - beforeCPU.children,
		"runtime_uses": f.source.uses.Load(), "passed": result == nil,
	}))
}
