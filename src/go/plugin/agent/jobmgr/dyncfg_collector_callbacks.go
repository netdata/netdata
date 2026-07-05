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

// disposeWarmResume disposes a warm job that will never start, SILENTLY:
// its emission gate is closed first (by captured handle), because V1
// Cleanup emits HOST/HOSTINFO lines even for a never-started job, and a
// dropped continuation must publish nothing - at shutdown that is the one
// rule, and on stale drops the identity of a job whose start was suppressed
// must not reach the wire. The gate is then deregistered matched by handle
// (a same-name replacement's gate must survive).
func (m *Manager) disposeWarmResume(r *warmResume) {
	if r.gate != nil {
		r.gate.Close()
	}
	r.job.Cleanup()
	m.emissionGates.removeMatching(r.cfg.FullName(), r.gate)
}

// disposeUnstartedJob cleans up a job that never started. When silent (the
// command's outcome publishes nothing - the shutdown paths), the job's
// emission gate closes first: V1 Cleanup emits HOST/HOSTINFO lines even for
// a never-started job, and identity output for a suppressed start must not
// reach the wire after a 503. The looked-up gate is this job's own - the
// key stays busy (or wedged) for the whole effect and its late window, so
// no same-name replacement can interleave.
func (m *Manager) disposeUnstartedJob(name string, job runtimeJob, silent bool) {
	if silent {
		if gate, ok := m.emissionGates.lookup(name); ok {
			gate.Close()
		}
	}
	job.Cleanup()
	m.emissionGates.remove(name)
}

// shutdownDisposal reports whether a never-started job's disposal must be
// silent because its command's outcome publishes nothing at shutdown:
// either shutdown cancelled the effect context, or the manager is shutting
// down (the manager check covers control-free restarts whose context cause
// stays pinned to the deadline). Outside shutdown, failure disposals keep
// today's cleanup emission - they accompany a published failure terminal.
func shutdownDisposal(ctx context.Context, m *Manager) bool {
	return errors.Is(context.Cause(ctx), context.Canceled) || m.baseContext().Err() != nil
}

