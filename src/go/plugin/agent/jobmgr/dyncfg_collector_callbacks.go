// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// scheduleRetryTask schedules a retry if the job supports auto-detection retry.
func (m *Manager) scheduleRetryTask(cfg confgroup.Config, job runtimeJob) {
	if !job.RetryAutoDetection() {
		return
	}
	m.Infof("%s[%s] job detection failed, will retry in %d seconds",
		cfg.Module(), cfg.Name(), job.AutoDetectionEvery())

	ctx, cancel := context.WithCancel(m.ctx)
	m.retryingTasks.add(cfg, &retryTask{cancel: cancel})

	go runRetryTask(ctx, m.addCh, cfg)
}

// resumeWarmJob starts an already-detected job whose detection was abandoned
// at its deadline but succeeded late. The caller (run loop) has already
// validated the continuation (no commits on the key since the abandon); here
// the exposed entry must still be this config in its abandoned-failed state,
// else the warm job is disposed (module cleanup + gate deregistration).
func (m *Manager) resumeWarmJob(r *warmResume) {
	entry, ok := m.collectorExposed.LookupByKey(r.cfg.ExposedKey())
	if !ok || entry.Cfg.UID() != r.cfg.UID() || entry.Status != dyncfg.StatusFailed {
		m.Infof("dropping late detection success for '%s': config no longer eligible", r.cfg.FullName())
		r.job.Cleanup()
		m.emissionGates.removeMatching(r.cfg.FullName(), r.gate)
		return
	}
	m.retryingTasks.remove(r.cfg)
	oldStatus := entry.Status
	m.startRunningJob(r.job)
	entry.Status = dyncfg.StatusRunning
	m.collectorHandler.NotifyConfigStatus(entry.Cfg, dyncfg.StatusRunning)
	m.collectorCallbacks.OnStatusChange(entry, oldStatus, dyncfg.NewFunction(functions.Function{}))
	m.Infof("late detection success for '%s': warm job started", r.cfg.FullName())
}

// --- collectorCallbacks implements dyncfg.Callbacks[confgroup.Config] ---

type collectorCallbacks struct {
	mgr *Manager
}

func (cb *collectorCallbacks) ExtractKey(fn dyncfg.Function) (key, name string, ok bool) {
	key, name, warnErr, ok := cb.mgr.collectorCommandKey(fn)
	if warnErr != nil {
		cb.mgr.Warningf("dyncfg: %s: %v", fn.Command(), warnErr)
	}
	return key, name, ok
}

// collectorCommandKey derives the exposed-cache key a collector dyncfg
// command addresses. It never logs so the executor can derive keys at event
// construction without duplicating ExtractKey's execution-time warnings:
// warnErr, when non-nil, is the warning ExtractKey reports; malformed IDs
// that ExtractKey rejects silently fail with a nil warnErr.
func (m *Manager) collectorCommandKey(fn dyncfg.Function) (key, name string, warnErr error, ok bool) {
	var mn, jn string

	if fn.Command() == dyncfg.CommandAdd {
		// For add: ID is module template, job name is in Args[2].
		mn, ok = m.extractModuleName(fn.ID())
		if !ok {
			return "", "", nil, false
		}
		if m.isSingleInstanceCollector(mn) {
			return "", "", fmt.Errorf("single-instance collector %s does not support add", mn), false
		}
		jn = fn.JobName()
		if jn == "" {
			return "", "", nil, false
		}
	} else {
		mn, ok = m.extractModuleName(fn.ID())
		if !ok {
			return "", "", nil, false
		}
		if m.isSingleInstanceCollector(mn) {
			if fn.ID() != m.dyncfgModID(mn) {
				return "", "", fmt.Errorf("single-instance collector %s must use config ID %s", mn, m.dyncfgModID(mn)), false
			}
			jn = mn
		} else {
			// For per-job commands: ID contains module:job.
			mn, jn, ok = m.extractModuleJobName(fn.ID())
			if !ok {
				return "", "", nil, false
			}
		}
	}
	if err := m.validateDyncfgCollectorIdentity(mn, jn); err != nil {
		return "", "", err, false
	}

	key = mn + "_" + jn
	if mn == jn {
		key = jn
	}
	return key, jn, nil, true
}

func (cb *collectorCallbacks) ValidateConfigName(name string) error {
	return dyncfg.JobNameRuleStrict(name)
}

