// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/logger"

	"gopkg.in/yaml.v2"
)

type Job interface {
	Name() string
	ModuleName() string
	FullName() string
	AutoDetection() bool
	AutoDetectionEvery() int
	RetryAutoDetection() bool
	Tick(clock int)
	Start()
	Stop()
	Cleanup()
}

type jobStatus = string

const (
	jobStatusRunning          jobStatus = "running"                    // Check() succeeded
	jobStatusRetrying         jobStatus = "retrying"                   // Check() failed, but we need keep trying auto-detection
	jobStatusStoppedFailed    jobStatus = "stopped_failed"             // Check() failed
	jobStatusStoppedDupLocal  jobStatus = "stopped_duplicate_local"    // a job with the same FullName is running
	jobStatusStoppedDupGlobal jobStatus = "stopped_duplicate_global"   // a job with the same FullName is registered by another plugin
	jobStatusStoppedRegErr    jobStatus = "stopped_registration_error" // an error during registration (only 'too many open files')
	jobStatusStoppedCreateErr jobStatus = "stopped_creation_error"     // an error during creation (yaml unmarshal)
)

func NewManager() *Manager {
	np := noop{}
	mgr := &Manager{
		Logger: logger.New().With(
			slog.String("component", "job manager"),
		),
		Out:         io.Discard,
		FileLock:    np,
		StatusSaver: np,
		StatusStore: np,
		Vnodes:      np,
		Dyncfg:      np,

		confGroupCache: confgroup.NewCache(),

		runningJobs:  newRunningJobsCache(),
		retryingJobs: newRetryingJobsCache(),

		addCh:    make(chan confgroup.Config),
		removeCh: make(chan confgroup.Config),
	}

	return mgr
}

type Manager struct {
	*logger.Logger

	PluginName string
	Out        io.Writer
	Modules    module.Registry

	FileLock    FileLocker
	StatusSaver StatusSaver
	StatusStore StatusStore
	Vnodes      Vnodes
	Dyncfg      Dyncfg

	confGroupCache *confgroup.Cache
	runningJobs    *runningJobsCache
	retryingJobs   *retryingJobsCache

	addCh    chan confgroup.Config
	removeCh chan confgroup.Config

	queueMux sync.Mutex
	queue    []Job
}

func (m *Manager) Run(ctx context.Context, in chan []*confgroup.Group) {
	m.Info("instance is started")
	defer func() { m.cleanup(); m.Info("instance is stopped") }()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); m.runConfigGroupsHandling(ctx, in) }()

	wg.Add(1)
	go func() { defer wg.Done(); m.runConfigsHandling(ctx) }()

	wg.Add(1)
	go func() { defer wg.Done(); m.runRunningJobsHandling(ctx) }()

	wg.Wait()
	<-ctx.Done()
}

func (m *Manager) runConfigGroupsHandling(ctx context.Context, in chan []*confgroup.Group) {
	for {
		select {
		case <-ctx.Done():
			return
		case groups := <-in:
			for _, gr := range groups {
				select {
				case <-ctx.Done():
					return
				default:
					a, r := m.confGroupCache.Add(gr)
					m.Debugf("received config group ('%s'): %d jobs (added: %d, removed: %d)", gr.Source, len(gr.Configs), len(a), len(r))
					sendConfigs(ctx, m.removeCh, r)
					sendConfigs(ctx, m.addCh, a)
				}
			}
		}
	}
}

func (m *Manager) runConfigsHandling(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cfg := <-m.addCh:
			m.addConfig(ctx, cfg)
		case cfg := <-m.removeCh:
			m.removeConfig(cfg)
		}
	}
}

func (m *Manager) cleanup() {
	for _, task := range *m.retryingJobs {
		task.cancel()
	}
	for name := range *m.runningJobs {
		_ = m.FileLock.Unlock(name)
	}
	// TODO: m.Dyncfg.Register() ?
	m.stopRunningJobs()
}

