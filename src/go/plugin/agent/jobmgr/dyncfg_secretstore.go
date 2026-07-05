// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secretsctl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func (m *Manager) dyncfgSecretStorePrefixValue() string {
	return m.secretsCtl.Prefix()
}

func (m *Manager) dyncfgSecretStoreExec(fn dyncfg.Function) {
	if fn.Command() == dyncfg.CommandSchema {
		m.dyncfgSecretStoreSeqExec(fn)
		return
	}
	m.enqueueDyncfgFunction(fn)
}

func (m *Manager) dyncfgSecretStoreSeqExec(fn dyncfg.Function) {
	m.secretsCtl.SeqExec(fn)
}

// secretStoreDependentPlan is one dependent job's restart order, snapshotted
// on the run loop while the store command holds the dependent's write claim
// (so the entry cannot change until the command commits).
type secretStoreDependentPlan struct {
	name    string
	display string
	cfg     confgroup.Config
	// wedged dependents are never restarted (their key is held by an
	// abandoned effect with no bounded release); they are skipped and
	// reported in the terminal message.
	wedged bool
}

// restartReplayBuffer collects completed dependent restarts' loop-side
// commits AS THEY COMPLETE, so a command abandoned at its deadline still
// replays the restarts that did happen. The mutex covers effect-goroutine
// appends vs loop-side drains; drain-on-flush makes every commit replay
// exactly once (at the command's commit, or at the late return for restarts
// that completed after the abandon).
type restartReplayBuffer struct {
	mu      sync.Mutex
	commits []func()
}

func (b *restartReplayBuffer) add(fn func()) {
	b.mu.Lock()
	b.commits = append(b.commits, fn)
	b.mu.Unlock()
}

// flush drains and runs the buffered commits in order. Loop-side only.
func (b *restartReplayBuffer) flush() {
	b.mu.Lock()
	commits := b.commits
	b.commits = nil
	b.mu.Unlock()
	for _, fn := range commits {
		fn()
	}
}

// stageDependentRestarts snapshots the store's dependent-restart plan on the
// run loop; the returned Run restarts the planned dependents sequentially
// inside the effect, and Flush replays the completed restarts' entry-state
// and CONFIG STATUS transitions on the loop.
func (m *Manager) stageDependentRestarts(storeKey string) *secretsctl.StagedRestarts {
	plan := m.secretStoreRestartPlan(storeKey)
	buf := &restartReplayBuffer{}
	return &secretsctl.StagedRestarts{
		Run: func(ctx context.Context) (string, error) {
			return m.runDependentRestarts(ctx, storeKey, plan, buf)
		},
		Flush: buf.flush,
	}
}

// secretStoreRestartPlan selects the store's restartable dependents (running
// or failed jobs referencing it) and marks the wedged ones for
// skip-and-report. Loop-owned state: call only on the run-loop goroutine.
func (m *Manager) secretStoreRestartPlan(storeKey string) []secretStoreDependentPlan {
	var plan []secretStoreDependentPlan
	for _, ref := range m.restartableAffectedJobs(storeKey) {
		entry, ok := m.lookupExposedByFullName(ref.ID)
		if !ok {
			continue
		}
		plan = append(plan, secretStoreDependentPlan{
			name:    ref.ID,
			display: ref.Display,
			cfg:     entry.Cfg,
			wedged:  m.executor.collectorKeyWedged(entry.Cfg.ExposedKey()),
		})
	}
	return plan
}

