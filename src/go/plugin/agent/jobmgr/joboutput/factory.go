// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
)

type ModuleCatalog interface {
	Lookup(string) (collectorapi.Creator, bool)
}

type RuntimeJob interface {
	ManagedJob
	collectorapi.RuntimeJob

	AutoDetectionManaged(context.Context) error
	AutoDetectionEvery() int
	RetryAutoDetection() bool
	CleanupRejected()
	Tick(int)
}

type HandlerLifecycle interface {
	Publish() error
	CloseAndDrain(context.Context) error
}

type ProcessHandlerLifecycle interface {
	HandlerLifecycle
	Detach(context.Context) error
	Finalize(context.Context) error
}

type StagedHandlerLifecycle interface {
	CloseAndDrain(context.Context) error
}

type JobHandlerStager interface {
	Stage(RuntimeJob) (StagedHandlerLifecycle, error)
}

type JobHandlerAttacher interface {
	Attach(lifecycle.ResourceIdentity, StagedHandlerLifecycle) (HandlerLifecycle, error)
}

type JobHooks interface {
	JobHandlerStager
	JobHandlerAttacher
}

type jobNamedModule interface {
	SetJobName(string)
}

type FactoryConfig struct {
	Epoch            uint64                                                            // target run generation
	PluginName       string                                                            // owning plugin name stamped into job config
	Attempts         jobmgr.ProcessAttemptAuthority                                    // process-owned candidate authority
	Modules          ModuleCatalog                                                     // collector creator registry
	Tasks            *lifecycle.TaskSupervisor                                         // supervisor owning inherited run-loop goroutines
	Frames           *lifecycle.FrameOwner                                             // frame owner used as the collector output sink
	ConfigModules    *ConfigModuleFactory                                              // resolved config application and short-lived probes
	Runtime          runtimecomp.Service                                               // V2 runtime service dependency
	RuntimeStaging   bool                                                              // construct a private V2 staging capability
	Vnodes           *vnoderegistry.Registry                                           // vnode registry for V2 jobs
	Vnode            func(string) (jobruntime.VnodeSnapshot, bool)                     // vnode snapshot lookup by name
	HandlerStager    JobHandlerStager                                                  // run-detached Function-handler staging
	HandlerAttacher  JobHandlerAttacher                                                // run-owned Function publication attachment
	Scheduler        *Scheduler                                                        // tick/registration scheduler
	Observer         lifecycle.RuntimeObserver                                         // runtime gauge sink
	RunWithoutClaims func(context.Context, func(context.Context) error) (error, error) // claim-yield adapter for probes
}

// Factory owns collector construction, validation, and transfer. It does not
// own current-job indexing or lifecycle state.
type Factory struct {
	config FactoryConfig
}

type factoryAttachment struct {
	tasks           *lifecycle.TaskSupervisor
	runtime         runtimecomp.Service
	vnode           func(string) (jobruntime.VnodeSnapshot, bool)
	handlerAttacher JobHandlerAttacher
	scheduler       *Scheduler
	observer        lifecycle.RuntimeObserver
}

func (f *Factory) attachment() factoryAttachment {
	if f == nil {
		return factoryAttachment{}
	}
	return factoryAttachment{
		tasks:           f.config.Tasks,
		runtime:         f.config.Runtime,
		vnode:           f.config.Vnode,
		handlerAttacher: f.config.HandlerAttacher,
		scheduler:       f.config.Scheduler,
		observer:        f.config.Observer,
	}
}

