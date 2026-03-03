// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/internal/naming"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/metricsaudit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// jobFactory builds runtime jobs from configs without mutating manager-owned runtime maps.
type jobFactory struct {
	logger *logger.Logger

	pluginName string
	modules    collectorapi.Registry
	vnodes     *vnodeStore
	out        io.Writer

	auditMode     bool
	auditAnalyzer metricsaudit.Analyzer
	auditDataDir  string

	runtimeService runtimecomp.Service
}

func newJobFactory(m *Manager) *jobFactory {
	return &jobFactory{
		logger: m.Logger,

		pluginName: m.pluginName,
		modules:    m.modules,
		vnodes:     m.vnodes,
		out:        m.out,

		auditMode:     m.auditMode,
		auditAnalyzer: m.auditAnalyzer,
		auditDataDir:  m.auditDataDir,

		runtimeService: m.runtimeService,
	}
}

func (f *jobFactory) create(cfg confgroup.Config) (runtimeJob, error) {
	creator, ok := f.modules[cfg.Module()]
	if !ok {
		return nil, fmt.Errorf("can not find %s module", cfg.Module())
	}

	functionOnly := creator.FunctionOnly || cfg.FunctionOnly()
	if cfg.FunctionOnly() && creator.Methods == nil && creator.JobMethods == nil {
		return nil, fmt.Errorf("function_only is set but %s module has no methods defined", cfg.Module())
	}

	var vnode *vnodes.VirtualNode
	if cfg.Vnode() != "" {
		n, ok := f.vnodes.Lookup(cfg.Vnode())
		if !ok || n == nil {
			return nil, fmt.Errorf("vnode '%s' is not found", cfg.Vnode())
		}
		vnode = n
	}

	f.logger.Debugf("creating %s[%s] job, config: %v", cfg.Module(), cfg.Name(), cfg)

	if creator.CreateV2 != nil {
		return f.createV2(cfg, creator, functionOnly, vnode)
	}
	return f.createV1(cfg, creator, functionOnly, vnode)
}

func (f *jobFactory) createV2(cfg confgroup.Config, creator collectorapi.Creator, functionOnly bool, vnode *vnodes.VirtualNode) (runtimeJob, error) {
	mod := creator.CreateV2()
	if mod == nil {
		return nil, fmt.Errorf("module %s CreateV2 returned nil", cfg.Module())
	}
	if err := applyConfig(cfg, mod); err != nil {
		return nil, err
	}

	jobCfg := jobruntime.JobV2Config{
		PluginName:      f.pluginName,
		Name:            cfg.Name(),
		ModuleName:      cfg.Module(),
		FullName:        cfg.FullName(),
		UpdateEvery:     cfg.UpdateEvery(),
		AutoDetectEvery: cfg.AutoDetectionRetry(),
		IsStock:         cfg.SourceType() == "stock",
		Labels:          makeLabels(cfg),
		Out:             f.out,
		Module:          mod,
		FunctionOnly:    functionOnly,
		RuntimeService:  f.runtimeService,
	}
	if vnode != nil {
		jobCfg.Vnode = *vnode.Copy()
	}
	return jobruntime.NewJobV2(jobCfg), nil
}

func (f *jobFactory) createV1(cfg confgroup.Config, creator collectorapi.Creator, functionOnly bool, vnode *vnodes.VirtualNode) (runtimeJob, error) {
	if creator.Create == nil {
		return nil, fmt.Errorf("module %s has no compatible creator", cfg.Module())
	}

	jobCaptureDir, err := f.createV1CaptureDir(cfg)
	if err != nil {
		return nil, err
	}

	mod := creator.Create()
	if err := applyConfig(cfg, mod); err != nil {
		return nil, err
	}

	if f.auditAnalyzer != nil && jobCaptureDir != "" {
		f.auditAnalyzer.RegisterJob(cfg.Name(), cfg.Module(), jobCaptureDir)
	}
	if jobCaptureDir != "" {
		if captureAware, ok := mod.(metricsaudit.Capturable); ok {
			captureAware.EnableCaptureArtifacts(jobCaptureDir)
		}
	}

	jobCfg := jobruntime.JobConfig{
		PluginName:      f.pluginName,
		Name:            cfg.Name(),
		ModuleName:      cfg.Module(),
		FullName:        cfg.FullName(),
		UpdateEvery:     cfg.UpdateEvery(),
		AutoDetectEvery: cfg.AutoDetectionRetry(),
		Priority:        cfg.Priority(),
		Labels:          makeLabels(cfg),
		IsStock:         cfg.SourceType() == "stock",
		Module:          mod,
		Out:             f.out,
		AuditMode:       f.auditMode,
		AuditAnalyzer:   f.auditAnalyzer,
		FunctionOnly:    functionOnly,
	}
	if vnode != nil {
		jobCfg.Vnode = *vnode.Copy()
	}

	return jobruntime.NewJob(jobCfg), nil
}

func (f *jobFactory) createV1CaptureDir(cfg confgroup.Config) (string, error) {
	if f.auditDataDir == "" {
		return "", nil
	}
	jobCaptureDir := filepath.Join(f.auditDataDir, naming.Sanitize(cfg.Module()), naming.Sanitize(cfg.Name()))
	if err := os.MkdirAll(jobCaptureDir, 0o755); err != nil {
		return "", fmt.Errorf("creating audit directory: %w", err)
	}
	return jobCaptureDir, nil
}
