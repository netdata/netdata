// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
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

func (cb *collectorCallbacks) ParseAndValidate(fn dyncfg.Function, name string) (confgroup.Config, error) {
	mn, ok := cb.mgr.extractModuleName(fn.ID())
	if !ok {
		return nil, fmt.Errorf("could not extract module name from ID: %s", fn.ID())
	}

	cfg, err := configFromPayload(fn)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration format: failed to create configuration from payload: %v", err)
	}

	cb.mgr.dyncfgSetConfigMeta(cfg, mn, name, fn)

	if err := cb.mgr.validateCollectorJob(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: failed to apply configuration: %v", err)
	}

	return cfg, nil
}

func (cb *collectorCallbacks) Start(cfg confgroup.Config) error {
	cb.mgr.retryingTasks.remove(cfg)

	job, err := cb.mgr.createCollectorJob(cfg)
	if err != nil {
		var ce dyncfg.CodedError
		if errors.As(err, &ce) {
			return err
		}
		return &codedError{err: fmt.Errorf("invalid configuration: failed to apply configuration: %w", err), code: 400}
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		var ce dyncfg.CodedError
		if errors.As(err, &ce) {
			if dyncfg.IsRetryableError(err) {
				cb.mgr.scheduleRetryTask(cfg, job)
			}
			return err
		}
		cb.mgr.scheduleRetryTask(cfg, job)
		return fmt.Errorf("job enable failed: %w", err)
	}

	cb.mgr.startRunningJob(job)
	return nil
}

func (cb *collectorCallbacks) Update(oldCfg, newCfg confgroup.Config) error {
	cb.mgr.retryingTasks.remove(oldCfg)
	cb.mgr.stopRunningJob(oldCfg.FullName())
	cb.mgr.fileStatus.remove(oldCfg)

	job, err := cb.mgr.createCollectorJob(newCfg)
	if err != nil {
		var ce dyncfg.CodedError
		if errors.As(err, &ce) {
			return err
		}
		return fmt.Errorf("job update failed: %w", err)
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		var ce dyncfg.CodedError
		if errors.As(err, &ce) {
			if dyncfg.IsRetryableError(err) {
				cb.mgr.scheduleRetryTask(newCfg, job)
			}
			return err
		}
		cb.mgr.scheduleRetryTask(newCfg, job)
		return fmt.Errorf("job update failed: %w", err)
	}

	cb.mgr.startRunningJob(job)
	return nil
}

func (cb *collectorCallbacks) Stop(cfg confgroup.Config) {
	cb.mgr.retryingTasks.remove(cfg)
	cb.mgr.stopRunningJob(cfg.FullName())
	cb.mgr.fileStatus.remove(cfg)
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