func (attachment factoryAttachment) attach(
	candidate ConstructedJob,
	identity lifecycle.ResourceIdentity,
	owner *stagedJobOwner,
) (ConstructedJob, error) {
	if !identity.Valid() ||
		candidate.candidateJob == nil ||
		(owner == nil && attachment.tasks == nil) ||
		attachment.scheduler == nil {
		return candidate, errors.New("job output: invalid candidate attachment")
	}
	var attached ConstructedJob
	var err error
	if owner == nil {
		attached, err = newManagedJob(
			candidate.Variant,
			candidate.candidateJob,
			attachment.tasks,
			identity,
			attachment.scheduler,
			candidate.CollectorCleanup,
		)
	} else {
		attached, err = newProcessManagedJob(
			candidate.Variant,
			candidate.candidateJob,
			identity,
			attachment.scheduler,
			candidate.CollectorCleanup,
			owner,
		)
	}
	if err != nil {
		return candidate, err
	}
	attached.Observer = attachment.observer
	attached.resolvedReferences = candidate.resolvedReferences
	attached.finalCleanup = candidate.finalCleanup
	attached.runtimeStage = candidate.runtimeStage
	attached.vnodeStage = candidate.vnodeStage
	attached.outputGate = candidate.outputGate
	attached.StagedHandlers = candidate.StagedHandlers
	if candidate.runtimeStage != nil ||
		candidate.vnodeStage != nil ||
		candidate.outputGate != nil {
		attached.activateAttachment = func() error {
			var runtimeErr error
			if candidate.runtimeStage != nil {
				runtimeErr = candidate.runtimeStage.attach(attachment.runtime)
			}
			if runtimeErr != nil {
				return runtimeErr
			}
			if candidate.vnodeStage != nil {
				if err := candidate.vnodeStage.attach(attachment.vnode); err != nil {
					return err
				}
			}
			if candidate.outputGate != nil {
				return candidate.outputGate.Activate()
			}
			return nil
		}
	}
	if candidate.StagedHandlers != nil {
		handlers, attachErr := callAttachHandlers(
			attachment.handlerAttacher,
			identity,
			candidate.StagedHandlers,
		)
		if handlers != nil {
			attached.Handlers = handlers
			attached.StagedHandlers = nil
		}
		if attachErr != nil {
			return attached, attachErr
		}
		if handlers == nil {
			return attached, errors.New("job output: nil attached handler lifecycle")
		}
		if owner != nil {
			if _, ok := handlers.(ProcessHandlerLifecycle); !ok {
				return attached, errors.New("job output: attached handler lifecycle cannot detach")
			}
		}
	}
	return attached, nil
}

func NewFactory(config FactoryConfig) (*Factory, error) {
	if config.PluginName == "" ||
		config.Modules == nil ||
		config.Tasks == nil ||
		config.Frames == nil ||
		config.ConfigModules == nil ||
		config.Vnodes == nil ||
		config.Scheduler == nil {
		return nil, errors.New("job output: incomplete factory configuration")
	}
	if config.RunWithoutClaims == nil {
		config.RunWithoutClaims = jobmgr.RunWithoutClaims
	}
	return &Factory{
		config: config,
	}, nil
}

func (f *Factory) ValidateConfig(ctx context.Context, config confgroup.Config) error {
	if f == nil || ctx == nil || config == nil {
		return errors.New("job output: invalid factory validation")
	}
	creator, ok := f.config.Modules.Lookup(config.Module())
	if !ok {
		return fmt.Errorf("job output: module %q is not registered", config.Module())
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return err
	}
	if config.FunctionOnly() && !creatorDeclaresFunctions(creator) {
		return invalidJobConfiguration(
			fmt.Errorf("job output: function_only is set but module %q declares no Functions", config.Module()),
		)
	}
	if _, err := f.lookupVNode(config); err != nil {
		return err
	}
	return f.config.ConfigModules.Validate(ctx, config)
}

func (f *Factory) build(
	ctx context.Context,
	config confgroup.Config,
) (constructed ConstructedJob, resultErr error) {
	if f == nil || ctx == nil || config == nil {
		return ConstructedJob{}, errors.New("job output: invalid factory build")
	}
	creator, ok := f.config.Modules.Lookup(config.Module())
	if !ok {
		return ConstructedJob{}, fmt.Errorf("job output: module %q is not registered", config.Module())
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return ConstructedJob{}, err
	}
	functionOnly := creator.FunctionOnly || config.FunctionOnly()
	hasFunctions := creatorDeclaresFunctions(creator)
	if functionOnly && !hasFunctions {
		return ConstructedJob{}, invalidJobConfiguration(
			fmt.Errorf(
				"job output: function_only is set but module %q declares no Functions",
				config.Module(),
			),
		)
	}
	vnode, err := f.lookupVNode(config)
	if err != nil {
		return ConstructedJob{}, err
	}
	var job RuntimeJob
	var variant JobVariant
	var redactLifecycle bool
	var runtimeStage *stagedRuntimeService
	var vnodeStage *stagedVNodeLookup
	outputGate, err := newGenerationOutputGate(f.config.Frames)
	if err != nil {
		return ConstructedJob{}, err
	}
	if vnode.Vnode != nil {
		vnodeStage = newStagedVNodeLookup(config.Vnode(), vnode)
	}
	defer func() {
		if redactLifecycle && resultErr != nil {
			resultErr = redactResolvedLifecycleError(resultErr)
		}
	}()
	if creator.CreateV2 != nil {
		job, runtimeStage, redactLifecycle, err = f.buildV2(
			ctx,
			config,
			creator,
			functionOnly,
			vnode,
			vnodeStage,
			outputGate,
		)
		variant = JobVariantV2
	} else {
		job, redactLifecycle, err = f.buildV1(
			ctx,
			config,
			creator,
			functionOnly,
			vnode,
			vnodeStage,
			outputGate,
		)
		variant = JobVariantV1
	}
	if err != nil {
		return ConstructedJob{}, err
	}
	cleanup := &factoryJobCleanup{
		job:          job,
		runtimeStage: runtimeStage,
		vnodeStage:   vnodeStage,
		redact:       redactLifecycle,
	}
	constructed = ConstructedJob{
		Variant:            variant,
		CollectorCleanup:   cleanup.reject,
		autoDetection:      job.AutoDetectionManaged,
		autoDetectionEvery: job.AutoDetectionEvery,
		retryAutoDetection: job.RetryAutoDetection,
		finalCleanup:       cleanup.final,
		resolvedReferences: redactLifecycle,
		candidateJob:       job,
		runtimeStage:       runtimeStage,
		vnodeStage:         vnodeStage,
		outputGate:         outputGate,
	}
	if hasFunctions && f.config.HandlerStager == nil {
		return constructed, errors.New("job output: function-bearing job has no handler lifecycle")
	}
	if hasFunctions {
		handlers, prepareErr := callStageHandlers(f.config.HandlerStager, job)
		constructed.StagedHandlers = handlers
		if prepareErr != nil {
			return constructed, prepareErr
		}
		if handlers == nil {
			return constructed, errors.New("job output: nil prepared handler lifecycle")
		}
	}
	return constructed, nil
}