func (cb *collectorCallbacks) ParseAndValidate(ctx context.Context, fn dyncfg.Function, name string) (confgroup.Config, error) {
	mn, ok := cb.mgr.extractModuleName(fn.ID())
	if !ok {
		return nil, fmt.Errorf("could not extract module name from ID: %s", fn.ID())
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration format: failed to create configuration from payload: %v", err)
	}

	cb.mgr.dyncfgSetConfigMeta(cfg, mn, name, fn)

	if err := cb.mgr.validateCollectorJob(ctx, cfg); err != nil {
		if errors.Is(context.Cause(ctx), context.Canceled) {
			// Shutdown interrupted validation (e.g. a cancelled secret
			// fetch): the config is not invalid - answer retryably.
			return nil, fmt.Errorf("validation interrupted: %w", dyncfg.ErrPhaseNeverRan)
		}
		return nil, fmt.Errorf("invalid configuration: failed to apply configuration: %v", err)
	}

	return cfg, nil
}

func (cb *collectorCallbacks) Start(ctx context.Context, cfg confgroup.Config) error {
	cb.mgr.retryingTasks.remove(cfg)

	job, err := cb.mgr.createCollectorJob(ctx, cfg)
	if err != nil {
		// Creation failures cannot outlive the deadline meaningfully; claim
		// so a raced abandon does not also expect a late outcome... they can:
		// the claim decides which side reports.
		effectControlFrom(ctx).claimCompletion()
		if errors.Is(context.Cause(ctx), context.Canceled) {
			return fmt.Errorf("job not created: %w", dyncfg.ErrPhaseNeverRan)
		}
		if _, ok := errors.AsType[dyncfg.CodedError](err); !ok {
			err = &codedError{err: fmt.Errorf("invalid configuration: failed to apply configuration: %w", err), code: 400}
		}
		return err
	}

	detErr := job.AutoDetection(ctx)

	if !effectControlFrom(ctx).claimCompletion() {
		// Abandoned at the deadline: the command already committed its
		// failure. SUCCESS becomes a warm-start continuation the loop
		// validates and starts; FAILURE cleans up and evaluates retry
		// eligibility now, under the normal rules.
		if detErr == nil {
			// The registered gate is this job's own (the key is busy for the
			// whole effect, so no same-name replacement can interleave); drop
			// paths deregister exactly it.
			gate, _ := cb.mgr.emissionGates.lookup(cfg.FullName())
			effectControlFrom(ctx).setResume(&warmResume{cfg: cfg, job: job, gate: gate})
			return nil
		}
		job.Cleanup()
		cb.mgr.emissionGates.remove(cfg.FullName())
		if _, ok := errors.AsType[dyncfg.CodedError](detErr); ok {
			if dyncfg.IsRetryableError(detErr) {
				cb.mgr.scheduleRetryTask(cfg, job)
			}
		} else {
			cb.mgr.scheduleRetryTask(cfg, job)
		}
		return detErr
	}

	if detErr != nil {
		job.Cleanup()
		// Tracking removal only. The gate itself stays open by contract -
		// closing is reserved for abandoning a wedged stop - and no writer
		// survives here anyway: the job never started.
		cb.mgr.emissionGates.remove(cfg.FullName())
		if errors.Is(context.Cause(ctx), context.Canceled) {
			// Shutdown interrupted the detection (a ctx-honoring module
			// returned early): not a detection failure - answer retryably,
			// publish nothing, schedule nothing.
			return fmt.Errorf("detection interrupted: %w", dyncfg.ErrPhaseNeverRan)
		}
		if _, ok := errors.AsType[dyncfg.CodedError](detErr); ok {
			if dyncfg.IsRetryableError(detErr) {
				cb.mgr.scheduleRetryTask(cfg, job)
			}
			return detErr
		}
		cb.mgr.scheduleRetryTask(cfg, job)
		return fmt.Errorf("job enable failed: %w", detErr)
	}

	if errors.Is(context.Cause(ctx), context.Canceled) {
		// Shutdown began while the detection was finishing (the child won
		// the completion claim against the cancelled context): never start
		// fresh collector work - the enable answers retryably instead.
		job.Cleanup()
		cb.mgr.emissionGates.remove(cfg.FullName())
		return fmt.Errorf("job not started: %w", dyncfg.ErrPhaseNeverRan)
	}

	cb.mgr.startRunningJob(job)
	return nil
}

