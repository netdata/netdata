// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/vnodectl"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
)

func jobLogSource(cfg confgroup.Config) string {
	sourceType := cfg.SourceType()
	provider := cfg.Provider()
	if sourceType != "" && sourceType == provider {
		return sourceType
	}
	return fmt.Sprintf("%s/%s", sourceType, provider)
}

// jobFactory builds runtime jobs from configs. It does not mutate
// manager-owned runtime maps, with one disclosed exception: a successful
// create registers the job's emission gateway in the manager's gate
// registry, which carries its own lock.
type jobFactory struct {
	logger *logger.Logger

	pluginName          string
	modules             collectorapi.Registry
	vnodeSnapshotLookup func(string) (jobruntime.VnodeSnapshot, bool)
	out                 io.Writer
	gates               *emissionGates
	onSuppressedWrite   func()

	validationOnly bool

	runtimeService runtimecomp.Service
	vnodeRegistry  *vnoderegistry.Registry

	secretResolver *secretresolver.Resolver
	secretStoreSvc secretstore.Service
	ctx            context.Context
}

type jobNameSetter interface {
	SetJobName(string)
}

func newJobFactory(m *Manager) *jobFactory {
	return &jobFactory{
		logger: m.Logger,

		pluginName: m.pluginName,
		modules:    m.modules,
		vnodeSnapshotLookup: func(name string) (jobruntime.VnodeSnapshot, bool) {
			snapshot, ok := m.vnodesCtl.LookupSnapshot(name)
			if !ok {
				return jobruntime.VnodeSnapshot{}, false
			}
			return toRuntimeVnodeSnapshot(snapshot), true
		},
		out:               m.out,
		gates:             m.emissionGates,
		onSuppressedWrite: m.observeSuppressedWrite,

		runtimeService: m.runtimeService,
		vnodeRegistry:  m.vnodeRegistry,
		secretResolver: m.secretResolver,
		secretStoreSvc: m.secretsCtl.Service(),
		ctx:            m.baseContext(),
	}
}

func toRuntimeVnodeSnapshot(snapshot vnodectl.Snapshot) jobruntime.VnodeSnapshot {
	return jobruntime.VnodeSnapshot{
		Vnode:            snapshot.Vnode,
		Revision:         snapshot.Revision,
		MetadataRevision: snapshot.MetadataRevision,
	}
}

func (f *jobFactory) validate(cfg confgroup.Config) error {
	clone := *f
	clone.validationOnly = true
	_, err := clone.create(cfg)
	return err
}

func (f *jobFactory) create(cfg confgroup.Config) (runtimeJob, error) {
	creator, ok := f.modules[cfg.Module()]
	if !ok {
		return nil, fmt.Errorf("can not find %s module", cfg.Module())
	}
	if err := validateCollectorConfigIdentity(cfg, creator); err != nil {
		return nil, err
	}

	functionOnly := creator.FunctionOnly || cfg.FunctionOnly()
	if cfg.FunctionOnly() && creator.SharedFunctions == nil && creator.AgentFunctions == nil && creator.InstanceFunctions == nil {
		return nil, fmt.Errorf("function_only is set but %s module has no functions defined", cfg.Module())
	}

	var vnode jobruntime.VnodeSnapshot
	if cfg.Vnode() != "" {
		if f.vnodeSnapshotLookup == nil {
			return nil, fmt.Errorf("vnode '%s' is not found", cfg.Vnode())
		}
		n, ok := f.vnodeSnapshotLookup(cfg.Vnode())
		if !ok || n.Vnode == nil {
			return nil, fmt.Errorf("vnode '%s' is not found", cfg.Vnode())
		}
		vnode = n
	}

	f.logger.Debugf("creating %s[%s] job, config: %v", cfg.Module(), cfg.Name(), cfg)

	// Every runnable job writes through its own emission gateway so job
	// output can be provably fenced off; the gate stays open for the job's
	// whole life on the normal path. Validation-only jobs never run, so they
	// get no gate. The gate is tracked only for jobs that construct
	// successfully; the create/detect and stop paths drop the entry when the
	// job fails or stops. A same-name replacement overwrites the previous
	// entry here, and startRunningJob preserves the tracked gate across its
	// defensive stop - the entry always belongs to the newest created job.
	gatedF := *f
	var gate *emissionGateway
	if !f.validationOnly {
		gate = newEmissionGateway(f.out, f.onSuppressedWrite)
		gatedF.out = gate
	}

	var job runtimeJob
	var err error
	if creator.CreateV2 != nil {
		job, err = gatedF.createV2(cfg, creator, functionOnly, vnode)
	} else {
		job, err = gatedF.createV1(cfg, creator, functionOnly, vnode)
	}
	if err != nil {
		return nil, err
	}
	if gate != nil && f.gates != nil {
		f.gates.add(cfg.FullName(), gate)
	}
	return job, nil
}