func (f *Factory) Prepare(
	ctx context.Context,
	config confgroup.Config,
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
) (PreparedJob, error) {
	if f == nil || ctx == nil || config == nil || !identity.Valid() || identity.ID != config.FullName() {
		return PreparedJob{}, errors.New("job output: invalid factory preparation")
	}
	candidate, err := f.build(ctx, config)
	if err != nil {
		cleanupErr := cleanupConstructed(context.WithoutCancel(ctx), candidate)
		err = errors.Join(err, cleanupErr)
		if lifecycle.OwnershipRetained(err) || cleanupErr != nil {
			err = lifecycle.RetainOwnership(err)
		}
		return PreparedJob{}, err
	}
	return prepareCandidateJob(
		identity,
		permit,
		candidate,
		f.attachment(),
		nil,
		false,
	)
}

func (f *Factory) Probe(ctx context.Context, prepared PreparedJob) (*autoDetectionFailure, error) {
	if f == nil || ctx == nil || !prepared.Valid() {
		return nil, errors.New("job output: invalid factory probe")
	}
	var failure *autoDetectionFailure
	var rejectErr error
	rejected := false
	probeErr, claimErr := f.config.RunWithoutClaims(ctx, func(probeParent context.Context) error {
		err := prepared.Probe(probeParent)
		if err == nil {
			return nil
		}
		_ = errors.As(err, &failure)
		rejectErr = prepared.reject(context.WithoutCancel(probeParent))
		rejected = true
		return err
	})
	if claimErr != nil {
		if !rejected {
			rejectErr = prepared.reject(context.WithoutCancel(ctx))
		}
		err := errors.Join(claimErr, probeErr, rejectErr)
		if rejectErr != nil {
			err = lifecycle.RetainOwnership(err)
		}
		return nil, err
	}
	if probeErr == nil {
		return nil, nil
	}
	if failure == nil {
		err := errors.Join(probeErr, rejectErr)
		if rejectErr != nil {
			err = lifecycle.RetainOwnership(err)
		}
		return nil, err
	}
	if rejectErr != nil {
		return nil, lifecycle.RetainOwnership(errors.Join(probeErr, rejectErr))
	}
	return failure, nil
}

type factoryJobCleanup struct {
	once         sync.Once
	job          RuntimeJob
	runtimeStage *stagedRuntimeService
	vnodeStage   *stagedVNodeLookup
	redact       bool
	err          error
}

func (fjc *factoryJobCleanup) reject(context.Context) error {
	fjc.once.Do(func() {
		fjc.err = callJobLifecycle("rejected collector Cleanup", func() error {
			fjc.job.CleanupRejected()
			fjc.runtimeStage.close()
			fjc.vnodeStage.close()
			return nil
		})
		if fjc.redact {
			fjc.err = redactResolvedLifecycleError(fjc.err)
		}
	})
	return fjc.err
}