func (m *Manager) addConfig(ctx context.Context, cfg confgroup.Config) {
	task, isRetry := m.retryingJobs.lookup(cfg)
	if isRetry {
		task.cancel()
		m.retryingJobs.remove(cfg)
	} else {
		m.Dyncfg.Register(cfg)
	}

	if m.runningJobs.has(cfg) {
		m.Infof("%s[%s] job is being served by another job, skipping it", cfg.Module(), cfg.Name())
		m.StatusSaver.Save(cfg, jobStatusStoppedDupLocal)
		m.Dyncfg.UpdateStatus(cfg, "error", "duplicate, served by another job")
		return
	}

	job, err := m.createJob(cfg)
	if err != nil {
		m.Warningf("couldn't create %s[%s]: %v", cfg.Module(), cfg.Name(), err)
		m.StatusSaver.Save(cfg, jobStatusStoppedCreateErr)
		m.Dyncfg.UpdateStatus(cfg, "error", fmt.Sprintf("build error: %s", err))
		return
	}

	cleanupJob := true
	defer func() {
		if cleanupJob {
			job.Cleanup()
		}
	}()

	if isRetry {
		job.AutoDetectEvery = task.timeout
		job.AutoDetectTries = task.retries
	} else if job.AutoDetectionEvery() == 0 {
		switch {
		case m.StatusStore.Contains(cfg, jobStatusRunning, jobStatusRetrying):
			m.Infof("%s[%s] job last status is running/retrying, applying recovering settings", cfg.Module(), cfg.Name())
			job.AutoDetectEvery = 30
			job.AutoDetectTries = 11
		case isInsideK8sCluster() && cfg.Provider() == "file watcher":
			m.Infof("%s[%s] is k8s job, applying recovering settings", cfg.Module(), cfg.Name())
			job.AutoDetectEvery = 10
			job.AutoDetectTries = 7
		}
	}

	switch detection(job) {
	case jobStatusRunning:
		if ok, err := m.FileLock.Lock(cfg.FullName()); ok || err != nil && !isTooManyOpenFiles(err) {
			cleanupJob = false
			m.runningJobs.put(cfg)
			m.StatusSaver.Save(cfg, jobStatusRunning)
			m.Dyncfg.UpdateStatus(cfg, "running", "")
			m.startJob(job)
		} else if isTooManyOpenFiles(err) {
			m.Error(err)
			m.StatusSaver.Save(cfg, jobStatusStoppedRegErr)
			m.Dyncfg.UpdateStatus(cfg, "error", "too many open files")
		} else {
			m.Infof("%s[%s] job is being served by another plugin, skipping it", cfg.Module(), cfg.Name())
			m.StatusSaver.Save(cfg, jobStatusStoppedDupGlobal)
			m.Dyncfg.UpdateStatus(cfg, "error", "duplicate, served by another plugin")
		}
	case jobStatusRetrying:
		m.Infof("%s[%s] job detection failed, will retry in %d seconds", cfg.Module(), cfg.Name(), job.AutoDetectionEvery())
		ctx, cancel := context.WithCancel(ctx)
		m.retryingJobs.put(cfg, retryTask{
			cancel:  cancel,
			timeout: job.AutoDetectionEvery(),
			retries: job.AutoDetectTries,
		})
		go runRetryTask(ctx, m.addCh, cfg, time.Second*time.Duration(job.AutoDetectionEvery()))
		m.StatusSaver.Save(cfg, jobStatusRetrying)
		m.Dyncfg.UpdateStatus(cfg, "error", "job detection failed, will retry later")
	case jobStatusStoppedFailed:
		m.StatusSaver.Save(cfg, jobStatusStoppedFailed)
		m.Dyncfg.UpdateStatus(cfg, "error", "job detection failed, stopping it")
	default:
		m.Warningf("%s[%s] job detection: unknown state", cfg.Module(), cfg.Name())
	}
}

func (m *Manager) removeConfig(cfg confgroup.Config) {
	if m.runningJobs.has(cfg) {
		m.stopJob(cfg.FullName())
		_ = m.FileLock.Unlock(cfg.FullName())
		m.runningJobs.remove(cfg)
	}

	if task, ok := m.retryingJobs.lookup(cfg); ok {
		task.cancel()
		m.retryingJobs.remove(cfg)
	}

	m.StatusSaver.Remove(cfg)
	m.Dyncfg.Unregister(cfg)
}

func (m *Manager) createJob(cfg confgroup.Config) (*module.Job, error) {
	creator, ok := m.Modules[cfg.Module()]
	if !ok {
		return nil, fmt.Errorf("can not find %s module", cfg.Module())
	}

	m.Debugf("creating %s[%s] job, config: %v", cfg.Module(), cfg.Name(), cfg)

	mod := creator.Create()
	if err := unmarshal(cfg, mod); err != nil {
		return nil, err
	}

	labels := make(map[string]string)
	for name, value := range cfg.Labels() {
		n, ok1 := name.(string)
		v, ok2 := value.(string)
		if ok1 && ok2 {
			labels[n] = v
		}
	}

	jobCfg := module.JobConfig{
		PluginName:      m.PluginName,
		Name:            cfg.Name(),
		ModuleName:      cfg.Module(),
		FullName:        cfg.FullName(),
		UpdateEvery:     cfg.UpdateEvery(),
		AutoDetectEvery: cfg.AutoDetectionRetry(),
		Priority:        cfg.Priority(),
		Labels:          labels,
		IsStock:         isStockConfig(cfg),
		Module:          mod,
		Out:             m.Out,
	}

	if cfg.Vnode() != "" {
		n, ok := m.Vnodes.Lookup(cfg.Vnode())
		if !ok {
			return nil, fmt.Errorf("vnode '%s' is not found", cfg.Vnode())
		}

		jobCfg.VnodeGUID = n.GUID
		jobCfg.VnodeHostname = n.Hostname
		jobCfg.VnodeLabels = n.Labels
	}

	job := module.NewJob(jobCfg)

	return job, nil
}

func detection(job Job) jobStatus {
	if !job.AutoDetection() {
		if job.RetryAutoDetection() {
			return jobStatusRetrying
		} else {
			return jobStatusStoppedFailed
		}
	}
	return jobStatusRunning
}

func runRetryTask(ctx context.Context, out chan<- confgroup.Config, cfg confgroup.Config, timeout time.Duration) {
	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
		sendConfig(ctx, out, cfg)
	}
}

func sendConfigs(ctx context.Context, out chan<- confgroup.Config, cfgs []confgroup.Config) {
	for _, cfg := range cfgs {
		sendConfig(ctx, out, cfg)
	}
}

func sendConfig(ctx context.Context, out chan<- confgroup.Config, cfg confgroup.Config) {
	select {
	case <-ctx.Done():
		return
	case out <- cfg:
	}
}

func unmarshal(conf interface{}, module interface{}) error {
	bs, err := yaml.Marshal(conf)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bs, module)
}

func isInsideK8sCluster() bool {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	return host != "" && port != ""
}

func isTooManyOpenFiles(err error) bool {
	return err != nil && strings.Contains(err.Error(), "too many open files")
}

func isStockConfig(cfg confgroup.Config) bool {
	if !strings.HasPrefix(cfg.Provider(), "file") {
		return false
	}
	return !strings.Contains(cfg.Source(), "/etc/netdata")
}