func (cb *collectorCallbacks) Update(ctx context.Context, oldCfg, newCfg confgroup.Config) error {
	cb.mgr.retryingTasks.remove(oldCfg)
	cb.mgr.fileStatus.remove(oldCfg)
	stopped := cb.mgr.stopRunningJob(ctx, oldCfg.FullName())
	if stopped {
		// Point of no return: the old instance is stopped. A shutdown
		// interruption from here on must commit as a disruptive failure - a
		// never-ran rollback would resurrect a state that no longer exists.
		// A no-op stop (the old config was not running) tears nothing down,
		// so never-ran (and its rollback) stays the truthful outcome.
		effectControlFrom(ctx).markDisruptive()
	}

	// A stop phase that exceeded its deadline fails the update here: the
	// replacement's start phase never begins (no second instance).
	if effectControlFrom(ctx).abandonedNow() {
		return fmt.Errorf("job update failed: the previous instance's stop timed out and was abandoned")
	}

	job, err := cb.mgr.createCollectorJob(ctx, newCfg)
	if err != nil {
		effectControlFrom(ctx).claimCompletion()
		if errors.Is(context.Cause(ctx), context.Canceled) {
			if !stopped {
				// Nothing was torn down: answer retryably with rollback.
				return fmt.Errorf("job not created: %w", dyncfg.ErrPhaseNeverRan)
			}
			// Shutdown interrupted the replacement's creation. The old
			// instance is already stopped: a disruptive failure, not a
			// rollback.
			return fmt.Errorf("job update failed: the manager is shutting down; the replacement was not created")
		}
		var ce dyncfg.CodedError
		if errors.As(err, &ce) {
			return err
		}
		return fmt.Errorf("job update failed: %w", err)
	}

	detErr := job.AutoDetection(ctx)

	if !effectControlFrom(ctx).claimCompletion() {
		if detErr == nil {
			gate, _ := cb.mgr.emissionGates.lookup(newCfg.FullName())
			effectControlFrom(ctx).setResume(&warmResume{cfg: newCfg, job: job, gate: gate})
			return nil
		}
		job.Cleanup()
		cb.mgr.emissionGates.remove(newCfg.FullName())
		if _, ok := errors.AsType[dyncfg.CodedError](detErr); ok {
			if dyncfg.IsRetryableError(detErr) {
				cb.mgr.scheduleRetryTask(newCfg, job)
			}
		} else {
			cb.mgr.scheduleRetryTask(newCfg, job)
		}
		return detErr
	}

	if detErr != nil {
		job.Cleanup()
		// Tracking removal only; see the identical note in Start.
		cb.mgr.emissionGates.remove(newCfg.FullName())
		if errors.Is(context.Cause(ctx), context.Canceled) {
			if !stopped {
				return fmt.Errorf("job not started: %w", dyncfg.ErrPhaseNeverRan)
			}
			// Shutdown interrupted the replacement's detection: disruptive
			// failure (the old instance is already stopped), no retry.
			return fmt.Errorf("job update failed: the manager is shutting down; the replacement was not started")
		}
		var ce dyncfg.CodedError
		if errors.As(detErr, &ce) {
			if dyncfg.IsRetryableError(detErr) {
				cb.mgr.scheduleRetryTask(newCfg, job)
			}
			return detErr
		}
		cb.mgr.scheduleRetryTask(newCfg, job)
		return fmt.Errorf("job update failed: %w", detErr)
	}

	if errors.Is(context.Cause(ctx), context.Canceled) {
		// Shutdown began while the replacement's detection was finishing:
		// never start fresh collector work.
		job.Cleanup()
		cb.mgr.emissionGates.remove(newCfg.FullName())
		if !stopped {
			// Nothing was torn down: answer retryably with rollback.
			return fmt.Errorf("job not started: %w", dyncfg.ErrPhaseNeverRan)
		}
		// The old instance is already stopped, so this is a disruptive
		// failure (status failed), not a rollback - unlike the never-ran
		// phase, half the update happened.
		return fmt.Errorf("job update failed: the manager is shutting down; the replacement was not started")
	}

	cb.mgr.startRunningJob(job)
	return nil
}

func (cb *collectorCallbacks) Stop(ctx context.Context, cfg confgroup.Config) {
	cb.mgr.retryingTasks.remove(cfg)
	// Persisted-status removal precedes the blocking wait so the deadline
	// path (which commits disable/remove while the stop is still wedged)
	// observes it done.
	cb.mgr.fileStatus.remove(cfg)
	cb.mgr.stopRunningJob(ctx, cfg.FullName())
	effectControlFrom(ctx).claimCompletion()
}

func (cb *collectorCallbacks) OnStatusChange(entry *dyncfg.Entry[confgroup.Config], _ dyncfg.Status, _ dyncfg.Function) {
	if entry.Status == dyncfg.StatusRunning && isDyncfg(entry.Cfg) {
		cb.mgr.fileStatus.add(entry.Cfg, entry.Status.String())
	}
}

func (cb *collectorCallbacks) ConfigID(cfg confgroup.Config) string {
	return cb.mgr.dyncfgConfigID(cfg)
}

func (cb *collectorCallbacks) ConfigType(cfg confgroup.Config) dyncfg.ConfigType {
	if cb.mgr.isSingleInstanceCollector(cfg.Module()) {
		return dyncfg.ConfigTypeSingle
	}
	return dyncfg.ConfigTypeJob
}

// codedError wraps an error with an HTTP status code for the handler.
type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string   { return e.err.Error() }
func (e *codedError) Unwrap() error   { return e.err }
func (e *codedError) DyncfgCode() int { return e.code }