// runDependentRestarts executes a staged restart plan inside a store
// command's effect. Restarts run sequentially in plan order; the deadline
// (and the command's abandonment) is checked before EACH restart, so a store
// command that runs out of time launches no new restarts and reports the
// remainder as skipped. Failures use the terminal message's existing per-job
// format. Every completed restart's replay lands in the buffer as it
// completes: the command's commit flushes it, and a restart completing only
// after a deadline abandon replays at the late return (the buffer flush is
// registered as the effect's late work; the dependents' write claims are
// held until then, so the late replay stays truthful).
//
// A deadline-cut sequence returns a non-nil error alongside the message:
// the classification must not depend on whether the effect's return or the
// worker's abandon wins their select race, so a command whose restarts were
// cut reaches the timeout failure even when its closure completes first.
// Wedged-dependent skips do NOT set the error - skip-and-report is the
// command's normal successful outcome for wedged keys.
func (m *Manager) runDependentRestarts(ctx context.Context, storeKey string, plan []secretStoreDependentPlan, buf *restartReplayBuffer) (string, error) {
	type failure struct {
		display string
		err     error
	}
	var failures []failure
	var cutErr error
	ctl := effectControlFrom(ctx)
	ctl.setLateWork(buf.flush)
	// Inner restarts must never claim the STORE command's completion or
	// register fences on it: they run under a control-free context that
	// still carries the deadline and cancellation.
	depCtx := context.WithValue(ctx, effectControlKey{}, (*effectControl)(nil))

	for _, dep := range plan {
		fail := func(err error) {
			m.Warningf("dyncfg: secretstore: failed to restart dependent job '%s' after store '%s' change: %v", dep.name, storeKey, err)
			failures = append(failures, failure{display: dep.display, err: err})
		}
		if dep.wedged {
			fail(fmt.Errorf("skipped: operation in progress"))
			continue
		}
		if ctx.Err() != nil || ctl.abandonedNow() {
			if cutErr == nil {
				cutErr = fmt.Errorf("the store operation timed out before all dependent restarts started")
			}
			fail(fmt.Errorf("skipped: the store operation timed out before this restart started"))
			continue
		}

		m.collectorCallbacks.Stop(depCtx, dep.cfg)
		status := dyncfg.StatusRunning
		if err := m.collectorCallbacks.Start(depCtx, dep.cfg); err != nil {
			fail(fmt.Errorf("job '%s' restart failed: %w", dep.name, err))
			status = dyncfg.StatusFailed
		}
		buf.add(m.commitDependentRestart(dep, status))
	}

	if len(failures) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(failures))
	for _, f := range failures {
		name := f.display
		if name == "" {
			name = "unknown"
		}
		parts = append(parts, fmt.Sprintf("%s (%v)", name, f.err))
	}
	return fmt.Sprintf("Secretstore change applied, but dependent collector restarts failed: %s.", strings.Join(parts, "; ")), cutErr
}

// commitDependentRestart is the loop-side half of one dependent restart: the
// entry-state transition, its CONFIG STATUS line, and the status-change hook
// (which publishes the restarted job's functions).
func (m *Manager) commitDependentRestart(dep secretStoreDependentPlan, status dyncfg.Status) func() {
	return func() {
		entry, ok := m.lookupExposedByFullName(dep.name)
		if !ok || entry.Cfg.UID() != dep.cfg.UID() {
			return
		}
		oldStatus := entry.Status
		entry.Status = status
		m.collectorHandler.NotifyConfigStatus(entry.Cfg, status)
		m.collectorCallbacks.OnStatusChange(entry, oldStatus, dyncfg.NewFunction(functions.Function{}))
	}
}

func (m *Manager) lookupExposedByFullName(fullName string) (*dyncfg.Entry[confgroup.Config], bool) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return nil, false
	}
	return m.collectorExposed.LookupByKey(fullName)
}

// secretStoreDepIdentity is the committed identity of a store dependency as
// observed on the loop: exposed status plus config content hash, "" when the
// store is not exposed. Captured at a collector command's deadline abandon
// and compared at its late return - any store mutation committed in between
// changes it and invalidates the warm continuation (executor.staleWarmDep).
// A re-apply of identical content (the handler's no-op path) leaves it
// unchanged by design.
func (m *Manager) secretStoreDepIdentity(storeKey string) string {
	entry, ok := m.secretsCtl.Lookup(storeKey)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s|%d", entry.Status, entry.Cfg.Hash())
}