func (fjc *factoryJobCleanup) final(context.Context) error {
	fjc.once.Do(func() {
		fjc.err = callJobLifecycle("collector Cleanup", func() error {
			fjc.job.Cleanup()
			fjc.runtimeStage.close()
			fjc.vnodeStage.close()
			return nil
		})
		if fjc.redact {
			fjc.err = redactResolvedLifecycleError(fjc.err)
		}
	})
	return fjc.err
}

func callStageHandlers(
	stager JobHandlerStager,
	job RuntimeJob,
) (handlers StagedHandlerLifecycle, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			handlers = nil
			err = lifecycle.RetainOwnership(fmt.Errorf(
				"%w in handler preparation: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			))
		}
	}()
	return stager.Stage(job)
}

func callAttachHandlers(
	attacher JobHandlerAttacher,
	identity lifecycle.ResourceIdentity,
	staged StagedHandlerLifecycle,
) (handlers HandlerLifecycle, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			handlers = nil
			err = fmt.Errorf(
				"%w in handler attachment: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			)
		}
	}()
	return attacher.Attach(identity, staged)
}

func (f *Factory) buildV1(
	ctx context.Context,
	config confgroup.Config,
	creator collectorapi.Creator,
	functionOnly bool,
	vnode jobruntime.VnodeSnapshot,
	vnodeStage *stagedVNodeLookup,
	outputGate *generationOutputGate,
) (job RuntimeJob, redactLifecycle bool, err error) {
	if creator.Create == nil {
		return nil, false, fmt.Errorf("job output: module %q has no V1 creator", config.Module())
	}
	var module collectorapi.CollectorV1
	defer func() {
		if recovered := recover(); recovered != nil {
			job = nil
			err = lifecycle.RetainOwnership(fmt.Errorf(
				"%w in V1 construction: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			))
		}
		if err != nil && module != nil {
			cleanupErr := callFactoryModuleCleanup(ctx, module.Cleanup)
			if redactLifecycle {
				cleanupErr = redactResolvedLifecycleError(cleanupErr)
			}
			err = errors.Join(err, cleanupErr)
			if cleanupErr != nil {
				err = lifecycle.RetainOwnership(err)
			}
		}
		if redactLifecycle && err != nil {
			err = redactResolvedLifecycleError(err)
		}
	}()
	module = creator.Create()
	if module == nil {
		return nil, false, fmt.Errorf("job output: module %q returned a nil V1 collector", config.Module())
	}
	setModuleJobName(module, config.Name())
	redactLifecycle, err = f.config.ConfigModules.applyResolved(ctx, config, module)
	if err != nil {
		return nil, redactLifecycle, err
	}
	jobConfig := jobruntime.JobConfig{
		PluginName:      f.config.PluginName,
		Name:            config.Name(),
		ModuleName:      config.Module(),
		FullName:        config.FullName(),
		Source:          factoryLogSource(config),
		Module:          module,
		Labels:          factoryLabels(config),
		Out:             outputGate,
		UpdateEvery:     config.UpdateEvery(),
		AutoDetectEvery: config.AutoDetectionRetry(),
		Priority:        config.Priority(),
		IsStock:         config.SourceType() == confgroup.TypeStock,
		FunctionOnly:    functionOnly,
	}
	if redactLifecycle {
		jobConfig.LifecycleErrorSanitizer = redactResolvedLifecycleError
	}
	if vnode.Vnode != nil {
		jobConfig.Vnode = *vnode.Vnode.Copy()
		jobConfig.VnodeName = config.Vnode()
		jobConfig.VnodeRevision = vnode.Revision
		jobConfig.VnodeMetadataRevision = vnode.MetadataRevision
		jobConfig.VnodeLookup = vnodeStage.Lookup
	}
	return jobruntime.NewJob(jobConfig), redactLifecycle, nil
}

func (f *Factory) buildV2(
	ctx context.Context,
	config confgroup.Config,
	creator collectorapi.Creator,
	functionOnly bool,
	vnode jobruntime.VnodeSnapshot,
	vnodeStage *stagedVNodeLookup,
	outputGate *generationOutputGate,
) (job RuntimeJob, runtimeStage *stagedRuntimeService, redactLifecycle bool, err error) {
	var module collectorapi.CollectorV2
	defer func() {
		if recovered := recover(); recovered != nil {
			job = nil
			err = lifecycle.RetainOwnership(fmt.Errorf(
				"%w in V2 construction: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			))
		}
		if err != nil && module != nil {
			cleanupErr := callFactoryModuleCleanup(ctx, module.Cleanup)
			if redactLifecycle {
				cleanupErr = redactResolvedLifecycleError(cleanupErr)
			}
			err = errors.Join(err, cleanupErr)
			if cleanupErr != nil {
				err = lifecycle.RetainOwnership(err)
			}
		}
		if err != nil && runtimeStage != nil {
			runtimeStage.close()
			runtimeStage = nil
		}
		if redactLifecycle && err != nil {
			err = redactResolvedLifecycleError(err)
		}
	}()
	module = creator.CreateV2()
	if module == nil {
		return nil, nil, false, fmt.Errorf("job output: module %q returned a nil V2 collector", config.Module())
	}
	setModuleJobName(module, config.Name())
	redactLifecycle, err = f.config.ConfigModules.applyResolved(ctx, config, module)
	if err != nil {
		return nil, nil, redactLifecycle, err
	}
	if f.config.Runtime != nil || f.config.RuntimeStaging {
		runtimeStage = newStagedRuntimeService()
	}
	jobConfig := jobruntime.JobV2Config{
		PluginName:      f.config.PluginName,
		Name:            config.Name(),
		ModuleName:      config.Module(),
		FullName:        config.FullName(),
		Source:          factoryLogSource(config),
		Module:          module,
		Labels:          factoryLabels(config),
		Out:             outputGate,
		UpdateEvery:     config.UpdateEvery(),
		AutoDetectEvery: config.AutoDetectionRetry(),
		IsStock:         config.SourceType() == confgroup.TypeStock,
		FunctionOnly:    functionOnly,
		RuntimeService:  runtimeStage,
		VnodeRegistry:   f.config.Vnodes,
	}
	if redactLifecycle {
		jobConfig.LifecycleErrorSanitizer = redactResolvedLifecycleError
	}
	if vnode.Vnode != nil {
		jobConfig.Vnode = *vnode.Vnode.Copy()
		jobConfig.VnodeName = config.Vnode()
		jobConfig.VnodeRevision = vnode.Revision
		jobConfig.VnodeMetadataRevision = vnode.MetadataRevision
		jobConfig.VnodeLookup = vnodeStage.Lookup
	}
	return jobruntime.NewJobV2(jobConfig), runtimeStage, redactLifecycle, nil
}

func callFactoryModuleCleanup(ctx context.Context, cleanup func(context.Context)) error {
	return callJobLifecycle("construction collector Cleanup", func() error {
		cleanup(context.WithoutCancel(ctx))
		return nil
	})
}

func (f *Factory) lookupVNode(config confgroup.Config) (jobruntime.VnodeSnapshot, error) {
	if config.Vnode() == "" {
		return jobruntime.VnodeSnapshot{}, nil
	}
	if f.config.Vnode == nil {
		return jobruntime.VnodeSnapshot{}, transientJobConstruction(
			fmt.Errorf("job output: vnode %q is unavailable", config.Vnode()),
		)
	}
	vnode, ok := f.config.Vnode(config.Vnode())
	if !ok || vnode.Vnode == nil {
		return jobruntime.VnodeSnapshot{}, transientJobConstruction(
			fmt.Errorf("job output: vnode %q is not registered", config.Vnode()),
		)
	}
	return vnode, nil
}

func validateFactoryConfigIdentity(config confgroup.Config, creator collectorapi.Creator) error {
	if creator.InstancePolicy != collectorapi.InstancePolicySingle || config.Name() == config.Module() {
		return nil
	}
	return invalidJobConfiguration(
		fmt.Errorf(
			"job output: single-instance module %q requires config name %q, got %q",
			config.Module(),
			config.Module(),
			config.Name(),
		),
	)
}

func factoryLogSource(config confgroup.Config) string {
	if config.SourceType() != "" && config.SourceType() == config.Provider() {
		return config.SourceType()
	}
	return fmt.Sprintf("%s/%s", config.SourceType(), config.Provider())
}

func factoryLabels(config confgroup.Config) map[string]string {
	labels := make(map[string]string)
	for rawName, rawValue := range config.Labels() {
		name, nameOK := rawName.(string)
		value, valueOK := rawValue.(string)
		if nameOK && valueOK {
			labels[name] = value
		}
	}
	return labels
}

func creatorDeclaresFunctions(creator collectorapi.Creator) bool {
	return creator.SharedFunctions != nil || creator.AgentFunctions != nil || creator.InstanceFunctions != nil
}

func setModuleJobName(module any, name string) {
	if named, ok := module.(jobNamedModule); ok {
		named.SetJobName(name)
	}
}