func (f *jobFactory) logApplyConfigError(cfg confgroup.Config, err error) {
	if f.validationOnly {
		return
	}
	f.logger.Errorf("failed to apply config for %s[%s] job: %v", cfg.Module(), cfg.Name(), err)
}

func (f *jobFactory) createV2(cfg confgroup.Config, creator collectorapi.Creator, functionOnly bool, vnode jobruntime.VnodeSnapshot) (runtimeJob, error) {
	mod := creator.CreateV2()
	if mod == nil {
		return nil, fmt.Errorf("module %s CreateV2 returned nil", cfg.Module())
	}
	if named, ok := mod.(jobNameSetter); ok {
		named.SetJobName(cfg.Name())
	}
	storeSnapshot := f.secretStoreSvc.Capture()
	resolveCtx := collectorSecretResolveContext(f.ctx, f.logger, cfg)
	if err := applyConfig(resolveCtx, cfg, mod, f.secretResolver, f.secretStoreSvc, storeSnapshot); err != nil {
		f.logApplyConfigError(cfg, err)
		return nil, err
	}

	jobCfg := jobruntime.JobV2Config{
		PluginName:      f.pluginName,
		Name:            cfg.Name(),
		ModuleName:      cfg.Module(),
		FullName:        cfg.FullName(),
		Source:          jobLogSource(cfg),
		UpdateEvery:     cfg.UpdateEvery(),
		AutoDetectEvery: cfg.AutoDetectionRetry(),
		IsStock:         cfg.SourceType() == "stock",
		Labels:          makeLabels(cfg),
		Out:             f.out,
		Module:          mod,
		FunctionOnly:    functionOnly,
		RuntimeService:  f.runtimeService,
		VnodeRegistry:   f.vnodeRegistry,
	}
	if vnode.Vnode != nil {
		jobCfg.Vnode = *vnode.Vnode.Copy()
		jobCfg.VnodeName = cfg.Vnode()
		jobCfg.VnodeRevision = vnode.Revision
		jobCfg.VnodeMetadataRevision = vnode.MetadataRevision
		jobCfg.VnodeLookup = f.vnodeSnapshotLookup
	}
	return jobruntime.NewJobV2(jobCfg), nil
}

func (f *jobFactory) createV1(cfg confgroup.Config, creator collectorapi.Creator, functionOnly bool, vnode jobruntime.VnodeSnapshot) (runtimeJob, error) {
	if creator.Create == nil {
		return nil, fmt.Errorf("module %s has no compatible creator", cfg.Module())
	}

	mod := creator.Create()
	if named, ok := mod.(jobNameSetter); ok {
		named.SetJobName(cfg.Name())
	}
	storeSnapshot := f.secretStoreSvc.Capture()
	resolveCtx := collectorSecretResolveContext(f.ctx, f.logger, cfg)
	if err := applyConfig(resolveCtx, cfg, mod, f.secretResolver, f.secretStoreSvc, storeSnapshot); err != nil {
		f.logApplyConfigError(cfg, err)
		return nil, err
	}

	jobCfg := jobruntime.JobConfig{
		PluginName:      f.pluginName,
		Name:            cfg.Name(),
		ModuleName:      cfg.Module(),
		FullName:        cfg.FullName(),
		Source:          jobLogSource(cfg),
		UpdateEvery:     cfg.UpdateEvery(),
		AutoDetectEvery: cfg.AutoDetectionRetry(),
		Priority:        cfg.Priority(),
		Labels:          makeLabels(cfg),
		IsStock:         cfg.SourceType() == "stock",
		Module:          mod,
		Out:             f.out,
		FunctionOnly:    functionOnly,
	}
	if vnode.Vnode != nil {
		jobCfg.Vnode = *vnode.Vnode.Copy()
		jobCfg.VnodeName = cfg.Vnode()
		jobCfg.VnodeRevision = vnode.Revision
		jobCfg.VnodeMetadataRevision = vnode.MetadataRevision
		jobCfg.VnodeLookup = f.vnodeSnapshotLookup
	}

	return jobruntime.NewJob(jobCfg), nil
}
