// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
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
	var mn, jn string

	if fn.Command() == dyncfg.CommandAdd {
		// For add: ID is module template, job name is in Args[2].
		mn, ok = cb.mgr.extractModuleName(fn.ID())
		if !ok {
			return "", "", false
		}
		jn = fn.JobName()
		if jn == "" {
			return "", "", false
		}
	} else {
		// For other commands: ID contains module:job.
		mn, jn, ok = cb.mgr.extractModuleJobName(fn.ID())
		if !ok {
			return "", "", false
		}
	}

	key = mn + "_" + jn
	if mn == jn {
		key = jn
	}
	return key, jn, true
}

func (cb *collectorCallbacks) ValidateJobName(name string) error {
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
		return &codedError{err: fmt.Errorf("invalid configuration: failed to apply configuration: %v", err), code: 400}
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		cb.mgr.scheduleRetryTask(cfg, job)
		return fmt.Errorf("job enable failed: %v", err)
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
		return fmt.Errorf("job update failed: %v", err)
	}

	if err := job.AutoDetection(); err != nil {
		job.Cleanup()
		cb.mgr.scheduleRetryTask(newCfg, job)
		return fmt.Errorf("job update failed: %v", err)
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
	return cb.mgr.dyncfgJobID(cfg)
}

// codedError wraps an error with an HTTP status code for the handler.
type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }
func (e *codedError) Code() int     { return e.code }