// resumeWarmJob starts an already-detected job whose detection was abandoned
// at its deadline but succeeded late. The caller (run loop) has already
// validated the continuation (no commits on the key since the abandon and
// store dependencies unchanged); here the exposed entry must still be this
// config in its abandoned-failed state, else the warm job is disposed
// silently. The vnode config the job snapshotted at creation is reconciled
// at registration (startRunningJob), covering updates that committed while
// the key was wedged.
func (m *Manager) resumeWarmJob(r *warmResume) {
	entry, ok := m.collectorExposed.LookupByKey(r.cfg.ExposedKey())
	if !ok || entry.Cfg.UID() != r.cfg.UID() || entry.Status != dyncfg.StatusFailed {
		m.Infof("dropping late detection success for '%s': config no longer eligible", r.cfg.FullName())
		m.disposeWarmResume(r)
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

// parseCollectorPayload is the CHEAP, deterministic parse prefix of
// ParseAndValidate (no I/O, no module instantiation), shared with
// ParsePayload so the stage gate and the effect produce identical outcomes
// for identical inputs.
func (cb *collectorCallbacks) parseCollectorPayload(fn dyncfg.Function, name string) (confgroup.Config, error) {
	mn, ok := cb.mgr.extractModuleName(fn.ID())
	if !ok {
		return nil, fmt.Errorf("could not extract module name from ID: %s", fn.ID())
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration format: failed to create configuration from payload: %v", err)
	}

	cb.mgr.dyncfgSetConfigMeta(cfg, mn, name, fn)
	return cfg, nil
}

// ParsePayload implements dyncfg.PayloadParser: malformed payloads answer
// their 400 at stage, before any claim or effect.
func (cb *collectorCallbacks) ParsePayload(fn dyncfg.Function, name string) error {
	_, err := cb.parseCollectorPayload(fn, name)
	return err
}

func (cb *collectorCallbacks) ParseAndValidate(ctx context.Context, fn dyncfg.Function, name string) (confgroup.Config, error) {
	cfg, err := cb.parseCollectorPayload(fn, name)
	if err != nil {
		return nil, err
	}

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
		// A late failure after a shutdown-finalized 503 must dispose
		// silently; outside shutdown the cleanup emission accompanies the
		// published timeout terminal, as today.
		cb.mgr.disposeUnstartedJob(cfg.FullName(), job, shutdownDisposal(ctx, cb.mgr))
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
		// Silent when shutdown owns the outcome (503-publish-nothing);
		// otherwise the cleanup emission accompanies the published failure
		// terminal, as today.
		cb.mgr.disposeUnstartedJob(cfg.FullName(), job, shutdownDisposal(ctx, cb.mgr))
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

	if errors.Is(context.Cause(ctx), context.Canceled) || cb.mgr.baseContext().Err() != nil {
		// Shutdown began while the detection was finishing: never start
		// fresh collector work. The manager-context check matters for
		// restarts running inside another command's effect (a store
		// command's dependent restart runs control-free, and its context
		// cause stays DeadlineExceeded once that effect's deadline fired -
		// it can never read Canceled, yet its late success must not start a
		// job after cleanup snapshotted the running set). The command
		// answers 503-publish-nothing, so the never-started job disposes
		// silently: the gate closes (this job's own - the key stays busy
		// for the whole effect, no same-name replacement can interleave)
		// before Cleanup can emit HOST/HOSTINFO lines.
		cb.mgr.disposeUnstartedJob(cfg.FullName(), job, true)
		return fmt.Errorf("job not started: %w", dyncfg.ErrPhaseNeverRan)
	}

	cb.mgr.startRunningJob(job)
	return nil
}

// StageUpdate splits the update at the stop seam: the old instance's routing
// removal happens on the staging goroutine (function routing to the job
// being replaced ends immediately), the blocking wait plus the replacement's
// creation/detection/start run in the effect. Undo reverts the routing
// removal when the effect never ran.
func (cb *collectorCallbacks) StageUpdate(oldCfg, newCfg confgroup.Config) dyncfg.StagedUpdate {
	staged := cb.mgr.newStagedJobStop(oldCfg.FullName())
	cb.mgr.retryingTasks.remove(oldCfg)
	return dyncfg.StagedUpdate{
		Run: func(ctx context.Context) error {
			cb.mgr.fileStatus.remove(oldCfg)
			staged.wait(ctx)
			return cb.startUpdatedJob(ctx, newCfg)
		},
		Undo: staged.undo,
	}
}

func (cb *collectorCallbacks) Update(ctx context.Context, oldCfg, newCfg confgroup.Config) error {
	return cb.StageUpdate(oldCfg, newCfg).Run(ctx)
}

// startUpdatedJob is the update effect's post-stop half: creation, detection
// and start of the replacement instance.
func (cb *collectorCallbacks) startUpdatedJob(ctx context.Context, newCfg confgroup.Config) error {
	// A stop phase that exceeded its deadline fails the update here: the
	// replacement's start phase never begins (no second instance).
	if effectControlFrom(ctx).abandonedNow() {
		return fmt.Errorf("job update failed: the previous instance's stop timed out and was abandoned")
	}

	job, err := cb.mgr.createCollectorJob(ctx, newCfg)
	if err != nil {
		effectControlFrom(ctx).claimCompletion()
		if errors.Is(context.Cause(ctx), context.Canceled) {
			// Shutdown interrupted the update: one rule - answer 503 and
			// publish nothing, whether or not the old instance was already
			// torn down.
			return fmt.Errorf("job not created: %w", dyncfg.ErrPhaseNeverRan)
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
		// Same silence rule as Start's late-failure arm.
		cb.mgr.disposeUnstartedJob(newCfg.FullName(), job, shutdownDisposal(ctx, cb.mgr))
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
		// Same silence rule as Start's claimed-failure arm.
		cb.mgr.disposeUnstartedJob(newCfg.FullName(), job, shutdownDisposal(ctx, cb.mgr))
		if errors.Is(context.Cause(ctx), context.Canceled) {
			// Shutdown interrupted the replacement's detection: one rule -
			// answer 503, publish nothing, schedule nothing.
			return fmt.Errorf("detection interrupted: %w", dyncfg.ErrPhaseNeverRan)
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

	if errors.Is(context.Cause(ctx), context.Canceled) || cb.mgr.baseContext().Err() != nil {
		// Shutdown began while the replacement's detection was finishing:
		// never start fresh collector work - one rule, answer 503. (The
		// manager-context check covers control-free restarts whose context
		// cause is pinned to DeadlineExceeded; see Start.) Publish-nothing
		// extends to the never-started replacement's disposal: close its
		// gate (its own - the key is busy for the whole effect) before
		// Cleanup can emit HOST/HOSTINFO lines.
		cb.mgr.disposeUnstartedJob(newCfg.FullName(), job, true)
		return fmt.Errorf("job not started: %w", dyncfg.ErrPhaseNeverRan)
	}

	cb.mgr.startRunningJob(job)
	return nil
}

// StageStop splits the stop at its blocking wait: routing removal happens on
// the staging goroutine, the wait runs in the effect. Undo reverts the
// routing removal when the wait never ran.
func (cb *collectorCallbacks) StageStop(cfg confgroup.Config) dyncfg.StagedStop {
	staged := cb.mgr.newFinalPhaseStagedJobStop(cfg.FullName())
	cb.mgr.retryingTasks.remove(cfg)
	return dyncfg.StagedStop{
		Wait: func(ctx context.Context) {
			// Persisted-status removal precedes the blocking wait so the
			// deadline path (which commits disable/remove while the stop is
			// still wedged) observes it done.
			cb.mgr.fileStatus.remove(cfg)
			staged.wait(ctx)
		},
		Undo: staged.undo,
	}
}

func (cb *collectorCallbacks) Stop(ctx context.Context, cfg confgroup.Config) {
	cb.StageStop(cfg).Wait(ctx)
}

func (cb *collectorCallbacks) OnStatusChange(entry *dyncfg.Entry[confgroup.Config], _ dyncfg.Status, _ dyncfg.Function) {
	if entry.Status != dyncfg.StatusRunning {
		return
	}
	// Function publication follows COMMITTED state: the effect started the
	// job and registered it in the running set; the commit that publishes
	// the running status publishes its functions (warm-start continuations
	// and dependent restarts commit through here too).
	cb.mgr.publishRunningJobFunctions(entry.Cfg.FullName())
	if isDyncfg(entry.Cfg) {
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
